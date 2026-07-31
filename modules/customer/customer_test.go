package customer_test

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/security"

	"github.com/arandu-io/examples/modules/customer"
)

// The repository tests below need no database: every method checks the Grant
// before touching the handle, which is exactly the property under test.
func repoWithoutDB() *customer.Repo { return customer.NewRepo(nil) }

// TestRepositoryWithoutGrantDoesNotCompile is the claim of the framework, made
// falsifiable. A claim about compilation can only be proven by attempting a
// compilation, so this runs the toolchain over the fixtures under testdata and
// requires them to fail -- with the specific message, because a fixture that
// fails for an unrelated reason proves nothing.
func TestRepositoryWithoutGrantDoesNotCompile(t *testing.T) {
	if testing.Short() {
		t.Skip("compiles a fixture with the go tool")
	}

	cases := []struct {
		fixture string
		reason  string
		want    string
	}{
		{
			fixture: "./testdata/missing_grant",
			reason:  "calling a repository without a Grant",
			want:    "not enough arguments in call to repo.Find",
		},
		{
			fixture: "./testdata/forged_grant",
			reason:  "building a valid Grant outside the security package",
			want:    "cannot refer to unexported field valid",
		},
	}

	for _, c := range cases {
		t.Run(c.reason, func(t *testing.T) {
			out, err := exec.Command("go", "vet", c.fixture).CombinedOutput()
			if err == nil {
				t.Fatalf("%s compiled. The framework thesis is broken.\n%s", c.reason, out)
			}
			if !strings.Contains(string(out), c.want) {
				t.Fatalf("the fixture failed for the wrong reason.\nwant a message containing: %s\ngot:\n%s", c.want, out)
			}
		})
	}
}

// TestEveryMethodRequiresItsGrant is the runtime half: the zero Grant -- the only
// one a caller outside the security package can build -- never gets through, and
// a grant for one action does not open another.
func TestEveryMethodRequiresItsGrant(t *testing.T) {
	repo := repoWithoutDB()
	ctx := context.Background()
	var zero security.Grant

	calls := map[string]func(security.Grant) error{
		"Find": func(g security.Grant) error {
			_, err := repo.Find(ctx, g, "id")
			return err
		},
		"List": func(g security.Grant) error {
			_, err := repo.List(ctx, g, data.Query{})
			return err
		},
		"Create": func(g security.Grant) error {
			_, err := repo.Create(ctx, g, customer.Customer{})
			return err
		},
		"Update": func(g security.Grant) error {
			_, err := repo.Update(ctx, g, customer.Customer{})
			return err
		},
		"Delete": func(g security.Grant) error {
			return repo.Delete(ctx, g, "id")
		},
	}

	for name, call := range calls {
		t.Run(name+" with no grant", func(t *testing.T) {
			if err := call(zero); !errors.Is(err, security.ErrForbidden) {
				t.Fatalf("error = %v, want ErrForbidden", err)
			}
		})
		t.Run(name+" with a grant for another action", func(t *testing.T) {
			if err := call(security.SystemGrant("some.other.action", "t1")); !errors.Is(err, security.ErrForbidden) {
				t.Fatalf("error = %v, want ErrForbidden", err)
			}
		})
	}
}

// TestListRejectsSortOutsideTheAllowlist: a sort field is a column name, and a
// column name taken from the request is injection through a door nobody watches.
func TestListRejectsSortOutsideTheAllowlist(t *testing.T) {
	repo := repoWithoutDB()
	g := security.SystemGrant(customer.ActionView, "t1")

	_, err := repo.List(context.Background(), g, data.Query{Sort: "name; DROP TABLE customers"})

	if !errors.Is(err, customer.ErrSortNotList) {
		t.Fatalf("error = %v, want ErrSortNotList", err)
	}
}

func TestPolicyIsolatesTenants(t *testing.T) {
	admin := security.Subject{ID: "a1", Tenant: "tenant-a", Roles: []string{customer.RoleAdmin}}
	foreign := customer.Customer{ID: "c9", TenantID: "tenant-b"}

	err := customer.Policy{}.Can(context.Background(), admin, customer.ActionView, foreign)

	if err == nil {
		t.Fatal("an admin reached a customer in another tenant")
	}
	if !strings.Contains(err.Error(), "another tenant") {
		t.Errorf("error = %v", err)
	}
}

