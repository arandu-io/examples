package repositories_test

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	"github.com/arandu-io/framework/security"

	"github.com/arandu-io/framework/data"

	models "github.com/arandu-io/examples/app/Models"
	policies "github.com/arandu-io/examples/app/Policies"
	repositories "github.com/arandu-io/examples/app/Repositories"
)

// TestRepositoryWithoutGrantDoesNotCompile is the claim this whole application
// is here to demonstrate, checked on generated code rather than on the
// framework's own.
//
// The framework proves it for the repository it ships. This proves it for the
// one `aru make:module post` wrote, which is the one a project actually gets --
// and a guarantee that held only for hand-written code would not be a guarantee.
func TestRepositoryWithoutGrantDoesNotCompile(t *testing.T) {
	if testing.Short() {
		t.Skip("compiles a fixture with the go tool")
	}

	out, err := exec.Command("go", "vet", "./testdata/missing_grant").CombinedOutput()
	if err == nil {
		t.Fatalf("calling the repository without a Grant compiled. The thesis is broken.\n%s", out)
	}
	if want := "not enough arguments in call to repo.Find"; !strings.Contains(string(out), want) {
		t.Fatalf("the fixture failed for the wrong reason.\nwant a message containing: %s\ngot:\n%s", want, out)
	}
}

// TestEveryMethodRequiresItsGrant is RULE 17: reading is not an exception.
//
// It is easy to believe authorization is about writes. It is not -- List and
// Find are where a tenant reads another tenant's rows, and a framework that
// checked only the writes would leak everything while looking careful.
//
// Every method is called with a Grant issued for the WRONG action, and every
// one has to refuse. The zero Grant would also refuse, and would prove less:
// this shows that carrying *a* Grant is not enough, it has to be the right one.
func TestEveryMethodRequiresItsGrant(t *testing.T) {
	repo := repositories.NewPostRepository(nil)
	ctx := context.Background()

	// A grant for delete, handed to every other method.
	//arandu:system-grant a test needs a Grant nothing issued, to prove the wrong one is refused
	wrong := security.SystemGrant(policies.PostDelete, "acme")

	calls := map[string]func() error{
		"Find":   func() error { _, err := repo.Find(ctx, wrong, "id"); return err },
		"List":   func() error { _, err := repo.List(ctx, wrong, data.Query{Limit: 10}); return err },
		"Create": func() error { _, err := repo.Create(ctx, wrong, models.Post{}); return err },
		"Update": func() error { _, err := repo.Update(ctx, wrong, models.Post{}); return err },
	}

	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			// nil is passed as the database on purpose: refusing has to happen
			// before anything is asked of it. A method that panics here is a
			// method that checked the Grant too late.
			if err := call(); err == nil {
				t.Fatalf("%s accepted a Grant issued for %s", name, policies.PostDelete)
			}
		})
	}
}
