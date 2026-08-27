package feature_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/observability"
	"github.com/arandu-io/framework/security"
	"github.com/arandu-io/hesape/database/model"

	requests "github.com/arandu-io/examples/app/Http/Requests"
	models "github.com/arandu-io/examples/app/Models"
	policies "github.com/arandu-io/examples/app/Policies"
	services "github.com/arandu-io/examples/app/Services"
	"github.com/arandu-io/examples/bootstrap"
	factories "github.com/arandu-io/examples/database/factories"
)

// The sections are the one entity of this application read and written through
// the model rather than through a repository, and this file is what says so out
// loud.
//
// It proves four things, and each of them is a claim that would otherwise rest
// on a doc comment:
//
//  1. a Grant is on every terminal, and a query that has none does not run;
//  2. the tenant off that Grant reaches the SQL, read at the statement rather
//     than inferred from the rows that came back;
//  3. a factory makes without one and creates with one;
//  4. the tenant a write files a row under is the Grant's, whatever the caller
//     put in the struct.
//
// The relation, its read back, and the delete guard built on it are next door,
// in TenantScoping_test.go, beside the fixture that writes the crossing rows
// they are about.

// TestNoSectionStatementRunsWithoutAGrant.
//
// Every terminal in the model layer takes an auth.Grant -- All, Find, First,
// Get, Value, Pluck, Insert, Update, Delete -- so a query reached without one
// is a compile error. What this covers is the runtime half: a Grant that
// carries no tenant is refused before any SQL is built, which is the only
// answer that is safe, because a statement with no tenant in its where clause
// reads every customer of the system.
//
// The zero Grant is the one a caller outside the security package can build,
// and it is the one a mistake produces.
func TestNoSectionStatementRunsWithoutAGrant(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	scopedFixture(t, db)

	col := observability.NewCollector("no-grant")
	ctx = observability.WithCollector(ctx, col)

	sections := models.Categories(db)
	var zero security.Grant

	terminals := map[string]func() error{
		"Get":    func() error { _, err := sections.NewQuery().Get(ctx, zero); return err },
		"First":  func() error { _, err := sections.NewQuery().First(ctx, zero); return err },
		"Value":  func() error { _, err := sections.NewQuery().Value(ctx, zero, "slug"); return err },
		"Pluck":  func() error { _, err := sections.NewQuery().Pluck(ctx, zero, "slug"); return err },
		"Count":  func() error { _, err := sections.NewQuery().Count(ctx, zero); return err },
		"Update": func() error { _, err := sections.NewQuery().Update(ctx, zero, map[string]any{"name": "x"}); return err },
		"Delete": func() error { _, err := sections.NewQuery().Delete(ctx, zero); return err },
	}

	for name, run := range terminals {
		t.Run(name, func(t *testing.T) {
			if err := run(); !errors.Is(err, model.ErrNoTenant) {
				t.Fatalf("%s = %v, want ErrNoTenant", name, err)
			}
		})
	}

	// The second half, and it is the one that matters: refused is not the same
	// as refused before the statement. Nothing reached the database, so there is
	// no window in which an unscoped select existed.
	if n := col.QueryCount(); n != 0 {
		t.Fatalf("%d statement(s) reached the database under a Grant with no tenant:\n%s",
			n, statements(col))
	}
}

