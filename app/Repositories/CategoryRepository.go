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

// Pagination bounds for categories. A request that asks for everything gets the
// maximum, never everything: an unbounded query is how one page load takes a
// database down.
const (
	categoryDefaultLimit = 50
	categoryMaxLimit     = 200
)

// CategoryRepository is the only door to the categories table.
//
// Every method starts with g.Check. The Grant is required by the signature, so
// forgetting it is a compile error, and the check proves the grant was issued for
// this exact action -- which catches copy-paste between methods.
//
// The SQL uses "?" placeholders and types every supported database shares, so
// these statements run unchanged on SQLite and PostgreSQL.
type CategoryRepository struct {
	db *data.DB
}

// NewCategoryRepository returns a repository over an instrumented handle.
func NewCategoryRepository(db *data.DB) *CategoryRepository { return &CategoryRepository{db: db} }

// Compile-time proof of the contract.
var _ data.Repository[models.Category, string] = (*CategoryRepository)(nil)

const categoryColumns = `id, tenant_id, name, slug, description, created_at`

// Find returns one category by id, scoped to the grant's tenant.
func (r *CategoryRepository) Find(ctx context.Context, g security.Grant, id string) (models.Category, error) {
	if err := g.Check(policies.CategoryView); err != nil {
		return models.Category{}, err
	}
	// The tenant comes from the Grant, never from the request. An id belonging
	// to another tenant matches nothing, so it answers ErrCategoryNotFound
	// rather than the row -- which is the same answer an id that does not exist
	// gets, and telling the two apart is itself a leak.
	row := r.db.QueryRowContext(ctx,
		`SELECT `+categoryColumns+` FROM categories WHERE id = ? AND tenant_id = ?`,
		id, data.Tenant(g))
	return r.scan(row)
}

// categorySortable is the ordering allowlist. A sort field is a column name, and a
// column name taken from the request is injection through a door nobody watches.
var categorySortable = map[string]string{
	"":            "created_at",
	"created_at":  "created_at",
	"name":        "name",
	"slug":        "slug",
	"description": "description",
}

// List returns a page of categories in the grant's tenant.
//
// Pagination is keyset based: OFFSET grows more expensive with every page and
// skips rows when data changes underneath it.
func (r *CategoryRepository) List(ctx context.Context, g security.Grant, q data.Query) ([]models.Category, error) {
	// CategoryList, not CategoryView. The specification can grant them to
	// different roles -- "a support agent may open the record it was given, but
	// may not page through every record there is" -- and checking view here made
	// the list permission decorative: whoever could read one could read all of them.
	if err := g.Check(policies.CategoryList); err != nil {
		return nil, err
	}
	column, ok := categorySortable[q.Sort]
	if !ok {
		return nil, fmt.Errorf("%w: %q", models.ErrCategorySort, q.Sort)
	}

	limit := q.Limit
	switch {
	case limit <= 0:
		limit = categoryDefaultLimit
	case limit > categoryMaxLimit:
		limit = categoryMaxLimit
	}

	query := `SELECT ` + categoryColumns + ` FROM categories WHERE tenant_id = ?`
	args := []any{data.Tenant(g)}
	if q.Cursor != "" {
		// The predicate names the column the ORDER BY names, and it is scoped
		// like the outer query: a cursor is the id of the last row read, it
		// arrives from the client, and a subquery that resolved it without the
		// tenant would let an id from another tenant decide where this page
		// starts.
		query += ` AND (` + column + ` > (SELECT ` + column + ` FROM categories WHERE id = ? AND tenant_id = ?)
		            OR (` + column + ` = (SELECT ` + column + ` FROM categories WHERE id = ? AND tenant_id = ?) AND id > ?))`
		args = append(args, q.Cursor, args[0], q.Cursor, args[0], q.Cursor)
	}
	query += ` ORDER BY ` + column + `, id LIMIT ?`
	args = append(args, limit)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.Category
	for rows.Next() {
		ca, err := r.scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, ca)
	}
	return out, rows.Err()
}

// Create inserts the category and returns it as stored.
func (r *CategoryRepository) Create(ctx context.Context, g security.Grant, ca models.Category) (models.Category, error) {
	if err := g.Check(policies.CategoryCreate); err != nil {
		return models.Category{}, err
	}

	var err error
	if ca.ID == "" {
		// The id comes from the application, not from a database default:
		// gen_random_uuid, UUID() and randomblob are three spellings of one idea,
		// and depending on any of them would tie the schema to one engine.
		if ca.ID, err = data.NewID(); err != nil {
			return models.Category{}, err
		}
	}
	// Overwritten rather than trusted. Whatever the caller put in the field, the
	// row is filed under the tenant that was authorized -- a candidate arrives
	// from a request body, and a request that could choose its tenant could
	// write into somebody else's.
	ca.TenantID = data.Tenant(g)
	ca.CreatedAt = time.Now().UTC()

	_, err = r.db.ExecContext(ctx,
		`INSERT INTO categories (`+categoryColumns+`) VALUES (?, ?, ?, ?, ?, ?)`,
		ca.ID, ca.TenantID, ca.Name, ca.Slug, ca.Description, ca.CreatedAt)
	if err != nil {
		if r.conflict(err) {
			return models.Category{}, models.ErrCategoryConflict
		}
		return models.Category{}, err
	}
	return ca, nil
}

