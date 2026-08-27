package unit_test

import (
	"context"
	"errors"
	"testing"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/security"

	requests "github.com/arandu-io/examples/app/Http/Requests"
	models "github.com/arandu-io/examples/app/Models"
	policies "github.com/arandu-io/examples/app/Policies"
	services "github.com/arandu-io/examples/app/Services"
)

// These tests need no database, and the nil handle is what proves it: every
// method below asks the Policy before it builds a statement, which is exactly
// the property under test. A method that panics here is a method that
// authorized too late.
//
// They used to be written against CategoryRepository, where the door was
// g.Check as the first line of each method. That repository is gone -- every
// statement in it was CRUD over one table, so the CRUD moved to the model --
// and the door is now CategoryService, which asks the Policy and spends the
// Grant it gets in the same function body.
//
// What is asserted is the same guarantee at the new door, and it is worth
// naming what could not come with it: g.Check answered "was this Grant issued
// for this exact action", and a model terminal does not ask that -- it takes
// the tenant off the Grant and nothing else. That question is only answerable
// where a Grant crosses a boundary, and here it crosses none: nothing hands
// this service a Grant, it mints its own. The two repositories that remain
// still check, because they are still handed one.
func categoryServiceWithoutDB() *services.CategoryService {
	return services.NewCategoryService(nil)
}

// TestEveryCategoryMethodRefusesASubjectNobodyFilledIn is the framework thesis
// at runtime, one layer up from where it used to be: the zero Subject -- a
// session that failed to load -- never gets through, on a read as much as on a
// write.
//
// Authorize refuses it before the Policy is even consulted, which is the right
// order: answering an authorization question about nobody is how a hole opens
// by accident.
func TestEveryCategoryMethodRefusesASubjectNobodyFilledIn(t *testing.T) {
	svc := categoryServiceWithoutDB()
	ctx := context.Background()
	var nobody security.Subject

	// Valid input, so that Create fails the authorization rather than the
	// validation: a refusal for the wrong reason proves nothing about the door.
	valid := requests.StoreCategory{Name: "Reports", Slug: "reports", Description: "A section."}

	calls := map[string]func(security.Subject) error{
		"Create": func(s security.Subject) error { _, err := svc.Create(ctx, s, valid); return err },
		"Get":    func(s security.Subject) error { _, err := svc.Get(ctx, s, "id"); return err },
		"List":   func(s security.Subject) error { _, err := svc.List(ctx, s, data.Query{}); return err },
		"Update": func(s security.Subject) error {
			_, err := svc.Update(ctx, s, requests.UpdateCategory{
				ID: "id", Name: "Reports", Slug: "reports", Description: "A section.",
			})
			return err
		},
		"Delete": func(s security.Subject) error { return svc.Delete(ctx, s, "id") },
		"BySlug": func(s security.Subject) error { _, err := svc.BySlug(ctx, s, "reports"); return err },
		"All":    func(s security.Subject) error { _, err := svc.All(ctx, s); return err },
	}

	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			// nil is passed as the database on purpose: refusing has to happen
			// before anything is asked of it.
			if err := call(nobody); !errors.Is(err, security.ErrForbidden) {
				t.Fatalf("error = %v, want ErrForbidden", err)
			}
		})
	}
}

// TestCreatingASectionNeedsMoreThanAnAccount is the other half: permission for
// one action does not open another.
//
// A reader may look at the sections -- the navigation is built from them, and a
// navigation that disappears when you sign out was never public -- and may not
// invent one. The subject below is allowed CategoryView and CategoryList by the
// same policy that refuses this call, so what is proved is the boundary between
// them rather than a subject nobody trusts.
//
// Update and Delete are not here, and that is not an omission: both read the
// stored row before they authorize the write, so proving their refusal needs a
// row, and that test is in the Feature suite where rows exist.
func TestCreatingASectionNeedsMoreThanAnAccount(t *testing.T) {
	svc := categoryServiceWithoutDB()
	reader := security.Subject{ID: "u1", Tenant: "t1", Roles: []string{models.RoleMember}}

	_, err := svc.Create(context.Background(), reader, requests.StoreCategory{
		Name: "Reports", Slug: "reports", Description: "A section.",
	})

	if !errors.Is(err, security.ErrForbidden) {
		t.Fatalf("error = %v, want ErrForbidden: an account is not an administrator", err)
	}
}

// TestTheCategoryPolicyDeniesWhatItDoesNotKnow is the property that keeps a
// policy safe as it grows: an action nobody wrote a rule for is refused, rather
// than falling through to allowed.
//
// It uses an action that will never be opened, so it keeps passing after you open
// the real ones -- a test that breaks when you do what the generator told you to
// do is a test people delete.
func TestTheCategoryPolicyDeniesWhatItDoesNotKnow(t *testing.T) {
	admin := security.Subject{ID: "a1", Tenant: "t1", Roles: []string{"admin", "staff"}}

	err := (policies.CategoryPolicy{}).Can(context.Background(), admin,
		"category.action_that_does_not_exist", models.Category{})

	if err == nil {
		t.Fatal("an action with no rule was allowed: the policy falls through to allowed")
	}
}

// TestCategoryListRejectsSortOutsideTheAllowlist keeps the one door a column
// name could come through closed.
//
// The model layer quotes an identifier; it does not judge it. A sort field is a
// column name, so the allowlist is what stands between the query builder and a
// value from the address bar -- and it is checked before the handle is touched,
// which is why this runs with none.
func TestCategoryListRejectsSortOutsideTheAllowlist(t *testing.T) {
	svc := categoryServiceWithoutDB()
	// A reader, because listing is open to everybody: the refusal below has to
	// be the allowlist and not the policy.
	reader := security.Subject{ID: "u1", Tenant: "t1", Roles: []string{models.RoleMember}}

	_, err := svc.List(context.Background(), reader, data.Query{Sort: "1; DROP TABLE categories"})

	if !errors.Is(err, models.ErrCategorySort) {
		t.Fatalf("error = %v, want ErrCategorySort", err)
	}
}
