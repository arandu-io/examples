package invoice

import (
	"context"
	"net/http"

	"github.com/arandu-io/framework/httpx"
	"github.com/arandu-io/framework/kernel"
	"github.com/arandu-io/framework/security"
)

// SubjectResolver returns who is acting, from the session.
type SubjectResolver func(r *http.Request) (security.Subject, error)

// Module registers the invoice routes.
type Module struct {
	svc     *Service
	subject SubjectResolver
}

// New returns the module.
func New(svc *Service, subject SubjectResolver) *Module {
	return &Module{svc: svc, subject: subject}
}

// Compile-time proof of the contracts this module claims.
var (
	_ kernel.Module     = (*Module)(nil)
	_ kernel.Migratable = (*Module)(nil)
)

// Name is the module identifier.
func (m *Module) Name() string { return "invoice" }

// Routes registers the module's routes.
func (m *Module) Routes(r *httpx.Router) {
	g := r.Group("/invoices")
	g.Get("/", m.list)
	g.Get("/outstanding", m.outstanding)
}

// Migrations declares the schema this module owns.
//
// Note what is missing: there is no index on status. That is deliberate -- the
// outstanding total scans, and the debug page is supposed to say so. A framework
// that only demonstrates its diagnostics against a synthetic problem has not
// demonstrated them.
func (m *Module) Migrations() []kernel.Migration {
	return []kernel.Migration{{
		ID: "2026_07_31_000002_create_invoices_table",
		Up: `CREATE TABLE invoices (
			id           TEXT PRIMARY KEY,
			tenant_id    TEXT NOT NULL,
			customer_id  TEXT NOT NULL,
			number       TEXT NOT NULL,
			amount_cents INTEGER NOT NULL,
			status       TEXT NOT NULL,
			issued_at    TIMESTAMP NOT NULL,
			UNIQUE (tenant_id, number)
		);
		CREATE INDEX invoices_customer_idx ON invoices (customer_id, issued_at);`,
		Down: `DROP TABLE invoices;`,
	}}
}

// Service holds the business rules of the module.
type Service struct {
	repo   *Repo
	policy Policy
}

// NewService wires the module.
func NewService(repo *Repo) *Service { return &Service{repo: repo} }

// Repo exposes the repository, for the seeder and for the demo routes that need
// to issue the two listing strategies side by side.
func (s *Service) Repo() *Repo { return s.repo }

// Authorize issues a grant for an action of this module, so callers outside it
// -- the demo routes -- still go through the policy.
func (s *Service) Authorize(ctx context.Context, actor security.Subject, a security.Action) (security.Grant, error) {
	return security.Authorize(ctx, s.policy, actor, a, Invoice{})
}

// ByCustomer returns the invoices of one customer.
func (s *Service) ByCustomer(ctx context.Context, actor security.Subject, customerID string) ([]Invoice, error) {
	g, err := s.Authorize(ctx, actor, ActionView)
	if err != nil {
		return nil, err
	}
	return s.repo.ListByCustomer(ctx, g, customerID)
}

// Outstanding returns the total of open invoices, in cents.
func (s *Service) Outstanding(ctx context.Context, actor security.Subject) (int64, error) {
	g, err := s.Authorize(ctx, actor, ActionView)
	if err != nil {
		return 0, err
	}
	return s.repo.TotalOutstanding(ctx, g)
}

// EnsureSeed creates an invoice during seeding.
func (s *Service) EnsureSeed(ctx context.Context, tenant string, i Invoice) (Invoice, error) {
	return s.repo.Create(ctx, security.SystemGrant(ActionCreate, tenant), i)
}
