package repositories

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/security"

	models "github.com/arandu-io/examples/app/Models"
	policies "github.com/arandu-io/examples/app/Policies"
)

// Pagination bounds for comments. A request that asks for everything gets the
// maximum, never everything: an unbounded query is how one page load takes a
// database down.
const (
	commentDefaultLimit = 50
	commentMaxLimit     = 200
)

// CommentRepository is the only door to the comments table.
//
// Every method starts with g.Check. The Grant is required by the signature, so
// forgetting it is a compile error, and the check proves the grant was issued for
// this exact action -- which catches copy-paste between methods.
//
// The SQL uses "?" placeholders and types every supported database shares, so
// these statements run unchanged on SQLite and PostgreSQL.
//
// # What kind of repository this is: specialised query, decided by the writes
//
// A repository is where a query too complex to generate lives, and here it is
// the writing that is too complex rather than the reading.
//
// Create is INSERT INTO comments ... SELECT ? ... WHERE EXISTS (SELECT 1 FROM
// posts WHERE id = ? AND tenant_id = ?). Update carries the same EXISTS beside
// its own id and tenant, and classifyUpdateMiss reads two of them in a single
// row. Three statements naming a second table, to enforce in one shot what a
// foreign key on post_id cannot say: not that the post exists somewhere, but
// that it exists in this tenant. A single-table insert of the kind a generated
// model emits cannot express that, and splitting it into a check and then a
// write puts a race between the two.
//
// The reads are plainer. Find, List and ForPost select from comments alone,
// keyed by id or by post_id and filtered by tenant, and they are CRUD.
// PublicForPost is the one read that is more: (approved = ? OR author = ?) is
// the visibility rule of the thread, in the predicate rather than applied to
// rows already in memory. It answers with the entity and not with a shape of
// its own, so it is a narrowed query rather than a read model -- there is no
// read model in this file.
type CommentRepository struct {
	db *data.DB
}

// NewCommentRepository returns a repository over an instrumented handle.
func NewCommentRepository(db *data.DB) *CommentRepository { return &CommentRepository{db: db} }

// Compile-time proof of the contract.
var _ data.Repository[models.Comment, string] = (*CommentRepository)(nil)

const commentColumns = `id, tenant_id, post_id, author, body, approved, created_at`

// Find returns one comment by id, scoped to the grant's tenant.
func (r *CommentRepository) Find(ctx context.Context, g security.Grant, id string) (models.Comment, error) {
	if err := g.Check(policies.CommentView); err != nil {
		return models.Comment{}, err
	}
	// The tenant comes from the Grant, never from the request. An id belonging
	// to another tenant matches nothing, so it answers ErrCommentNotFound rather
	// than the row -- which is the same answer an id that does not exist gets,
	// and telling the two apart is itself a leak.
	row := r.db.QueryRowContext(ctx,
		`SELECT `+commentColumns+` FROM comments WHERE id = ? AND tenant_id = ?`,
		id, data.Tenant(g))
	return r.scan(row)
}

// commentSortable is the ordering allowlist. A sort field is a column name, and a
// column name taken from the request is injection through a door nobody watches.
var commentSortable = map[string]string{
	"":           "created_at",
	"created_at": "created_at",
	"post_id":    "post_id",
	"author":     "author",
	"body":       "body",
}

// List returns a page of comments in the grant's tenant.
//
// Pagination is keyset based: OFFSET grows more expensive with every page and
// skips rows when data changes underneath it.
func (r *CommentRepository) List(ctx context.Context, g security.Grant, q data.Query) ([]models.Comment, error) {
	// CommentList, not CommentView. The specification can grant them to
	// different roles -- "a support agent may open the record it was given, but
	// may not page through every record there is" -- and checking view here made
	// the list permission decorative: whoever could read one could read all of them.
	if err := g.Check(policies.CommentList); err != nil {
		return nil, err
	}
	column, ok := commentSortable[q.Sort]
	if !ok {
		return nil, fmt.Errorf("%w: %q", models.ErrCommentSort, q.Sort)
	}

	limit := q.Limit
	switch {
	case limit <= 0:
		limit = commentDefaultLimit
	case limit > commentMaxLimit:
		limit = commentMaxLimit
	}

	query := `SELECT ` + commentColumns + ` FROM comments WHERE tenant_id = ?`
	args := []any{data.Tenant(g)}
	if q.Cursor != "" {
		// The predicate names the column the ORDER BY names, and it is scoped
		// like the outer query: a cursor is the id of the last row read, it
		// arrives from the client, and a subquery that resolved it without the
		// tenant would let an id from another tenant decide where this page
		// starts.
		query += ` AND (` + column + ` > (SELECT ` + column + ` FROM comments WHERE id = ? AND tenant_id = ?)
		            OR (` + column + ` = (SELECT ` + column + ` FROM comments WHERE id = ? AND tenant_id = ?) AND id > ?))`
		args = append(args, q.Cursor, args[0], q.Cursor, args[0], q.Cursor)
	}
	query += ` ORDER BY ` + column + `, id LIMIT ?`
	args = append(args, limit)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.Comment
	for rows.Next() {
		co, err := r.scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, co)
	}
	return out, rows.Err()
}