// TestReadingTheFullDocumentIsItsOwnPermission is the decision worth copying from
// this module: seeing a record and seeing every field of it are different
// questions, and most systems learn that during an incident.
func TestReadingTheFullDocumentIsItsOwnPermission(t *testing.T) {
	ctx := context.Background()
	support := security.Subject{ID: "s1", Tenant: "t1", Roles: []string{customer.RoleSupport}}
	record := customer.Customer{ID: "c1", TenantID: "t1"}

	if err := (customer.Policy{}).Can(ctx, support, customer.ActionView, record); err != nil {
		t.Fatalf("support must be able to read the customer: %v", err)
	}
	if err := (customer.Policy{}).Can(ctx, support, customer.ActionViewFull, record); err == nil {
		t.Fatal("support read the full document, which is a separate permission")
	}
}

func TestPolicyDecisions(t *testing.T) {
	ctx := context.Background()
	admin := security.Subject{ID: "a1", Tenant: "t1", Roles: []string{customer.RoleAdmin}}
	sales := security.Subject{ID: "s1", Tenant: "t1", Roles: []string{customer.RoleSales}}
	support := security.Subject{ID: "p1", Tenant: "t1", Roles: []string{customer.RoleSupport}}
	nobody := security.Subject{ID: "n1", Tenant: "t1"}
	record := customer.Customer{ID: "c1", TenantID: "t1"}

	cases := []struct {
		name    string
		subject security.Subject
		action  security.Action
		allowed bool
	}{
		{"admin views", admin, customer.ActionView, true},
		{"admin creates", admin, customer.ActionCreate, true},
		{"admin deletes", admin, customer.ActionDelete, true},
		{"sales views", sales, customer.ActionView, true},
		{"sales creates", sales, customer.ActionCreate, true},
		{"sales deletes", sales, customer.ActionDelete, false},
		{"support views", support, customer.ActionView, true},
		{"support creates", support, customer.ActionCreate, false},
		{"nobody views", nobody, customer.ActionView, false},
		{"unknown action is denied", admin, "customer.impersonate", false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := customer.Policy{}.Can(ctx, c.subject, c.action, record)
			if c.allowed && err != nil {
				t.Fatalf("denied: %v", err)
			}
			if !c.allowed && err == nil {
				t.Fatal("allowed, want denied")
			}
		})
	}
}

// TestDocumentNeverLeaves is why the entity has MarshalJSON and LogValue: one
// dump or one log line would otherwise publish a national registration number on
// the debug page.
func TestDocumentNeverLeaves(t *testing.T) {
	c := customer.Customer{
		ID: "c1", TenantID: "t1", Name: "Ferrovia Central",
		Email: "contato@ferroviacentral.test", Document: "11222333000181",
	}

	encoded, err := c.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	if strings.Contains(string(encoded), "11222333000181") {
		t.Fatalf("the document reached JSON: %s", encoded)
	}

	if logged := c.LogValue().String(); strings.Contains(logged, "11222333000181") {
		t.Fatalf("the document reached the log: %s", logged)
	}

	if masked := c.MaskedDocument(); masked != "***0181" {
		t.Fatalf("MaskedDocument = %q, want ***0181", masked)
	}
}

func TestValidation(t *testing.T) {
	cases := map[string]struct {
		request customer.CreateRequest
		fields  []string
	}{
		"valid": {
			request: customer.CreateRequest{Name: "Ferrovia", Email: "a@b.co", Document: "11222333000181"},
		},
		"missing everything": {
			request: customer.CreateRequest{},
			fields:  []string{"name", "email", "document"},
		},
		"malformed email": {
			request: customer.CreateRequest{Name: "Ferrovia", Email: "not-an-email", Document: "11222333000181"},
			fields:  []string{"email"},
		},
		"short document": {
			request: customer.CreateRequest{Name: "Ferrovia", Email: "a@b.co", Document: "123"},
			fields:  []string{"document"},
		},
		"document with punctuation is fine": {
			request: customer.CreateRequest{Name: "Ferrovia", Email: "a@b.co", Document: "11.222.333/0001-81"},
		},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			errs := c.request.Validate()
			if len(c.fields) == 0 {
				if errs.Any() {
					t.Fatalf("valid input was rejected: %v", errs)
				}
				return
			}
			for _, f := range c.fields {
				if len(errs[f]) == 0 {
					t.Errorf("field %q was not reported: %v", f, errs)
				}
			}
		})
	}
}

func TestNewIDIsAUUIDv4(t *testing.T) {
	id, err := customer.NewID()
	if err != nil {
		t.Fatalf("NewID: %v", err)
	}

	if len(id) != 36 || id[14] != '4' {
		t.Fatalf("id = %q, want a version 4 uuid", id)
	}
	other, _ := customer.NewID()
	if id == other {
		t.Fatal("two generated ids are identical")
	}
}