// TestTheTenantReachesTheSQLOfEverySectionStatement.
//
// The tests next door prove the tenant by the rows that come back, which is the
// right way round: a filter that is present and wrong is still wrong. This one
// reads the statement instead, because the two answer different questions --
// "did this query return another tenant's rows" and "is the predicate in the
// statement at all" -- and a fixture with one tenant in it answers the first
// one green either way.
//
// It reads what the Collector recorded, which is the same handle the
// repositories next door write through: the model layer runs every statement
// through QueryContext and ExecContext on *data.DB, so the recording, the
// placeholder numbering and any open transaction are the handle's.
func TestTheTenantReachesTheSQLOfEverySectionStatement(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()

	col := observability.NewCollector("tenant-in-sql")
	ctx = observability.WithCollector(ctx, col)

	svc := services.NewCategoryService(db)
	ours := bootstrap.Tenant()
	actor := security.Subject{ID: "u1", Tenant: ours, Roles: []string{"admin"}}

	// One of each shape the service issues: an insert, a lookup by key, a
	// lookup by a unique column, an ordered listing, a keyset page with the
	// anchor lookup inside it, an update and a delete.
	created, err := svc.Create(ctx, actor, requests.StoreCategory{
		Name: "Reports", Slug: "reports", Description: "A section.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Get(ctx, actor, created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.BySlug(ctx, actor, "reports"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.All(ctx, actor); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.List(ctx, actor, data.Query{Limit: 10}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.List(ctx, actor, data.Query{Limit: 10, Cursor: created.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Update(ctx, actor, requests.UpdateCategory{
		ID: created.ID, Name: "Dispatches", Slug: "dispatches", Description: "A section.",
	}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(ctx, actor, created.ID); err != nil {
		t.Fatal(err)
	}

	recorded := col.Queries()
	if len(recorded) < 9 {
		t.Fatalf("the Collector saw %d statements and the calls above issue at least nine: "+
			"the model layer is not running through the instrumented handle:\n%s",
			len(recorded), statements(col))
	}

	for _, q := range recorded {
		sql := strings.ToLower(q.SQL)
		// The insert names the column in its column list rather than in a where
		// clause, so the check is the column and the value, not the predicate.
		if !strings.Contains(sql, "tenant_id") {
			t.Errorf("a statement names no tenant column:\n%s\nargs=%v", q.SQL, q.Args)
			continue
		}
		if !bound(q.Args, ours) {
			t.Errorf("a statement names the tenant column and does not bind this tenant:\n%s\nargs=%v",
				q.SQL, q.Args)
		}
	}
}

// TestTheFactoryMakesWithoutAGrantAndCreatesWithOne.
//
// The two signatures are the guarantee, and this is what stands behind them.
// Make takes no context and no Grant, so it cannot write and does not: the row
// it answers is not in the table. Create takes one, and the row it stores is
// filed under data.Tenant(g).
//
// The definition sets another tenant on purpose. A factory that trusted the
// field would be exactly the back door the Grant exists to close -- seeding is
// the one place in an application where somebody is holding a system grant and
// a struct they wrote by hand.
func TestTheFactoryMakesWithoutAGrantAndCreatesWithOne(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	ours := bootstrap.Tenant()

	made, err := factories.Categories(db).
		State(func(ca *models.Category) { ca.TenantID = theirTenant }).
		MakeOne()
	if err != nil {
		t.Fatal(err)
	}
	if made.Name == "" || made.Slug == "" {
		t.Fatalf("the factory made a section with nothing in it: %+v", made)
	}
	if categoryName(t, db, made.ID) != "" {
		t.Fatal("Make wrote a row: it takes no Grant, so it must take no statement either")
	}

	//arandu:system-grant a factory is written against the same Grant a seeder holds, and a test has no session
	g := security.SystemGrant(policies.CategoryCreate, ours)
	stored, err := factories.Categories(db).
		State(func(ca *models.Category) { ca.TenantID = theirTenant }).
		CreateOne(ctx, g)
	if err != nil {
		t.Fatal(err)
	}

	// Read off the table rather than off the value in hand: the struct still
	// holds what the definition wrote, and the question is what the INSERT put
	// in the column.
	if tenant := categoryTenant(t, db, stored.ID); tenant != ours {
		t.Fatalf("the factory filed the row under %q, which its own definition chose", tenant)
	}
	if categoryName(t, db, stored.ID) != stored.Name {
		t.Fatalf("the row the factory returned is not the row it stored")
	}
}

// TestChangingASectionNeedsMoreThanAnAccount is the half of the policy boundary
// that needs a row.
//
// Update and Delete read the stored section before they authorize the write, so
// the policy decides against what is in the table rather than against what the
// client claims is there. That is why they cannot be proved with no database,
// and it is the order that makes them worth proving: a check that ran on the
// caller's copy would pass on attacker-supplied data.
func TestChangingASectionNeedsMoreThanAnAccount(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	scopedFixture(t, db)

	svc := services.NewCategoryService(db)
	ours := bootstrap.Tenant()
	reader := security.Subject{ID: "u1", Tenant: ours, Roles: []string{models.RoleMember}}

	// The reader can see it. The refusals below are therefore about the action
	// and not about the row being out of reach.
	if _, err := svc.Get(ctx, reader, ourCategory); err != nil {
		t.Fatalf("a reader could not open a section: %v", err)
	}

	_, err := svc.Update(ctx, reader, requests.UpdateCategory{
		ID: ourCategory, Name: "Rewritten", Slug: "rewritten", Description: "A section.",
	})
	if !errors.Is(err, security.ErrForbidden) {
		t.Fatalf("Update = %v, want ErrForbidden", err)
	}
	if name := categoryName(t, db, ourCategory); name != "Reports" {
		t.Fatalf("the section now reads %q", name)
	}

	if err := svc.Delete(ctx, reader, ourCategory); !errors.Is(err, security.ErrForbidden) {
		t.Fatalf("Delete = %v, want ErrForbidden", err)
	}
	if categoryName(t, db, ourCategory) == "" {
		t.Fatal("a reader deleted a section")
	}
}

// TestSavingASectionWithoutChangingItIsNotAMissingRow.
//
// An update that sets a row to what it already holds affects zero rows on the
// engines that count changed rows rather than matched ones. Reading that as
// "the section is gone" is the bug a user meets on their first save, and it is
// invisible on SQLite -- which counts matched rows, so this test would pass over
// the mistake either way if it only asserted the answer.
//
// So it asserts the row as well: the section is still there, under its own
// name, after a save that wrote nothing.
func TestSavingASectionWithoutChangingItIsNotAMissingRow(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	scopedFixture(t, db)

	svc := services.NewCategoryService(db)
	ours := bootstrap.Tenant()
	actor := security.Subject{ID: "u1", Tenant: ours, Roles: []string{"admin"}}

	stored, err := svc.Get(ctx, actor, ourCategory)
	if err != nil {
		t.Fatal(err)
	}

	same, err := svc.Update(ctx, actor, requests.UpdateCategory{
		ID: stored.ID, Name: stored.Name, Slug: stored.Slug, Description: stored.Description,
	})
	if err != nil {
		t.Fatalf("saving a section unchanged = %v, and the row is still there", err)
	}
	if same.Name != stored.Name {
		t.Fatalf("the section came back as %q and was saved as %q", same.Name, stored.Name)
	}
	if name := categoryName(t, db, stored.ID); name != stored.Name {
		t.Fatalf("the table holds %q after a save that changed nothing", name)
	}
}

// bound reports whether one of the statement's arguments is this value.
func bound(args []any, want string) bool {
	for _, arg := range args {
		if s, ok := arg.(string); ok && s == want {
			return true
		}
	}
	return false
}

// statements renders what the Collector saw, for a failure message that says
// which statement rather than how many.
func statements(col *observability.Collector) string {
	var out strings.Builder
	for _, q := range col.Queries() {
		out.WriteString("  ")
		out.WriteString(q.SQL)
		out.WriteString("\n")
	}
	return out.String()
}