// ForPost is the thread of one post, oldest first.
//
// A method of its own rather than a Filter on data.Query. data.Query has no
// filter, and adding one back would be a query builder -- which is the one thing
// a repository exists to avoid. A predicate the application needs is a method
// here, with its SQL visible and its parameters bound.
func (r *CommentRepository) ForPost(ctx context.Context, g security.Grant, postID string) ([]models.Comment, error) {
	if err := g.Check(policies.CommentList); err != nil {
		return nil, err
	}

	const query = `SELECT ` + commentColumns + `
		FROM comments
		WHERE tenant_id = ? AND post_id = ?
		ORDER BY created_at, id
		LIMIT ?`

	// The post id arrives from the address bar, so the tenant is scoped as well
	// as the thread: without it, a post id from another tenant would return its
	// conversation.
	rows, err := r.db.QueryContext(ctx, query, data.Tenant(g), postID, commentMaxLimit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.Comment
	for rows.Next() {
		co, err := r.scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, co)
	}
	return out, rows.Err()
}

// Create inserts the comment and returns it as stored.
func (r *CommentRepository) Create(ctx context.Context, g security.Grant, co models.Comment) (models.Comment, error) {
	if err := g.Check(policies.CommentCreate); err != nil {
		return models.Comment{}, err
	}

	var err error
	if co.ID == "" {
		// The id comes from the application, not from a database default:
		// gen_random_uuid, UUID() and randomblob are three spellings of one idea,
		// and depending on any of them would tie the schema to one engine.
		if co.ID, err = data.NewID(); err != nil {
			return models.Comment{}, err
		}
	}
	// Overwritten rather than trusted. Whatever the caller put in the field, the
	// row is filed under the tenant that was authorized -- a candidate arrives
	// from a request body, and a request that could choose its tenant could
	// write into somebody else's.
	co.TenantID = data.Tenant(g)
	co.CreatedAt = time.Now().UTC()

	// The relationship is scoped in the same statement as the insert. A plain
	// foreign key on post_id only proves that the id exists somewhere; it would
	// accept a post of another tenant and leave this tenant holding a comment
	// attached to an aggregate it cannot read. EXISTS answers both facts without
	// a check-then-insert race between two statements.
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO comments (`+commentColumns+`)
		 SELECT ?, ?, ?, ?, ?, ?, ?
		 WHERE EXISTS (SELECT 1 FROM posts WHERE id = ? AND tenant_id = ?)`,
		co.ID, co.TenantID, co.PostId, co.Author, co.Body, co.Approved, co.CreatedAt,
		co.PostId, data.Tenant(g))
	if err != nil {
		if r.conflict(err) {
			return models.Comment{}, models.ErrCommentConflict
		}
		return models.Comment{}, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return models.Comment{}, err
	}
	if n == 0 {
		// Missing and belonging to another tenant deliberately share an answer:
		// telling the caller which one happened confirms another tenant's row.
		return models.Comment{}, models.ErrPostNotFound
	}
	return co, nil
}

// Update writes the mutable fields. The tenant is not one of them: moving a row
// between tenants is not an update, it is a migration.
func (r *CommentRepository) Update(ctx context.Context, g security.Grant, co models.Comment) (models.Comment, error) {
	if err := g.Check(policies.CommentUpdate); err != nil {
		return models.Comment{}, err
	}

	res, err := r.db.ExecContext(ctx,
		`UPDATE comments SET post_id = ?, author = ?, body = ?, approved = ?
		 WHERE id = ? AND tenant_id = ?
		   AND EXISTS (SELECT 1 FROM posts WHERE id = ? AND tenant_id = ?)`,
		co.PostId, co.Author, co.Body, co.Approved, co.ID, data.Tenant(g),
		co.PostId, data.Tenant(g))
	if err != nil {
		if r.conflict(err) {
			return models.Comment{}, models.ErrCommentConflict
		}
		return models.Comment{}, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return models.Comment{}, err
	}
	if n == 0 {
		// Classify after the atomic write rather than checking before it. The
		// UPDATE remains the operation that enforces both relationships, while
		// this read preserves the repository's not-found contract and accepts
		// drivers that report zero for a matched no-op update.
		if err := r.classifyUpdateMiss(ctx, g, co.ID, co.PostId); err != nil {
			return models.Comment{}, err
		}
	}
	return co, nil
}

// classifyUpdateMiss distinguishes the two tenant-scoped reasons the guarded
// update can affect no row. A row in another tenant is deliberately identical
// to a missing row, so neither existence check can disclose it.
func (r *CommentRepository) classifyUpdateMiss(ctx context.Context, g security.Grant, commentID, postID string) error {
	var commentExists, postExists int
	tenant := data.Tenant(g)
	err := r.db.QueryRowContext(ctx,
		`SELECT
			CASE WHEN EXISTS (SELECT 1 FROM comments WHERE id = ? AND tenant_id = ?) THEN 1 ELSE 0 END,
			CASE WHEN EXISTS (SELECT 1 FROM posts WHERE id = ? AND tenant_id = ?) THEN 1 ELSE 0 END`,
		commentID, tenant, postID, tenant,
	).Scan(&commentExists, &postExists)
	if err != nil {
		return err
	}
	if commentExists == 0 {
		return models.ErrCommentNotFound
	}
	if postExists == 0 {
		return models.ErrPostNotFound
	}
	return nil
}

// Delete removes one comment within the grant's tenant.
func (r *CommentRepository) Delete(ctx context.Context, g security.Grant, id string) error {
	if err := g.Check(policies.CommentDelete); err != nil {
		return err
	}
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM comments WHERE id = ? AND tenant_id = ?`, id, data.Tenant(g))
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return models.ErrCommentNotFound
	}
	return nil
}

