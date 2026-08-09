package unit_test

import (
	"context"
	"testing"
	"time"

	"github.com/arandu-io/framework/security"

	models "github.com/arandu-io/examples/app/Models"
	policies "github.com/arandu-io/examples/app/Policies"
)

// TestAGuestReadsAPublishedPostAndNothingElse is the rule this blog exists to
// demonstrate: a page served to somebody with no account, decided by a policy
// rather than by a middleware that waved the request through.
//
// It is a table because the interesting part is what is refused. A test that
// only proved the allowed case would pass just as well against a policy that
// allowed everything.
func TestAGuestReadsAPublishedPostAndNothingElse(t *testing.T) {
	published := models.Post{ID: "p1", PublishedAt: time.Now().Add(-time.Hour)}
	draft := models.Post{ID: "p2"}

	for _, c := range []struct {
		what    string
		action  security.Action
		post    models.Post
		allowed bool
	}{
		// The two a reader is here for.
		{"the published listing", policies.PostPublicList, models.Post{}, true},
		{"a published post", policies.PostView, published, true},

		// PostView is asked twice, and the first time is about the zero value:
		// permission to look at all, before the row has been read. Refusing it
		// would mean a guest never reaches the second question.
		{"permission to look", policies.PostView, models.Post{}, true},

		// A draft is not a page yet.
		{"a draft", policies.PostView, draft, false},

		// The listing that pages through everything, drafts included. It is a
		// separate action for exactly this reason: one permission answering both
		// questions would mean something different depending on who asked.
		{"the full listing", policies.PostList, models.Post{}, false},

		// Writing, in every form.
		{"creating", policies.PostCreate, models.Post{}, false},
		{"updating", policies.PostUpdate, published, false},
		{"deleting", policies.PostDelete, published, false},

		// And an action nobody has defined. A policy that fell through to
		// allowed here would open itself the next time somebody added one.
		{"an unknown action", "post.something_new", models.Post{}, false},
	} {
		t.Run(c.what, func(t *testing.T) {
			err := (policies.PostPolicy{}).Can(context.Background(), security.Guest("t1"), c.action, c.post)
			switch {
			case c.allowed && err != nil:
				t.Errorf("a guest is refused %s: %v", c.what, err)
			case !c.allowed && err == nil:
				t.Errorf("a guest is allowed %s", c.what)
			}
		})
	}
}

// TestASubjectNobodyFilledInIsNotAGuest.
//
// The distinction is the whole reason security.Guest exists. An empty Subject is
// almost always a session that failed to load, and Authorize refuses it before
// consulting a policy -- so this test is about Authorize, not about the policy:
// what it pins is that a forgotten session cannot borrow the public path.
func TestASubjectNobodyFilledInIsNotAGuest(t *testing.T) {
	published := models.Post{ID: "p1", PublishedAt: time.Now().Add(-time.Hour)}

	if _, err := security.Authorize(context.Background(), policies.PostPolicy{},
		security.Subject{}, policies.PostView, published); err == nil {
		t.Fatal("an empty subject read a post: a forgotten session load is now a public request")
	}

	if _, err := security.Authorize(context.Background(), policies.PostPolicy{},
		security.Guest("t1"), policies.PostView, published); err != nil {
		t.Fatalf("a declared guest was refused a published post: %v", err)
	}
}
