package feature_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/security"

	models "github.com/arandu-io/examples/app/Models"
	policies "github.com/arandu-io/examples/app/Policies"
	repositories "github.com/arandu-io/examples/app/Repositories"
	"github.com/arandu-io/examples/bootstrap"
)

// Every read of this application filters on the tenant carried by the Grant, and
// the filter is invisible while there is one tenant in the database.
//
// TestAPostOfAnotherTenantIsNeitherListedNorReadable proves it for the two
// queries a guest arrives through. This file proves it for the rest, at the
// repository rather than through the browser, because most of them are not
// reachable from a page a guest can open -- and the filter each one carries is
// its own. Dropping it from Find is a change Published does not notice.
//
// It was written by removing each predicate in turn and watching the suite stay
// green. Every method below is one that did.

// theirTenant is a second tenant, put in the database so that a query which
// forgot to scope itself has something to return. Its rows are the ones no call
// here may see.
const theirTenant = "22222222-2222-4222-8222-222222222222"

// The ids of the fixture, and the order they sort in is the point.
//
// The other tenant's rows sort before ours and are written first, so a query
// that lost its tenant reaches theirs rather than ours. Written the other way
// round, an unscoped SELECT still answers with our row -- because it is the
// first one -- and the test passes for a reason that has nothing to do with the
// predicate it is about.
const (
	theirCategory = "00000000-0000-4000-8000-0000000ba001"
	theirPost     = "00000000-0000-4000-8000-0000000ba002"
	theirComment  = "00000000-0000-4000-8000-0000000ba003"

	ourCategory = "00000000-0000-4000-8000-0000000bb001"
	ourPost     = "00000000-0000-4000-8000-0000000bb002"
	ourComment  = "00000000-0000-4000-8000-0000000bb003"
	ourSecond   = "00000000-0000-4000-8000-0000000bb004"
)

// scopedFixture writes one section, published posts in it and one approved
// comment, for each of two tenants.
func scopedFixture(t *testing.T, db *data.DB) {
	t.Helper()

	// Theirs first, and the section carries the same slug as ours: a slug is
	// unique per tenant and not globally, so a lookup that dropped the tenant
	// has two rows to choose between and answers with whichever it reached
	// first -- which is now theirs.
	seedCategory(t, db, theirCategory, theirTenant, "Dispatches", "reports")
	seedPostInCategory(t, db, theirPost, theirTenant, theirCategory,
		"Belonging to another tenant", "belonging-to-another")
	seedComment(t, db, theirComment, theirTenant, theirPost, "u9", true)

	ours := bootstrap.Tenant()
	seedCategory(t, db, ourCategory, ours, "Reports", "reports")
	seedPostInCategory(t, db, ourPost, ours, ourCategory, "Ours to read", "ours-to-read")
	// A second post of ours, so that a page has somewhere to continue from and
	// the cursor cases below have something to assert about.
	seedPostInCategory(t, db, ourSecond, ours, ourCategory, "Ours as well", "ours-as-well")
	seedComment(t, db, ourComment, ours, ourPost, "u1", true)
}

