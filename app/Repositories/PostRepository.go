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

// Pagination bounds for posts. A request that asks for everything gets the
// maximum, never everything: an unbounded query is how one page load takes a
// database down.
const (
	postDefaultLimit = 50
	postMaxLimit     = 200
)

// PostRepository is the only door to the posts table.
//
// Every method starts with g.Check. The Grant is required by the signature, so
// forgetting it is a compile error, and the check proves the grant was issued for
// this exact action -- which catches copy-paste between methods.
//
// The SQL uses "?" placeholders and types every supported database shares, so
// these statements run unchanged on SQLite and PostgreSQL.
type PostRepository struct {
	db *data.DB
}

// NewPostRepository returns a repository over an instrumented handle.
func NewPostRepository(db *data.DB) *PostRepository { return &PostRepository{db: db} }

// Compile-time proof of the contract.
var _ data.Repository[models.Post, string] = (*PostRepository)(nil)

const postColumns = `id, tenant_id, title, slug, body, category_id, views, published_at, created_at`

// Find returns one post by id, scoped to the grant's tenant.
func (r *PostRepository) Find(ctx context.Context, g security.Grant, id string) (models.Post, error) {
	if err := g.Check(policies.PostView); err != nil {
		return models.Post{}, err
	}
	// The tenant comes from the Grant, never from the request. An id belonging
	// to another tenant matches nothing, so it answers ErrPostNotFound rather
	// than the row -- which is the same answer an id that does not exist gets,
	// and telling the two apart is itself a leak.
	row := r.db.QueryRowContext(ctx,
		`SELECT `+postColumns+` FROM posts WHERE id = ? AND tenant_id = ?`,
		id, data.Tenant(g))
	return r.scan(row)
}

// postSortable is the ordering allowlist. A sort field is a column name, and a
// column name taken from the request is injection through a door nobody watches.
var postSortable = map[string]string{
	"":             "created_at",
	"created_at":   "created_at",
	"title":        "title",
	"slug":         "slug",
	"body":         "body",
	"published_at": "published_at",
}