// Update writes the mutable fields. The tenant is not one of them: moving a row
// between tenants is not an update, it is a migration.
func (r *CategoryRepository) Update(ctx context.Context, g security.Grant, ca models.Category) (models.Category, error) {
	if err := g.Check(policies.CategoryUpdate); err != nil {
		return models.Category{}, err
	}

	res, err := r.db.ExecContext(ctx,
		`UPDATE categories SET name = ?, slug = ?, description = ?
		 WHERE id = ? AND tenant_id = ?`,
		ca.Name, ca.Slug, ca.Description, ca.ID, data.Tenant(g))
	if err != nil {
		if r.conflict(err) {
			return models.Category{}, models.ErrCategoryConflict
		}
		return models.Category{}, err
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return models.Category{}, models.ErrCategoryNotFound
	}
	return ca, nil
}

// Delete removes one category within the grant's tenant.
func (r *CategoryRepository) Delete(ctx context.Context, g security.Grant, id string) error {
	if err := g.Check(policies.CategoryDelete); err != nil {
		return err
	}
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM categories WHERE id = ? AND tenant_id = ?`, id, data.Tenant(g))
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return models.ErrCategoryNotFound
	}
	return nil
}

// Health reports whether the repository can reach its storage.
//
// Nothing calls it out of the box. It is here so AppServiceProvider can
// implement kernel.Health over the repositories it owns, which is what puts this
// table on /_arandu/health and on the error page's diagnosis.
func (r *CategoryRepository) Health(ctx context.Context) error { return r.db.PingContext(ctx) }

// scan reads one row.
//
// It takes the interface inline rather than a named one, and it is a method
// rather than a function, for the same reason as the helpers below: every
// repository of the application shares this package, and a second module
// declaring rowScanner would not compile.
func (r *CategoryRepository) scan(row interface{ Scan(dest ...any) error }) (models.Category, error) {
	var (
		ca     models.Category
		tenant sql.NullString
	)
	err := row.Scan(&ca.ID, &tenant, &ca.Name, &ca.Slug, &ca.Description, &ca.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return models.Category{}, models.ErrCategoryNotFound
	}
	if err != nil {
		return models.Category{}, err
	}
	// The column arrived in a later migration and is nullable, so a plain
	// *string scan is an error on every row written before it. Those rows read
	// back with an empty tenant, which no Grant carries, so no query returns
	// them.
	ca.TenantID = tenant.String
	return ca, nil
}

// normalize lowercases and trims, which is what keeps a plain UNIQUE index
// case-insensitive on every engine.
func (r *CategoryRepository) normalize(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// conflict recognizes a duplicate key across engines by message, which is the
// price of not importing a driver into a repository.
func (r *CategoryRepository) conflict(err error) bool {
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

// FindBySlug returns one category by the name it has in a URL.
//
// The slug is what a reader's address carries and the id is what a form posts.
// Both are unique, so this is Find with another key rather than another kind of
// lookup -- and it checks the same CategoryView, because it answers the same
// question about the same row.
func (r *CategoryRepository) FindBySlug(ctx context.Context, g security.Grant, slug string) (models.Category, error) {
	if err := g.Check(policies.CategoryView); err != nil {
		return models.Category{}, err
	}
	// A slug is unique per tenant and not globally, so the tenant is part of the
	// lookup rather than a check on the row that came back. A slug is also the
	// half of the address a reader can type, which makes this the query most
	// likely to be handed another tenant's value.
	row := r.db.QueryRowContext(ctx,
		`SELECT `+categoryColumns+` FROM categories WHERE slug = ? AND tenant_id = ?`,
		r.normalize(slug), data.Tenant(g))
	return r.scan(row)
}

// All returns every category of the grant's tenant, ordered by name.
//
// No cursor and no limit, deliberately. A blog has a dozen sections, the
// navigation shows all of them, and paging a navigation is a navigation that
// hides part of itself. The bound is the schema: if this ever returns hundreds,
// the sections are being used as tags and the answer is a different feature.
func (r *CategoryRepository) All(ctx context.Context, g security.Grant) ([]models.Category, error) {
	if err := g.Check(policies.CategoryList); err != nil {
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+categoryColumns+` FROM categories WHERE tenant_id = ? ORDER BY name, id LIMIT ?`,
		data.Tenant(g), categoryMaxLimit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.Category
	for rows.Next() {
		ca, err := r.scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, ca)
	}
	return out, rows.Err()
}

// arandu:end custom
