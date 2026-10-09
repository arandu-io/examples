package unit_test

import (
	"context"
	"errors"
	"testing"

	"github.com/arandu-io/hesape/auth"
	"github.com/arandu-io/hesape/database"

	models "github.com/arandu-io/examples/app/Models"
	policies "github.com/arandu-io/examples/app/Policies"
	repositories "github.com/arandu-io/examples/app/Repositories"
)

// These tests need no database: every repository method checks the Grant before
// touching the handle, which is exactly the property under test.
func commentRepoWithoutDB() *repositories.CommentRepository {
	return repositories.NewCommentRepository(nil)
}

// TestEveryCommentMethodRequiresItsGrant is the framework thesis at runtime:
// the zero Grant -- the only one a caller outside the security package can build
// -- never gets through, and a grant for one action does not open another.
func TestEveryCommentMethodRequiresItsGrant(t *testing.T) {
	repo := commentRepoWithoutDB()
	ctx := context.Background()
	var zero auth.Grant

	calls := map[string]func(auth.Grant) error{
		"Find": func(g auth.Grant) error {
			_, err := repo.Find(ctx, g, "id")
			return err
		},
		"List": func(g auth.Grant) error {
			_, err := repo.List(ctx, g, database.Query{})
			return err
		},
		"Create": func(g auth.Grant) error {
			_, err := repo.Create(ctx, g, models.Comment{})
			return err
		},
		"Update": func(g auth.Grant) error {
			_, err := repo.Update(ctx, g, models.Comment{})
			return err
		},
		"Delete": func(g auth.Grant) error {
			return repo.Delete(ctx, g, "id")
		},
	}

	for name, call := range calls {
		t.Run(name+" with no grant", func(t *testing.T) {
			if err := call(zero); !errors.Is(err, auth.ErrForbidden) {
				t.Fatalf("error = %v, want ErrForbidden", err)
			}
		})
		t.Run(name+" with a grant for another action", func(t *testing.T) {
			if err := call(auth.SystemGrant("some.other.action", "t1")); !errors.Is(err, auth.ErrForbidden) {
				t.Fatalf("error = %v, want ErrForbidden", err)
			}
		})
	}
}

// TestTheCommentPolicyDeniesWhatItDoesNotKnow is the property that keeps a
// policy safe as it grows: an action nobody wrote a rule for is refused, rather
// than falling through to allowed.
//
// It uses an action that will never be opened, so it keeps passing after you open
// the real ones -- a test that breaks when you do what the generator told you to
// do is a test people delete.
func TestTheCommentPolicyDeniesWhatItDoesNotKnow(t *testing.T) {
	admin := auth.Subject{ID: "a1", Tenant: "t1", Roles: []string{"admin", "staff"}}

	err := (policies.CommentPolicy{}).Can(context.Background(), admin,
		"comment.action_that_does_not_exist", models.Comment{})

	if err == nil {
		t.Fatal("an action with no rule was allowed: the policy falls through to allowed")
	}
}

// TestCommentListRejectsSortOutsideTheAllowlist keeps the one door a column
// name could come through closed.
func TestCommentListRejectsSortOutsideTheAllowlist(t *testing.T) {
	repo := commentRepoWithoutDB()
	// CommentList, because listing is its own permission: a role may be
	// allowed to open the record it was given and not to page through every one.
	g := auth.SystemGrant(policies.CommentList, "t1")

	_, err := repo.List(context.Background(), g, database.Query{Sort: "1; DROP TABLE comments"})

	if !errors.Is(err, models.ErrCommentSort) {
		t.Fatalf("error = %v, want ErrCommentSort", err)
	}
}

// arandu:begin custom
// Tests for the rules you wrote go here, and survive regeneration.

// TestEveryCommentQueryBeyondTheFiveRequiresItsGrant covers the two thread
// queries this application added.
//
// PublicForPost is the one drawn under every article, for everybody, signed in
// or not. It is also the query that decides which comments are visible while
// they wait for review, so a Grant it never checked would be an unauthorized
// read on the most public page there is.
func TestEveryCommentQueryBeyondTheFiveRequiresItsGrant(t *testing.T) {
	repo := commentRepoWithoutDB()
	ctx := context.Background()
	var zero auth.Grant

	calls := map[string]func(auth.Grant) error{
		"ForPost": func(g auth.Grant) error {
			_, err := repo.ForPost(ctx, g, "p1")
			return err
		},
		"PublicForPost": func(g auth.Grant) error {
			_, err := repo.PublicForPost(ctx, g, "p1", "u1")
			return err
		},
	}

	for name, call := range calls {
		t.Run(name+" with no grant", func(t *testing.T) {
			if err := call(zero); !errors.Is(err, auth.ErrForbidden) {
				t.Fatalf("error = %v, want ErrForbidden", err)
			}
		})
		t.Run(name+" with a grant for another action", func(t *testing.T) {
			if err := call(auth.SystemGrant("some.other.action", "t1")); !errors.Is(err, auth.ErrForbidden) {
				t.Fatalf("error = %v, want ErrForbidden", err)
			}
		})
	}
}

// arandu:end custom