// Health reports whether the repository can reach its storage.
//
// Nothing calls it out of the box. It is here so AppServiceProvider can
// implement kernel.Health over the repositories it owns, which is what puts this
// table on /_arandu/health and on the error page's diagnosis.
func (r *CommentRepository) Health(ctx context.Context) error { return r.db.PingContext(ctx) }

// scan reads one row.
//
// It takes the interface inline rather than a named one, and it is a method
// rather than a function, for the same reason as the helpers below: every
// repository of the application shares this package, and a second module
// declaring rowScanner would not compile.
func (r *CommentRepository) scan(row interface{ Scan(dest ...any) error }) (models.Comment, error) {
	var (
		co     models.Comment
		tenant sql.NullString
	)
	err := row.Scan(&co.ID, &tenant, &co.PostId, &co.Author, &co.Body, &co.Approved, &co.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return models.Comment{}, models.ErrCommentNotFound
	}
	if err != nil {
		return models.Comment{}, err
	}
	// The column arrived in a later migration and is nullable, so a plain
	// *string scan is an error on every row written before it. Those rows read
	// back with an empty tenant, which no Grant carries, so no query returns
	// them.
	co.TenantID = tenant.String
	return co, nil
}

// normalize lowercases and trims, which is what keeps a plain UNIQUE index
// case-insensitive on every engine.
func (r *CommentRepository) normalize(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// conflict recognizes a duplicate key across engines by message, which is the
// price of not importing a driver into a repository.
func (r *CommentRepository) conflict(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique constraint") ||
		strings.Contains(msg, "duplicate key") ||
		strings.Contains(msg, "duplicate entry")
}

// arandu:begin custom
// Queries beyond the five above go here, and survive regeneration. Keep the
// g.Check as the first line of each one.

// PublicForPost is the thread as a reader sees it.
//
// Approved, plus anything the reader wrote themselves -- which is what lets
// somebody see their own comment marked "awaiting review" instead of watching it
// vanish, and is the single most common complaint about a moderated thread.
//
// reader is empty for somebody with no account, and an empty string matches
// nothing: the author column holds a subject id and is never blank. So the
// anonymous case needs no second query and no branch.
//
// The predicate is in the SQL and not in Go. A filter applied after the rows are
// read is a filter somebody removes while tidying, and by then the pending
// comments are already in memory next to the ones being rendered.
func (r *CommentRepository) PublicForPost(ctx context.Context, g security.Grant, postID, reader string) ([]models.Comment, error) {
	if err := g.Check(policies.CommentPublicList); err != nil {
		return nil, err
	}

	const query = `SELECT ` + commentColumns + `
		FROM comments
		WHERE tenant_id = ? AND post_id = ? AND (approved = ? OR author = ?)
		ORDER BY created_at, id
		LIMIT ?`

	// The tenant is the outermost predicate, ahead of the OR. Inside a
	// parenthesised OR it would apply to one branch and not the other, and the
	// branch that lost it -- `author = ?` -- is the one that returns rows
	// nobody has approved.
	rows, err := r.db.QueryContext(ctx, query, data.Tenant(g), postID, true, reader, commentMaxLimit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.Comment
	for rows.Next() {
		co, err := r.scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, co)
	}
	return out, rows.Err()
}

// arandu:end custom