// List returns a page of posts in the grant's tenant.
//
// Pagination is keyset based: OFFSET grows more expensive with every page and
// skips rows when data changes underneath it.
func (r *PostRepository) List(ctx context.Context, g security.Grant, q data.Query) ([]models.Post, error) {
	// PostList, not PostView. The specification can grant them to
	// different roles -- "a support agent may open the record it was given, but
	// may not page through every record there is" -- and checking view here made
	// the list permission decorative: whoever could read one could read all of them.
	if err := g.Check(policies.PostList); err != nil {
		return nil, err
	}
	column, ok := postSortable[q.Sort]
	if !ok {
		return nil, fmt.Errorf("%w: %q", models.ErrPostSort, q.Sort)
	}

	limit := q.Limit
	switch {
	case limit <= 0:
		limit = postDefaultLimit
	case limit > postMaxLimit:
		limit = postMaxLimit
	}

	query := `SELECT ` + postColumns + ` FROM posts WHERE tenant_id = ?`
	args := []any{data.Tenant(g)}
	if q.Cursor != "" {
		// The predicate names the column the ORDER BY names, and it is scoped
		// like the outer query: a cursor is the id of the last row read, it
		// arrives from the client, and a subquery that resolved it without the
		// tenant would let an id from another tenant decide where this page
		// starts.
		query += ` AND (` + column + ` > (SELECT ` + column + ` FROM posts WHERE id = ? AND tenant_id = ?)
		            OR (` + column + ` = (SELECT ` + column + ` FROM posts WHERE id = ? AND tenant_id = ?) AND id > ?))`
		args = append(args, q.Cursor, args[0], q.Cursor, args[0], q.Cursor)
	}
	query += ` ORDER BY ` + column + `, id LIMIT ?`
	args = append(args, limit)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.Post
	for rows.Next() {
		p, err := r.scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Published is the public listing of the grant's tenant: what is out, newest
// first.
//
// A query of its own rather than List with a filter. The predicate is what makes
// a draft unreachable rather than merely unlisted -- a filter applied after the
// rows are read is one somebody removes while tidying, and the row is already
// in memory when they do.
//
// It checks PostPublicList, so a Grant issued for PostList is refused here. That
// is the point of the two actions being two: this method answers a narrower
// question, and the narrower permission is what may ask it.
func (r *PostRepository) Published(ctx context.Context, g security.Grant, limit int) ([]models.Post, error) {
	if err := g.Check(policies.PostPublicList); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > postMaxLimit {
		limit = postMaxLimit
	}

	const query = `SELECT ` + postColumns + `
		FROM posts
		WHERE tenant_id = ? AND published_at IS NOT NULL AND published_at > ?
		ORDER BY published_at DESC, id
		LIMIT ?`

	// IS NOT NULL is not enough, and finding that out cost a draft in the
	// sitemap. PublishedAt is a time.Time, not a pointer, so "not published" is
	// the zero value -- which the driver writes as 0001-01-01, a real timestamp
	// that is very much not null. The predicate has to say what it means.
	rows, err := r.db.QueryContext(ctx, query, data.Tenant(g), time.Time{}, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.Post
	for rows.Next() {
		p, err := r.scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Create inserts the post and returns it as stored.
func (r *PostRepository) Create(ctx context.Context, g security.Grant, p models.Post) (models.Post, error) {
	if err := g.Check(policies.PostCreate); err != nil {
		return models.Post{}, err
	}

	var err error
	if p.ID == "" {
		// The id comes from the application, not from a database default:
		// gen_random_uuid, UUID() and randomblob are three spellings of one idea,
		// and depending on any of them would tie the schema to one engine.
		if p.ID, err = data.NewID(); err != nil {
			return models.Post{}, err
		}
	}
	// Overwritten rather than trusted. Whatever the caller put in the field, the
	// row is filed under the tenant that was authorized -- a candidate arrives
	// from a request body, and a request that could choose its tenant could
	// write into somebody else's.
	p.TenantID = data.Tenant(g)
	p.CreatedAt = time.Now().UTC()

	_, err = r.db.ExecContext(ctx,
		`INSERT INTO posts (`+postColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.TenantID, p.Title, p.Slug, p.Body, nullID(p.CategoryID), p.Views, p.PublishedAt, p.CreatedAt)
	if err != nil {
		if r.conflict(err) {
			return models.Post{}, models.ErrPostConflict
		}
		return models.Post{}, err
	}
	return p, nil
}

// Update writes the mutable fields. The tenant is not one of them: moving a row
// between tenants is not an update, it is a migration.
func (r *PostRepository) Update(ctx context.Context, g security.Grant, p models.Post) (models.Post, error) {
	if err := g.Check(policies.PostUpdate); err != nil {
		return models.Post{}, err
	}

	res, err := r.db.ExecContext(ctx,
		`UPDATE posts SET title = ?, slug = ?, body = ?, category_id = ?, published_at = ?
		 WHERE id = ? AND tenant_id = ?`,
		p.Title, p.Slug, p.Body, nullID(p.CategoryID), p.PublishedAt, p.ID, data.Tenant(g))
	if err != nil {
		if r.conflict(err) {
			return models.Post{}, models.ErrPostConflict
		}
		return models.Post{}, err
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return models.Post{}, models.ErrPostNotFound
	}
	return p, nil
}

// Delete removes one post within the grant's tenant.
func (r *PostRepository) Delete(ctx context.Context, g security.Grant, id string) error {
	if err := g.Check(policies.PostDelete); err != nil {
		return err
	}
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM posts WHERE id = ? AND tenant_id = ?`, id, data.Tenant(g))
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return models.ErrPostNotFound
	}
	return nil
}

// Health reports whether the repository can reach its storage.
//
// Nothing calls it out of the box. It is here so AppServiceProvider can
// implement kernel.Health over the repositories it owns, which is what puts this
// table on /_arandu/health and on the error page's diagnosis.
func (r *PostRepository) Health(ctx context.Context) error { return r.db.PingContext(ctx) }

// scan reads one row.
//
// It takes the interface inline rather than a named one, and it is a method
// rather than a function, for the same reason as the helpers below: every
// repository of the application shares this package, and a second module
// declaring rowScanner would not compile.
func (r *PostRepository) scan(row interface{ Scan(dest ...any) error }) (models.Post, error) {
	var (
		p        models.Post
		tenant   sql.NullString
		category sql.NullString
		views    sql.NullInt64
	)
	err := row.Scan(&p.ID, &tenant, &p.Title, &p.Slug, &p.Body, &category, &views,
		&p.PublishedAt, &p.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return models.Post{}, models.ErrPostNotFound
	}
	if err != nil {
		return models.Post{}, err
	}
	// Read through sql.NullString for the same reason as the two below: the
	// column arrived in a later migration and is nullable, so a plain *string
	// scan is an error on every row written before it. Those rows read back with
	// an empty tenant, which no Grant carries, so no query returns them.
	p.TenantID = tenant.String
	// The column arrived in a later migration, so every post written before it
	// has NULL there. Reading through sql.NullString rather than into the field
	// is what keeps those rows readable -- a plain *string scan of a NULL is an
	// error, on every post that existed before categories did.
	p.CategoryID = category.String
	// Same story as the column above: views arrived in 2026_08_09_000001 as a
	// nullable column with no default, and this scan read it straight into an int.
	// Every post written before that migration -- and every post the PREVIOUS
	// binary inserted during the rollout, because it does not know the column
	// exists -- answered "converting NULL to int is unsupported" on every read of
	// this table.
	p.Views = int(views.Int64)
	return p, nil
}

// nullID keeps an empty id out of the column as NULL rather than as "".
//
// They read back the same, so it does not matter on the way out. It matters on
// the way in: `WHERE category_id = ?` with an empty string matches the posts
// filed nowhere, and a section page built from that query would list every
// unfiled draft under whatever section was asked for.
func nullID(id string) any {
	if id == "" {
		return nil
	}
	return id
}

// normalize lowercases and trims, which is what keeps a plain UNIQUE index
// case-insensitive on every engine.
func (r *PostRepository) normalize(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// conflict recognizes a duplicate key across engines by message, which is the
// price of not importing a driver into a repository.
func (r *PostRepository) conflict(err error) bool {
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

// PublishedInCategory is the public listing narrowed to one section.
//
// A method rather than an argument on Published, because the predicate is what
// makes a draft unreachable and an optional filter is one somebody makes
// optional in the other direction. It checks the same PostPublicList: the
// question is narrower, not different.
func (r *PostRepository) PublishedInCategory(ctx context.Context, g security.Grant, categoryID string, limit int) ([]models.Post, error) {
	if err := g.Check(policies.PostPublicList); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > postMaxLimit {
		limit = postMaxLimit
	}

	const query = `SELECT ` + postColumns + `
		FROM posts
		WHERE tenant_id = ? AND category_id = ? AND published_at IS NOT NULL AND published_at > ?
		ORDER BY published_at DESC, id
		LIMIT ?`

	// The tenant is scoped as well as the section, and not instead of it. The
	// section id reaches here from a slug in the address bar, so without the
	// tenant a section id guessed from another tenant would list its posts.
	rows, err := r.db.QueryContext(ctx, query, data.Tenant(g), categoryID, time.Time{}, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.Post
	for rows.Next() {
		p, err := r.scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// CountByCategory returns how many posts each section of the grant's tenant
// holds.
//
// One query for the whole navigation rather than one per section: a sidebar with
// eight sections would otherwise be eight round trips per page, which is the
// N+1 that an ORM is usually blamed for and that hand-written SQL reproduces
// just as easily.
//
// The empty key is not returned: posts filed nowhere are not a section.
func (r *PostRepository) CountByCategory(ctx context.Context, g security.Grant) (map[string]int, error) {
	if err := g.Check(policies.PostPublicList); err != nil {
		return nil, err
	}

	const query = `SELECT category_id, COUNT(*)
		FROM posts
		WHERE tenant_id = ? AND category_id IS NOT NULL AND published_at IS NOT NULL AND published_at > ?
		GROUP BY category_id`

	// An aggregate is a read like any other. A COUNT that crossed tenants would
	// not return a single row of anybody's data and would still report how much
	// of it there is, which is the kind of leak a report is made of.
	rows, err := r.db.QueryContext(ctx, query, data.Tenant(g), time.Time{})
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]int{}
	for rows.Next() {
		var (
			id sql.NullString
			n  int
		)
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		if id.Valid {
			out[id.String] = n
		}
	}
	return out, rows.Err()
}

// IncrementViews adds one to the counter.
//
// `views = views + 1` in the statement, not read-add-write in Go: two readers
// opening the same article at the same time both read the same number, and the
// second write lands on the first, so one of the two views never happened.
//
// It checks PostView -- the same permission as reading the article, because that
// is what it is a side effect of. A separate action would be a permission
// nobody could grant without also granting the read it always follows.
func (r *PostRepository) IncrementViews(ctx context.Context, g security.Grant, id string) error {
	if err := g.Check(policies.PostView); err != nil {
		return err
	}
	_, err := r.db.ExecContext(ctx,
		`UPDATE posts SET views = views + 1 WHERE id = ? AND tenant_id = ?`,
		id, data.Tenant(g))
	return err
}

// arandu:end custom