// TestNoPostQueryReadsAnotherTenantsRows.
//
// Six queries and six separate predicates. The two the browser reaches are
// covered by the test next door; these are the four it does not, and each of
// them was watched to survive losing its tenant with the suite green.
//
// CountByCategory is here because an aggregate is a read like any other: it
// returns not one row of anybody's data and still reports how much of it there
// is, which is what a leak in a report is made of.
func TestNoPostQueryReadsAnotherTenantsRows(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	scopedFixture(t, db)

	repo := repositories.NewPostRepository(db)
	ours := bootstrap.Tenant()

	t.Run("Find", func(t *testing.T) {
		//arandu:system-grant a fixture needs a Grant for a tenant no session in this test carries
		g := security.SystemGrant(policies.PostView, ours)

		if _, err := repo.Find(ctx, g, ourPost); err != nil {
			t.Fatalf("our own post was not readable, so a refusal below would prove nothing: %v", err)
		}
		if _, err := repo.Find(ctx, g, theirPost); err == nil {
			t.Fatal("another tenant's post was read by id")
		}
	})

	t.Run("List", func(t *testing.T) {
		g := security.SystemGrant(policies.PostList, ours)
		found, err := repo.List(ctx, g, data.Query{Limit: 50})
		if err != nil {
			t.Fatal(err)
		}
		assertOnlyOurs(t, ours, tenantsOfPosts(found), 2)
	})

	// The cursor is the id of the last row read and it arrives from the client,
	// so it is resolved by a subquery of its own -- which needs the tenant as
	// much as the outer query does. Handed an id from another tenant, a scoped
	// subquery resolves nothing and the page is empty; an unscoped one lets that
	// id decide where our page starts.
	t.Run("List from another tenant's cursor", func(t *testing.T) {
		g := security.SystemGrant(policies.PostList, ours)
		found, err := repo.List(ctx, g, data.Query{Limit: 50, Cursor: theirPost})
		if err != nil {
			t.Fatal(err)
		}
		if len(found) != 0 {
			t.Fatalf("a cursor from another tenant paged %d of our rows", len(found))
		}
	})

	t.Run("Published", func(t *testing.T) {
		g := security.SystemGrant(policies.PostPublicList, ours)
		found, err := repo.Published(ctx, g, 50)
		if err != nil {
			t.Fatal(err)
		}
		assertOnlyOurs(t, ours, tenantsOfPosts(found), 2)
	})

	// Asked for THEIR section, because ours narrows the answer by itself: a
	// section id belongs to one tenant, so a query that lost the tenant and kept
	// the section still returns our rows when our section is the one asked for.
	// The id a reader's slug resolves to is the one that has to be refused.
	t.Run("PublishedInCategory", func(t *testing.T) {
		g := security.SystemGrant(policies.PostPublicList, ours)

		found, err := repo.PublishedInCategory(ctx, g, ourCategory, 50)
		if err != nil {
			t.Fatal(err)
		}
		assertOnlyOurs(t, ours, tenantsOfPosts(found), 2)

		found, err = repo.PublishedInCategory(ctx, g, theirCategory, 50)
		if err != nil {
			t.Fatal(err)
		}
		if len(found) != 0 {
			t.Fatalf("another tenant's section listed %d posts", len(found))
		}
	})

	t.Run("CountByCategory", func(t *testing.T) {
		g := security.SystemGrant(policies.PostPublicList, ours)
		counts, err := repo.CountByCategory(ctx, g)
		if err != nil {
			t.Fatal(err)
		}
		if counts[ourCategory] != 2 {
			t.Fatalf("our own section counts %d and not 2", counts[ourCategory])
		}
		if len(counts) != 1 {
			t.Fatalf("the counts cover %d sections, and this tenant has one: an aggregate crossed tenants", len(counts))
		}
	})

	// The three statements that write. A read that crosses tenants shows
	// somebody a row; a write that crosses tenants changes one, and there is no
	// version of that which is recoverable by refreshing the page.
	t.Run("IncrementViews", func(t *testing.T) {
		g := security.SystemGrant(policies.PostView, ours)
		if err := repo.IncrementViews(ctx, g, theirPost); err != nil {
			// The statement matches nothing and reports no error, which is
			// right: it is a counter, not an answer. What is asserted is the
			// row, below.
			t.Fatalf("incrementing: %v", err)
		}
		if views := viewsOf(t, db, theirPost); views != 0 {
			t.Fatalf("another tenant's post counts %d views: the update crossed tenants", views)
		}
	})

	t.Run("Update", func(t *testing.T) {
		g := security.SystemGrant(policies.PostUpdate, ours)
		_, err := repo.Update(ctx, g, models.Post{ID: theirPost, Title: "Rewritten", Slug: "rewritten"})
		if err == nil {
			t.Fatal("another tenant's post was rewritten")
		}
		if title := titleOf(t, db, theirPost); title != "Belonging to another tenant" {
			t.Fatalf("another tenant's post now reads %q", title)
		}
	})

	t.Run("Delete", func(t *testing.T) {
		g := security.SystemGrant(policies.PostDelete, ours)
		if err := repo.Delete(ctx, g, theirPost); err == nil {
			t.Fatal("another tenant's post was deleted")
		}
		if title := titleOf(t, db, theirPost); title == "" {
			t.Fatal("another tenant's post is gone")
		}
	})
}

