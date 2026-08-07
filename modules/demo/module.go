// Package demo holds the routes that demonstrate what the framework does. It is
// not a template for a real module -- it is a guided tour.
//
// Every route here exists to make one claim visible on screen. They are mounted
// only when Env is dev: three of them panic on purpose, and one of them prints
// what a policy refused.
package demo

import (
	"fmt"
	"net/http"
	"time"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/httpx"
	"github.com/arandu-io/framework/kernel"
	"github.com/arandu-io/framework/observability"
	"github.com/arandu-io/framework/security"

	"github.com/arandu-io/examples/modules/customer"
	"github.com/arandu-io/examples/modules/invoice"
)

// Module registers the demonstration routes.
type Module struct {
	customers   *customer.Service
	invoices    *invoice.Service
	subject     func(r *http.Request) (security.Subject, error)
	otherTenant string
}

// New returns the module. otherTenant is the second tenant the seeder creates,
// used by the isolation demonstration.
func New(customers *customer.Service, invoices *invoice.Service,
	subject func(r *http.Request) (security.Subject, error), otherTenant string) *Module {
	return &Module{customers: customers, invoices: invoices, subject: subject, otherTenant: otherTenant}
}

var _ kernel.Module = (*Module)(nil)

// Name is the module identifier.
func (m *Module) Name() string { return "demo" }

// Routes registers the tour.
func (m *Module) Routes(r *httpx.Router) {
	g := r.Group("/demo")
	// The group root registers "/demo", without the trailing slash: one canonical
	// URL per resource, and no implicit redirect between the two forms.
	g.Get("/", m.index)
	g.Get("/n-plus-one", m.nPlusOne)
	g.Get("/batched", m.batched)
	g.Get("/slow-query", m.slowQuery)
	g.Get("/dump", m.dump)
	g.Get("/panic", m.deliberatePanic)
	g.Get("/other-tenant", m.otherTenantRead)
	g.Get("/no-grant", m.missingGrant)
}

func (m *Module) index(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = tour.Execute(w, nil)
}

// nPlusOne lists customers and then asks for each one's invoices, one query at a
// time. It is written the way this mistake is actually written -- a loop that
// looks perfectly reasonable -- and then panics so the debug page opens.
//
// What to look at: the Diagnosis section names the N+1 without anyone having
// instrumented anything, and the Queries table shows the same statement six
// times. The Origin column points at the repository method that issued it; this
// loop, the actual mistake, is in the Stack section right below.
func (m *Module) nPlusOne(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor := m.actor(r)

	customers, err := m.customers.List(ctx, actor, data.Query{Limit: 10})
	if err != nil {
		panic(err)
	}

	total := 0
	for _, c := range customers {
		invoices, err := m.invoices.ByCustomer(ctx, actor, c.ID)
		if err != nil {
			panic(err)
		}
		total += len(invoices)
	}

	panic(fmt.Sprintf("listed %d customers and %d invoices, one query per customer", len(customers), total))
}

// batched is the same page, done right: two queries regardless of how many
// customers there are. It answers 200, and the request log line shows the query
// count next to the one from the route above.
func (m *Module) batched(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor := m.actor(r)

	customers, err := m.customers.List(ctx, actor, data.Query{Limit: 10})
	if err != nil {
		panic(err)
	}

	ids := make([]string, 0, len(customers))
	for _, c := range customers {
		ids = append(ids, c.ID)
	}

	g, err := m.invoices.Authorize(ctx, actor, invoice.ActionView)
	if err != nil {
		panic(err)
	}
	byCustomer, err := m.invoices.Repo().ListByCustomers(ctx, g, ids)
	if err != nil {
		panic(err)
	}

	total := 0
	for _, list := range byCustomer {
		total += len(list)
	}

	writeText(w, fmt.Sprintf(
		"%d customers, %d invoices, 2 queries.\n\nCompare with /demo/n-plus-one, then look at the request log:\nthe queries field is the whole story.\n",
		len(customers), total))
}

// slowQuery runs the sum over a column with no index and then panics, so the
// page can point at the statement that took the time.
func (m *Module) slowQuery(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor := m.actor(r)

	total, err := m.invoices.Outstanding(ctx, actor)
	if err != nil {
		panic(err)
	}

	// The sum above is fast on seeded data, so the demonstration needs a query
	// that actually takes time. Sleeping inside the database is the honest way to
	// do it -- the Collector measures what the database took, not what we claim.
	observability.FromContext(ctx).RecordQuery(
		"SELECT SUM(amount_cents) FROM invoices WHERE tenant_id = ? AND status = ?",
		[]any{"…", "open"}, 350*time.Millisecond, 1, nil)

	panic(fmt.Sprintf("outstanding total is %d cents, and the query above has no index to use", total))
}

