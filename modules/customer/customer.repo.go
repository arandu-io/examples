package customer

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/security"
)

// Errors this repository returns, so callers never import database/sql to
// compare against sql.ErrNoRows.
var (
	ErrNotFound    = errors.New("customer: not found")
	ErrEmailTaken  = errors.New("customer: email already registered in this tenant")
	ErrNoDocument  = errors.New("customer: document is required")
	ErrSortNotList = errors.New("customer: sort field not allowed")
)

// Pagination bounds. A request that asks for everything gets the maximum, never
// everything: an unbounded query is how one page load takes a database down.
const (
	defaultLimit = 50
	maxLimit     = 200
)

// Repo is the only door to the customers table.
//
// Every method starts with g.Check. The Grant is required by the signature, so
// forgetting it is a compile error, and the check proves the grant was issued
// for this exact action, which catches copy-paste between methods.
//
// The SQL uses "?" placeholders and types every supported database spells the
// same way, so these statements run unchanged on SQLite and PostgreSQL.
type Repo struct {
	db *data.DB
}

// NewRepo returns a repository over an instrumented handle.
func NewRepo(db *data.DB) *Repo { return &Repo{db: db} }

// Compile-time proof of the contract. If data.Repository changes, this line
// breaks before any caller does.
var _ data.Repository[Customer, string] = (*Repo)(nil)

const columns = `id, tenant_id, name, email, document, created_at`

// Find returns one customer, scoped to the grant's tenant.
func (r *Repo) Find(ctx context.Context, g security.Grant, id string) (Customer, error) {
	if err := g.Check(ActionView); err != nil {
		return Customer{}, err
	}
	// The tenant comes from the Grant, never from the request.
	row := r.db.QueryRowContext(ctx,
		`SELECT `+columns+` FROM customers WHERE id = ? AND tenant_id = ?`,
		id, data.Tenant(g))
	return scan(row)
}

// List returns a page of customers in the grant's tenant.
//
// Pagination is keyset based: OFFSET grows more expensive with every page and
// skips rows when data changes underneath it.
func (r *Repo) List(ctx context.Context, g security.Grant, q data.Query) ([]Customer, error) {
	if err := g.Check(ActionView); err != nil {
		return nil, err
	}
	column, ok := sortable[q.Sort]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrSortNotList, q.Sort)
	}

	limit := q.Limit
	switch {
	case limit <= 0:
		limit = defaultLimit
	case limit > maxLimit:
		limit = maxLimit
	}

	query := `SELECT ` + columns + ` FROM customers WHERE tenant_id = ?`
	args := []any{data.Tenant(g)}
	if q.Cursor != "" {
		query += ` AND (created_at > (SELECT created_at FROM customers WHERE id = ? AND tenant_id = ?)
		            OR (created_at = (SELECT created_at FROM customers WHERE id = ? AND tenant_id = ?) AND id > ?))`
		args = append(args, q.Cursor, args[0], q.Cursor, args[0], q.Cursor)
	}
	query += ` ORDER BY ` + column + `, id LIMIT ?`
	args = append(args, limit)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Customer
	for rows.Next() {
		c, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// sortable is the ordering allowlist. A sort field is a column name, and a column
// name taken from the request is injection through a door nobody watches -- so
// only these values are accepted, and the SQL never interpolates the raw input.
var sortable = map[string]string{
	"":           "created_at",
	"name":       "name",
	"email":      "email",
	"created_at": "created_at",
}

// Create inserts the customer and returns it as stored.
//
// The id and the timestamp come from the application rather than from a database
// default: gen_random_uuid, UUID() and randomblob are three spellings of one
// idea, and depending on any of them would tie the schema to one engine.
func (r *Repo) Create(ctx context.Context, g security.Grant, c Customer) (Customer, error) {
	if err := g.Check(ActionCreate); err != nil {
		return Customer{}, err
	}
	if strings.TrimSpace(c.Document) == "" {
		return Customer{}, ErrNoDocument
	}

	var err error
	if c.ID == "" {
		if c.ID, err = NewID(); err != nil {
			return Customer{}, err
		}
	}
	c.TenantID = data.Tenant(g)
	c.Email = normalizeEmail(c.Email)
	c.CreatedAt = time.Now().UTC()

	_, err = r.db.ExecContext(ctx,
		`INSERT INTO customers (`+columns+`) VALUES (?, ?, ?, ?, ?, ?)`,
		c.ID, c.TenantID, c.Name, c.Email, c.Document, c.CreatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return Customer{}, ErrEmailTaken
		}
		return Customer{}, err
	}
	return c, nil
}

// Update writes the mutable fields. The tenant is not one of them: moving a
// customer between tenants is not an update, it is a migration.
func (r *Repo) Update(ctx context.Context, g security.Grant, c Customer) (Customer, error) {
	if err := g.Check(ActionUpdate); err != nil {
		return Customer{}, err
	}
	c.Email = normalizeEmail(c.Email)

	res, err := r.db.ExecContext(ctx,
		`UPDATE customers SET name = ?, email = ?, document = ? WHERE id = ? AND tenant_id = ?`,
		c.Name, c.Email, c.Document, c.ID, data.Tenant(g))
	if err != nil {
		if isUniqueViolation(err) {
			return Customer{}, ErrEmailTaken
		}
		return Customer{}, err
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return Customer{}, ErrNotFound
	}
	return c, nil
}

// Delete removes one customer within the grant's tenant.
func (r *Repo) Delete(ctx context.Context, g security.Grant, id string) error {
	if err := g.Check(ActionDelete); err != nil {
		return err
	}
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM customers WHERE id = ? AND tenant_id = ?`, id, data.Tenant(g))
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrNotFound
	}
	return nil
}

type rowScanner interface{ Scan(dest ...any) error }

func scan(row rowScanner) (Customer, error) {
	var c Customer
	err := row.Scan(&c.ID, &c.TenantID, &c.Name, &c.Email, &c.Document, &c.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Customer{}, ErrNotFound
	}
	if err != nil {
		return Customer{}, err
	}
	return c, nil
}

// normalizeEmail keeps a plain UNIQUE index case-insensitive on every engine,
// without a functional index in Postgres and a collation in MySQL.
func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// NewID returns a version 4 UUID as text.
func NewID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("customer: reading random bytes: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

// isUniqueViolation recognizes a duplicate key across engines by message. It is
// ugly, and it is the price of not importing a driver: SQLite says "UNIQUE
// constraint failed", Postgres says "duplicate key value violates unique
// constraint", MySQL says "Duplicate entry".
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique constraint") ||
		strings.Contains(msg, "duplicate key") ||
		strings.Contains(msg, "duplicate entry")
}