// TestNoCommentQueryReadsAnotherTenantsRows.
//
// PublicForPost is the one drawn under every article, and it is asked for a post
// id that arrives from the address bar. Without the tenant, an id from another
// tenant returns its conversation to whoever guessed it.
func TestNoCommentQueryReadsAnotherTenantsRows(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	scopedFixture(t, db)

	repo := repositories.NewCommentRepository(db)
	ours := bootstrap.Tenant()

	t.Run("Find", func(t *testing.T) {
		g := security.SystemGrant(policies.CommentView, ours)
		if _, err := repo.Find(ctx, g, ourComment); err != nil {
			t.Fatalf("our own comment was not readable: %v", err)
		}
		if _, err := repo.Find(ctx, g, theirComment); err == nil {
			t.Fatal("another tenant's comment was read by id")
		}
	})

	t.Run("List", func(t *testing.T) {
		g := security.SystemGrant(policies.CommentList, ours)
		found, err := repo.List(ctx, g, data.Query{Limit: 50})
		if err != nil {
			t.Fatal(err)
		}
		assertOnlyOurs(t, ours, tenantsOfComments(found), 1)
	})

	t.Run("List from another tenant's cursor", func(t *testing.T) {
		g := security.SystemGrant(policies.CommentList, ours)
		found, err := repo.List(ctx, g, data.Query{Limit: 50, Cursor: theirComment})
		if err != nil {
			t.Fatal(err)
		}
		if len(found) != 0 {
			t.Fatalf("a cursor from another tenant paged %d of our rows", len(found))
		}
	})

	t.Run("ForPost", func(t *testing.T) {
		g := security.SystemGrant(policies.CommentList, ours)
		found, err := repo.ForPost(ctx, g, theirPost)
		if err != nil {
			t.Fatal(err)
		}
		if len(found) != 0 {
			t.Fatalf("the thread of another tenant's post returned %d comments", len(found))
		}
	})

	t.Run("PublicForPost", func(t *testing.T) {
		g := security.SystemGrant(policies.CommentPublicList, ours)
		found, err := repo.PublicForPost(ctx, g, theirPost, "u9")
		if err != nil {
			t.Fatal(err)
		}
		if len(found) != 0 {
			t.Fatalf("the public thread of another tenant's post returned %d comments", len(found))
		}
	})
}

// TestNoCategoryQueryReadsAnotherTenantsRows.
//
// The section list is on every page of the site, and FindBySlug takes the half
// of the address a reader can type. The fixture gives both tenants a section
// with the slug "reports" on purpose: without the tenant in the lookup there are
// two rows that match and the answer is whichever the engine reached first.
func TestNoCategoryQueryReadsAnotherTenantsRows(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	scopedFixture(t, db)

	repo := repositories.NewCategoryRepository(db)
	ours := bootstrap.Tenant()

	t.Run("Find", func(t *testing.T) {
		g := security.SystemGrant(policies.CategoryView, ours)
		if _, err := repo.Find(ctx, g, ourCategory); err != nil {
			t.Fatalf("our own section was not readable: %v", err)
		}
		if _, err := repo.Find(ctx, g, theirCategory); err == nil {
			t.Fatal("another tenant's section was read by id")
		}
	})

	t.Run("List", func(t *testing.T) {
		g := security.SystemGrant(policies.CategoryList, ours)
		found, err := repo.List(ctx, g, data.Query{Limit: 50})
		if err != nil {
			t.Fatal(err)
		}
		assertOnlyOurs(t, ours, tenantsOfCategories(found), 1)
	})

	t.Run("List from another tenant's cursor", func(t *testing.T) {
		g := security.SystemGrant(policies.CategoryList, ours)
		found, err := repo.List(ctx, g, data.Query{Limit: 50, Cursor: theirCategory})
		if err != nil {
			t.Fatal(err)
		}
		if len(found) != 0 {
			t.Fatalf("a cursor from another tenant paged %d of our rows", len(found))
		}
	})

	t.Run("All", func(t *testing.T) {
		g := security.SystemGrant(policies.CategoryList, ours)
		found, err := repo.All(ctx, g)
		if err != nil {
			t.Fatal(err)
		}
		assertOnlyOurs(t, ours, tenantsOfCategories(found), 1)
	})

	t.Run("FindBySlug", func(t *testing.T) {
		g := security.SystemGrant(policies.CategoryView, ours)
		found, err := repo.FindBySlug(ctx, g, "reports")
		if err != nil {
			t.Fatalf("our own section was not found by its slug: %v", err)
		}
		if found.TenantID != ours {
			t.Fatalf("the slug resolved to the section of tenant %q, which carries the same slug", found.TenantID)
		}
	})
}

// TestTheTenantOnAWriteComesFromTheGrant.
//
// The candidate arrives from a request body, and a request that could choose its
// tenant could write into somebody else's. Each repository overwrites the field
// rather than trusting it, and this is what says so: the value below is another
// tenant's, and the row that comes back is ours.
func TestTheTenantOnAWriteComesFromTheGrant(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	ours := bootstrap.Tenant()
	var postID string

	t.Run("post", func(t *testing.T) {
		g := security.SystemGrant(policies.PostCreate, ours)
		created, err := repositories.NewPostRepository(db).Create(ctx, g,
			models.Post{TenantID: theirTenant, Title: "Ours", Slug: "ours"})
		if err != nil {
			t.Fatal(err)
		}
		if created.TenantID != ours {
			t.Fatalf("the post was filed under %q, which the caller chose", created.TenantID)
		}
		postID = created.ID
	})

	t.Run("comment", func(t *testing.T) {
		g := security.SystemGrant(policies.CommentCreate, ours)
		created, err := repositories.NewCommentRepository(db).Create(ctx, g,
			models.Comment{TenantID: theirTenant, PostId: postID, Author: "u1", Body: "A remark."})
		if err != nil {
			t.Fatal(err)
		}
		if created.TenantID != ours {
			t.Fatalf("the comment was filed under %q, which the caller chose", created.TenantID)
		}
	})

	t.Run("category", func(t *testing.T) {
		g := security.SystemGrant(policies.CategoryCreate, ours)
		created, err := repositories.NewCategoryRepository(db).Create(ctx, g,
			models.Category{TenantID: theirTenant, Name: "Reports", Slug: "reports"})
		if err != nil {
			t.Fatal(err)
		}
		if created.TenantID != ours {
			t.Fatalf("the section was filed under %q, which the caller chose", created.TenantID)
		}
	})
}