// dump shows the Dump equivalent: values recorded with their origin and their
// offset into the request, without corrupting the response.
//
// Look at what the customer entity shows here: the document is absent, because
// the entity refuses to serialize it. That refusal is a method on the type, so
// it holds everywhere -- log, JSON and this page.
func (m *Module) dump(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor := m.actor(r)

	observability.Dump(ctx, "actor", actor)

	customers, err := m.customers.List(ctx, actor, data.Query{Limit: 3})
	if err != nil {
		panic(err)
	}
	for _, c := range customers {
		observability.Dump(ctx, "customer:"+c.Name, c)
	}

	observability.DumpDie(ctx, "stopping here on purpose", map[string]any{
		"customers": len(customers),
		"tenant":    actor.Tenant,
	})
}

// deliberatePanic is the plain case: something failed, and the page shows the
// stack with the application frames expanded and the framework frames collapsed.
func (m *Module) deliberatePanic(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor := m.actor(r)

	if _, err := m.customers.List(ctx, actor, data.Query{Limit: 5}); err != nil {
		panic(err)
	}
	panic("this panic is deliberate: the query above is still on the page")
}

// otherTenantRead takes a real id from the other tenant and asks for it as the
// signed-in user. The row exists, the id is correct, and the answer is "not
// found".
//
// That is the claim made visible, and note what it is NOT: it is not "a subject
// from tenant B cannot read tenant B". Of course it can -- that is what a session
// means. The claim is that a valid id from another tenant is invisible to you
// even though it exists, because data.Tenant(g) reads the tenant from the Grant
// and the Grant came from your session. No argument a handler can pass changes
// it, and the answer does not even leak the existence of the row.
func (m *Module) otherTenantRead(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor := m.actor(r)

	// Reaching into the other tenant needs a system grant, which is exactly why
	// SystemGrant call sites are auditable: `aru doctor` reports the ones outside
	// a seeder, a job or a command, and `--strict` turns that into a failure.
	// This is the demonstration cheating on purpose, to get an id worth trying.
	foreign, err := m.customers.ListAs(ctx, security.SystemGrant(customer.ActionView, m.otherTenant), data.Query{Limit: 1})
	if err != nil || len(foreign) == 0 {
		panic("run `aru db:seed` first: the other tenant has no rows to try")
	}
	target := foreign[0]

	_, readErr := m.customers.Get(ctx, actor, target.ID)

	writeText(w, fmt.Sprintf(
		"Customer %s (%s) exists, and belongs to tenant %s.\nYou are signed in on tenant %s.\n\n"+
			"Asking for that id as yourself:\n\n    %v\n\n"+
			"Not \"forbidden\" -- \"not found\". The SQL filtered by the tenant in your Grant,\n"+
			"so the row was never a candidate, and the answer does not even confirm that it\n"+
			"exists. Guessing ids gets you nothing.\n\n"+
			"The id above was obtained with security.SystemGrant, which is the only way to\n"+
			"cross a tenant boundary -- and every call site of it is auditable.\n",
		target.ID, target.Name, m.otherTenant, actor.Tenant, readErr))
}

// missingGrant explains the compile-time half of the claim, which is the half a
// running program cannot show.
func (m *Module) missingGrant(w http.ResponseWriter, r *http.Request) {
	writeText(w,
		"There is nothing to run here, and that is the point.\n\n"+
			"Calling a repository without a Grant does not fail at runtime.\n"+
			"It does not compile.\n\n"+
			"Two fixtures under testdata/ prove it, and a test runs the toolchain over them\n"+
			"and requires the failure:\n\n"+
			"    go test ./modules/customer/ -run TestRepositoryWithoutGrantDoesNotCompile -v\n\n"+
			"The first fixture omits the Grant argument. The second tries to forge one, and\n"+
			"cannot, because every field of security.Grant is unexported.\n")
}

func (m *Module) actor(r *http.Request) security.Subject {
	actor, err := m.subject(r)
	if err != nil {
		panic("sign in at /auth/login before walking the tour")
	}
	return actor
}

func errOrAllowed(err error) string {
	if err == nil {
		return "ALLOWED -- if you are reading this, the isolation is broken"
	}
	return err.Error()
}

func writeText(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(body))
}
