package controllers_test

import (
	"context"
	"errors"
	"testing"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/security"

	models "github.com/arandu-io/examples/app/Models"
	policies "github.com/arandu-io/examples/app/Policies"
	repositories "github.com/arandu-io/examples/app/Repositories"
)

// These tests need no database: every repository method checks the Grant before
// touching the handle, which is exactly the property under test.
func postRepoWithoutDB() *repositories.PostRepository {
	return repositories.NewPostRepository(nil)
}

// TestEveryPostMethodRequiresItsGrant is the framework thesis at runtime:
// the zero Grant -- the only one a caller outside the security package can build
// -- never gets through, and a grant for one action does not open another.
func TestEveryPostMethodRequiresItsGrant(t *testing.T) {
	repo := postRepoWithoutDB()
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
			_, err := repo.Create(ctx, g, models.Post{})
			return err
		},
		"Update": func(g security.Grant) error {
			_, err := repo.Update(ctx, g, models.Post{})
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

// TestThePostPolicyDeniesWhatItDoesNotKnow is the property that keeps a
// policy safe as it grows: an action nobody wrote a rule for is refused, rather
// than falling through to allowed.
//
// It uses an action that will never be opened, so it keeps passing after you open
// the real ones -- a test that breaks when you do what the generator told you to
// do is a test people delete.
func TestThePostPolicyDeniesWhatItDoesNotKnow(t *testing.T) {
	admin := security.Subject{ID: "a1", Tenant: "t1", Roles: []string{"admin", "staff"}}

	err := (policies.PostPolicy{}).Can(context.Background(), admin,
		"post.action_that_does_not_exist", models.Post{})

	if err == nil {
		t.Fatal("an action with no rule was allowed: the policy falls through to allowed")
	}
}

// TestPostListRejectsSortOutsideTheAllowlist keeps the one door a column
// name could come through closed.
func TestPostListRejectsSortOutsideTheAllowlist(t *testing.T) {
	repo := postRepoWithoutDB()
	// PostList, because listing is its own permission: a role may be
	// allowed to open the record it was given and not to page through every one.
	g := security.SystemGrant(policies.PostList, "t1")

	_, err := repo.List(context.Background(), g, data.Query{Sort: "1; DROP TABLE posts"})

	if !errors.Is(err, models.ErrPostSort) {
		t.Fatalf("error = %v, want ErrPostSort", err)
	}
}

// arandu:begin custom
// Tests for the rules you wrote go here, and survive regeneration.
// arandu:end custom