// assertOnlyOurs fails when a result carries a tenant that is not the one asked
// for, and when the expected number of rows did not come back.
//
// Both halves, because a query that returns nothing satisfies "no other tenant's
// rows" perfectly, and an application that serves nothing is not what is being
// proved.
func assertOnlyOurs(t *testing.T, ours string, tenants []string, want int) {
	t.Helper()

	if len(tenants) != want {
		t.Fatalf("the query returned %d rows and this tenant has %d", len(tenants), want)
	}
	for _, tenant := range tenants {
		if tenant != ours {
			t.Fatalf("a row of tenant %q came back for tenant %q", tenant, ours)
		}
	}
}

func tenantsOfPosts(found []models.Post) []string {
	out := make([]string, 0, len(found))
	for _, p := range found {
		out = append(out, p.TenantID)
	}
	return out
}

func tenantsOfComments(found []models.Comment) []string {
	out := make([]string, 0, len(found))
	for _, c := range found {
		out = append(out, c.TenantID)
	}
	return out
}

func tenantsOfCategories(found []models.Category) []string {
	out := make([]string, 0, len(found))
	for _, c := range found {
		out = append(out, c.TenantID)
	}
	return out
}

// viewsOf and titleOf read straight from the table, because the repository
// cannot be asked about a row it is meant to refuse -- and a row that is meant
// to be untouched has to be looked at to prove it.
func viewsOf(t *testing.T, db *data.DB, id string) int {
	t.Helper()

	var views int
	if err := db.QueryRowContext(context.Background(),
		`SELECT views FROM posts WHERE id = ?`, id).Scan(&views); err != nil {
		t.Fatalf("reading the counter: %v", err)
	}
	return views
}

// titleOf answers with the empty string when the row is gone, which is what the
// deletion case asks about.
func titleOf(t *testing.T, db *data.DB, id string) string {
	t.Helper()

	var title string
	err := db.QueryRowContext(context.Background(),
		`SELECT title FROM posts WHERE id = ?`, id).Scan(&title)
	if errors.Is(err, sql.ErrNoRows) {
		return ""
	}
	if err != nil {
		t.Fatalf("reading the title: %v", err)
	}
	return title
}

// The three seeds. Straight SQL, which is the one place this project allows it:
// a Grant only ever carries one tenant, so the rows these tests need could not
// be written through a repository at all.

func seedCategory(t *testing.T, db *data.DB, id, tenant, name, slug string) {
	t.Helper()

	_, err := db.ExecContext(context.Background(),
		`INSERT INTO categories (id, tenant_id, name, slug, description, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		id, tenant, name, slug, "A section.", time.Now())
	if err != nil {
		t.Fatalf("seeding a category: %v", err)
	}
}

func seedPostInCategory(t *testing.T, db *data.DB, id, tenant, category, title, slug string) {
	t.Helper()
	seedPostInCategoryAt(t, db, id, tenant, category, title, slug, time.Now().Add(-time.Hour))
}

// seedPostInCategoryAt is the same, with the publication moment named. The zero
// time is a draft: published_at is a timestamp rather than a nullable column, so
// "not published" is the zero value and not NULL.
func seedPostInCategoryAt(t *testing.T, db *data.DB, id, tenant, category, title, slug string, published time.Time) {
	t.Helper()

	_, err := db.ExecContext(context.Background(),
		`INSERT INTO posts (id, tenant_id, title, slug, body, category_id, views, published_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, 0, ?, ?)`,
		id, tenant, title, slug, "The body.", category, published, time.Now())
	if err != nil {
		t.Fatalf("seeding a post: %v", err)
	}
}

func seedComment(t *testing.T, db *data.DB, id, tenant, post, author string, approved bool) {
	t.Helper()

	_, err := db.ExecContext(context.Background(),
		`INSERT INTO comments (id, tenant_id, post_id, author, body, approved, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, tenant, post, author, "A remark.", approved, time.Now())
	if err != nil {
		t.Fatalf("seeding a comment: %v", err)
	}
}
