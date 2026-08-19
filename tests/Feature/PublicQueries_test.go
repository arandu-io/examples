package feature_test

import (
	"context"
	"testing"
	"time"

	"github.com/arandu-io/framework/security"

	models "github.com/arandu-io/examples/app/Models"
	policies "github.com/arandu-io/examples/app/Policies"
	repositories "github.com/arandu-io/examples/app/Repositories"
	"github.com/arandu-io/examples/bootstrap"
)

// The two public queries carry a predicate that is not about the tenant: one
// keeps a draft out of the listing, the other keeps a comment awaiting review
// out of a stranger's page. Both are written in the SQL rather than applied to
// the rows afterwards, because a filter applied in Go is one somebody removes
// while tidying and the rows are already in memory when they do.
//
// Both were removed, one at a time, and the suite stayed green. These are the
// assertions that changes.

// TestThePublicListingHidesADraft.
//
// A draft in the published listing is the failure this predicate exists for, and
// it has happened: the first spelling of it was `published_at IS NOT NULL`,
// which answers true for every draft, because the column is a timestamp and the
// zero value is written as 0001-01-01 -- a real date, and very much not null.
//
// The published post is asserted beside the draft rather than instead of it. Two
// absences read the same as a query that returns nothing, and that query would
// pass a test with only the absence in it.
func TestThePublicListingHidesADraft(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	ours := bootstrap.Tenant()

	const section = "00000000-0000-4000-8000-0000000000e1"
	seedCategory(t, db, section, ours, "Reports", "reports")
	seedPostInCategory(t, db, "00000000-0000-4000-8000-0000000000e2", ours, section,
		"Out in the open", "out-in-the-open")
	seedPostInCategoryAt(t, db, "00000000-0000-4000-8000-0000000000e3", ours, section,
		"Still being written", "still-being-written", time.Time{})

	repo := repositories.NewPostRepository(db)
	//arandu:system-grant a fixture needs a Grant for a tenant no session in this test carries
	g := security.SystemGrant(policies.PostPublicList, ours)

	t.Run("the listing", func(t *testing.T) {
		found, err := repo.Published(ctx, g, 50)
		if err != nil {
			t.Fatal(err)
		}
		assertPublishedOnly(t, found)
	})

	t.Run("the section", func(t *testing.T) {
		found, err := repo.PublishedInCategory(ctx, g, section, 50)
		if err != nil {
			t.Fatal(err)
		}
		assertPublishedOnly(t, found)
	})

	// The count is what draws the number beside a section in the navigation, and
	// a draft counted there advertises an article nobody can open.
	t.Run("the section count", func(t *testing.T) {
		counts, err := repo.CountByCategory(ctx, g)
		if err != nil {
			t.Fatal(err)
		}
		if counts[section] != 1 {
			t.Fatalf("the section counts %d and one of its two posts is a draft", counts[section])
		}
	})
}

// TestTheThreadHidesWhatIsWaitingForReview.
//
// What is public about a comment thread is the thread, not the queue. A reader
// sees what has been approved, plus their own while it waits -- which is the one
// exception, and it exists so that somebody who just wrote a comment is not told
// their words vanished.
//
// The author is read from the Grant's subject by the service and never from an
// argument a request could set, so the two readers below are the whole story:
// one who wrote nothing and one who did.
func TestTheThreadHidesWhatIsWaitingForReview(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	ours := bootstrap.Tenant()

	const post = "00000000-0000-4000-8000-0000000000f1"
	seedCategory(t, db, "00000000-0000-4000-8000-0000000000f0", ours, "Reports", "reports")
	seedPostInCategory(t, db, post, ours, "00000000-0000-4000-8000-0000000000f0",
		"Out in the open", "out-in-the-open")

	seedComment(t, db, "00000000-0000-4000-8000-0000000000f2", ours, post, "u1", true)
	seedComment(t, db, "00000000-0000-4000-8000-0000000000f3", ours, post, "u9", false)

	repo := repositories.NewCommentRepository(db)
	g := security.SystemGrant(policies.CommentPublicList, ours)

	t.Run("a stranger sees only what was approved", func(t *testing.T) {
		found, err := repo.PublicForPost(ctx, g, post, "u2")
		if err != nil {
			t.Fatal(err)
		}
		if len(found) != 1 {
			t.Fatalf("the thread returned %d comments, and one of the two is waiting for review", len(found))
		}
		if !found[0].Approved {
			t.Fatal("a comment awaiting review was drawn for somebody who did not write it")
		}
	})

	t.Run("its author sees their own as well", func(t *testing.T) {
		found, err := repo.PublicForPost(ctx, g, post, "u9")
		if err != nil {
			t.Fatal(err)
		}
		if len(found) != 2 {
			t.Fatalf("the author of a pending comment saw %d comments and not both: "+
				"somebody who just wrote one is told their words vanished", len(found))
		}
	})
}

// assertPublishedOnly fails when a draft came back, and when the published post
// did not.
func assertPublishedOnly(t *testing.T, found []models.Post) {
	t.Helper()

	if len(found) != 1 {
		t.Fatalf("the query returned %d posts, and one of the two is a draft", len(found))
	}
	if !found[0].Published() {
		t.Fatal("the one post that came back is the draft")
	}
}
