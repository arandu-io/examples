// Package invoice exists for one reason: it belongs to a customer, and that
// relation is what produces a real N+1.
//
// The two listing methods below are the demonstration. Both return the same
// data. One issues a query per customer, the other issues two queries in total.
// The Collector tells them apart without anyone instrumenting anything, and the
// error page names the problem instead of showing a wall of SQL.
package invoice

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/security"
)

// ErrNotFound is returned when no row matches.
var ErrNotFound = errors.New("invoice: not found")

// Actions of this module.
const (
	ActionView   security.Action = "invoice.view"
	ActionCreate security.Action = "invoice.create"
)

// Status of an invoice.
const (
	StatusOpen  = "open"
	StatusPaid  = "paid"
	StatusVoid  = "void"
	centsInUnit = 100
)

// Invoice is the entity.
type Invoice struct {
	ID          string
	TenantID    string
	CustomerID  string
	Number      string
	AmountCents int64
	Status      string
	IssuedAt    time.Time
}

// Amount returns the value in currency units, for display.
//
// The money is stored in cents as an integer, never as a float: 0.1 + 0.2 is not
// 0.3 in binary floating point, and an invoice that is off by a cent is a support
// ticket that costs more than this comment.
func (i Invoice) Amount() float64 { return float64(i.AmountCents) / centsInUnit }

// Policy decides who may see and create invoices.
type Policy struct{}

// Can decides whether the subject may perform the action on the invoice.
func (Policy) Can(ctx context.Context, s security.Subject, a security.Action, i Invoice) error {
	if i.ID != "" && i.TenantID != s.Tenant {
		return fmt.Errorf("invoice belongs to another tenant")
	}
	switch a {
	case ActionView:
		if s.HasRole("admin") || s.HasRole("sales") || s.HasRole("support") {
			return nil
		}
	case ActionCreate:
		if s.HasRole("admin") || s.HasRole("sales") {
			return nil
		}
	}
	return fmt.Errorf("role insufficient for %s", a)
}

// Repo is the only door to the invoices table.
type Repo struct {
	db *data.DB
}

// NewRepo returns a repository over an instrumented handle.
func NewRepo(db *data.DB) *Repo { return &Repo{db: db} }

const columns = `id, tenant_id, customer_id, number, amount_cents, status, issued_at`

// ListByCustomer returns the invoices of one customer.
//
// Called in a loop -- which is what /demo/n-plus-one does -- this is the classic
// N+1. The method itself is correct: the mistake is always at the call site.
//
// Worth knowing when you read the debug page: the Origin column points here, at
// the repository, because that is the frame that issued the statement. The loop
// that called it six times is in the Stack section. Naming the loop directly
// would need the Collector to keep a few frames instead of one, which is a phase
// 3 improvement to the console, not something to fake here.
func (r *Repo) ListByCustomer(ctx context.Context, g security.Grant, customerID string) ([]Invoice, error) {
	if err := g.Check(ActionView); err != nil {
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+columns+` FROM invoices WHERE customer_id = ? AND tenant_id = ? ORDER BY issued_at`,
		customerID, data.Tenant(g))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collect(rows)
}

// ListByCustomers returns the invoices of many customers in one query.
//
// The placeholder list is built from the number of ids, never from their values,
// so this stays a parameterized query -- the count is structure, the ids are
// data. Interpolating the ids themselves is how a batch fetch becomes an
// injection point.
func (r *Repo) ListByCustomers(ctx context.Context, g security.Grant, customerIDs []string) (map[string][]Invoice, error) {
	if err := g.Check(ActionView); err != nil {
		return nil, err
	}
	if len(customerIDs) == 0 {
		return map[string][]Invoice{}, nil
	}

	placeholders := make([]byte, 0, len(customerIDs)*2)
	args := make([]any, 0, len(customerIDs)+1)
	for i, id := range customerIDs {
		if i > 0 {
			placeholders = append(placeholders, ',')
		}
		placeholders = append(placeholders, '?')
		args = append(args, id)
	}
	args = append(args, data.Tenant(g))

	rows, err := r.db.QueryContext(ctx,
		`SELECT `+columns+` FROM invoices WHERE customer_id IN (`+string(placeholders)+`) AND tenant_id = ?
		 ORDER BY customer_id, issued_at`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	all, err := collect(rows)
	if err != nil {
		return nil, err
	}

	byCustomer := make(map[string][]Invoice, len(customerIDs))
	for _, i := range all {
		byCustomer[i.CustomerID] = append(byCustomer[i.CustomerID], i)
	}
	return byCustomer, nil
}

// Create inserts an invoice.
func (r *Repo) Create(ctx context.Context, g security.Grant, i Invoice) (Invoice, error) {
	if err := g.Check(ActionCreate); err != nil {
		return Invoice{}, err
	}

	var err error
	if i.ID == "" {
		if i.ID, err = newID(); err != nil {
			return Invoice{}, err
		}
	}
	i.TenantID = data.Tenant(g)
	if i.Status == "" {
		i.Status = StatusOpen
	}
	if i.IssuedAt.IsZero() {
		i.IssuedAt = time.Now().UTC()
	}

	_, err = r.db.ExecContext(ctx,
		`INSERT INTO invoices (`+columns+`) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		i.ID, i.TenantID, i.CustomerID, i.Number, i.AmountCents, i.Status, i.IssuedAt)
	if err != nil {
		return Invoice{}, err
	}
	return i, nil
}

// TotalOutstanding sums the open invoices of the tenant.
//
// It exists to demonstrate the slow query hint: the sum runs over a column with
// no index, on purpose, and the debug page points at it.
func (r *Repo) TotalOutstanding(ctx context.Context, g security.Grant) (int64, error) {
	if err := g.Check(ActionView); err != nil {
		return 0, err
	}
	var total sql.NullInt64
	err := r.db.QueryRowContext(ctx,
		`SELECT SUM(amount_cents) FROM invoices WHERE tenant_id = ? AND status = ?`,
		data.Tenant(g), StatusOpen).Scan(&total)
	if err != nil {
		return 0, err
	}
	return total.Int64, nil
}

func collect(rows *sql.Rows) ([]Invoice, error) {
	var out []Invoice
	for rows.Next() {
		var i Invoice
		if err := rows.Scan(&i.ID, &i.TenantID, &i.CustomerID, &i.Number, &i.AmountCents, &i.Status, &i.IssuedAt); err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

func newID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("invoice: reading random bytes: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}
