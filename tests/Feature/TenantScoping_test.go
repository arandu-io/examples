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

	// The two rows of theirs that point at ours.
	theirPostInOurSection = "00000000-0000-4000-8000-0000000ba004"
	theirCommentOnOurPost = "00000000-0000-4000-8000-0000000ba005"

	ourCategory = "00000000-0000-4000-8000-0000000bb001"
	ourPost     = "00000000-0000-4000-8000-0000000bb002"
	ourComment  = "00000000-0000-4000-8000-0000000bb003"
	ourSecond   = "00000000-0000-4000-8000-0000000bb004"
)

// scopedFixture writes one section, published posts in it and one approved
// comment, for each of two tenants -- and then two rows of theirs that name
// ours.
//
// The crossing pair is what makes a matching key a worse witness than the
// tenant: a post of theirs filed under OUR section id, and a comment of theirs
// hanging off OUR article. Nothing in the schema forbids either. category_id
// and post_id are plain id columns with no tenant beside them, so a row of one
// tenant can name a row of another, and every query that groups by a section or
// reads a thread by its post then has a row whose key matches and whose tenant
// does not.
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

	// The crossing pair, written last so that the assertions above it read in
	// the order the rows were meant. The slug is its own, because posts.slug is
	// unique across the whole table and not per tenant.
	seedPostInCategory(t, db, theirPostInOurSection, theirTenant, ourCategory,
		"Filed under our section", "filed-under-our-section")
	seedComment(t, db, theirCommentOnOurPost, theirTenant, ourPost, "u9", true)
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

// The shapes a tenant filter can be dropped from without any of the tests above
// noticing.
//
// Every method they ask about is a statement whose outermost WHERE carries the
// tenant, and that is the shape easiest to get right and easiest to watch. Four
// others are not: a nested query, where a second SELECT inside the statement
// needs a tenant of its own; an eager load, where children are fetched by the
// key of a parent that was already scoped; a pivot; and an aggregate, which
// returns no row of anybody's data and still reports how much of it there is.
//
// Three of the four are below. The fourth is not, and inventing it would be
// worse than leaving it out.
//
// # The pivot, and why there is no test for it
//
// A pivot is a row whose entire content is the pair of ids it joins, which is
// what makes it the hardest of the four: there is no column on it to be wrong
// about except the two keys, so the tenant has to be carried in from outside or
// it is not there at all.
//
// This application has no such table. Its migrations create posts, comments and
// categories; its relations are two plain id columns, posts.category_id and
// comments.post_id, each a one-to-many; and not one statement in
// app/Repositories joins a second table except through the EXISTS guards the
// nested-query test below covers. A many-to-many written here to have something
// to run against would be a test of its own fixture, and the shape would still
// be untested on the day a real one arrived.

// TestANestedQueryCarriesATenantOfItsOwn.
//
// The comment repository writes through EXISTS. Create is an INSERT ... SELECT
// guarded by EXISTS (SELECT 1 FROM posts WHERE id = ? AND tenant_id = ?),
// Update carries the same, and classifyUpdateMiss reads two of them to say
// which of the two rows was missing. Each of those inner SELECTs is a query in
// its own right and needs its own tenant: the outer statement being scoped says
// nothing about what the subquery reached.
//
// The other nested query in this application is the keyset predicate List
// builds for its cursor, and the three "List from another tenant's cursor"
// cases above are what watch that one.
func TestANestedQueryCarriesATenantOfItsOwn(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	scopedFixture(t, db)

	repo := repositories.NewCommentRepository(db)
	ours := bootstrap.Tenant()

	// The insert's EXISTS. A post id arrives in the request body, so this is
	// the subquery most likely to be handed another tenant's value.
	t.Run("Create against another tenant's post", func(t *testing.T) {
		//arandu:system-grant a fixture needs a Grant for a tenant no session in this test carries
		g := security.SystemGrant(policies.CommentCreate, ours)

		_, err := repo.Create(ctx, g,
			models.Comment{PostId: theirPost, Author: "u1", Body: "A remark."})
		if !errors.Is(err, models.ErrPostNotFound) {
			t.Fatalf("Create = %v, and a post of another tenant is not a post this tenant can be attached to", err)
		}
		if n := commentsOn(t, db, theirPost); n != 1 {
			t.Fatalf("another tenant's article carries %d comments and the fixture wrote it one", n)
		}
	})

	// The update's EXISTS, which is the same guard on the other statement: a
	// comment that cannot be created against their article must not be moved
	// onto it either.
	t.Run("Update onto another tenant's post", func(t *testing.T) {
		g := security.SystemGrant(policies.CommentUpdate, ours)

		_, err := repo.Update(ctx, g, models.Comment{
			ID: ourComment, PostId: theirPost, Author: "u1", Body: "Moved.", Approved: true,
		})
		if !errors.Is(err, models.ErrPostNotFound) {
			t.Fatalf("Update = %v, and the article it was moved onto belongs to another tenant", err)
		}
		if post, _ := commentRow(t, db, ourComment); post != ourPost {
			t.Fatalf("our comment now hangs off %q", post)
		}
	})

	// The subquery inside classifyUpdateMiss, which is the one with no outer
	// statement to hide behind: it runs only after the guarded UPDATE matched
	// nothing, and its answer is what the caller is told. If its EXISTS over
	// comments lost the tenant it would find their row, find our post beside
	// it, conclude that nothing was wrong, and return success on a write that
	// never happened.
	t.Run("Update of another tenant's comment", func(t *testing.T) {
		g := security.SystemGrant(policies.CommentUpdate, ours)

		_, err := repo.Update(ctx, g, models.Comment{
			ID: theirComment, PostId: ourPost, Author: "u1", Body: "Rewritten.", Approved: true,
		})
		if !errors.Is(err, models.ErrCommentNotFound) {
			t.Fatalf("Update = %v, and a comment of another tenant is one this tenant does not have", err)
		}
		if _, body := commentRow(t, db, theirComment); body != "A remark." {
			t.Fatalf("another tenant's comment now reads %q", body)
		}
	})
}

// TestAnEagerLoadedThreadHoldsNoRowOfAnotherTenant.
//
// The article page is an eager load written by hand: one query for the parents
// and one for the children of each, keyed by the parent's id and merged in
// memory. The parent set is scoped, and that is exactly what makes the second
// query easy to leave unscoped -- the ids being asked about are ours, so the
// filter looks redundant.
//
// It is not. A comment of another tenant carrying our post_id is a row nothing
// in the schema forbids, and a thread that trusted the key would draw it under
// our article for every reader.
func TestAnEagerLoadedThreadHoldsNoRowOfAnotherTenant(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	scopedFixture(t, db)

	posts := repositories.NewPostRepository(db)
	comments := repositories.NewCommentRepository(db)
	ours := bootstrap.Tenant()

	//arandu:system-grant a fixture needs a Grant for a tenant no session in this test carries
	listing := security.SystemGrant(policies.PostPublicList, ours)
	reading := security.SystemGrant(policies.CommentPublicList, ours)
	moderating := security.SystemGrant(policies.CommentList, ours)

	parents, err := posts.Published(ctx, listing, 50)
	if err != nil {
		t.Fatal(err)
	}
	assertOnlyOurs(t, ours, tenantsOfPosts(parents), 2)

	byPost := make(map[string][]models.Comment, len(parents))
	for _, p := range parents {
		found, err := comments.PublicForPost(ctx, reading, p.ID, "u1")
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range found {
			if c.TenantID != ours {
				t.Fatalf("a comment of tenant %q was loaded under our article %q", c.TenantID, p.ID)
			}
		}
		byPost[p.ID] = found
	}

	// The two halves together. One comment came back for our article, and the
	// table holds two for it -- so the number is a filter that ran and not a
	// row that was never written.
	if n := len(byPost[ourPost]); n != 1 {
		t.Fatalf("the thread of our article holds %d comments and one was written for it", n)
	}
	if n := commentsOn(t, db, ourPost); n != 2 {
		t.Fatalf("the table holds %d comments on our article, and the fixture wrote two: "+
			"the second is another tenant's, and leaving it out is what the thread is being asked to do", n)
	}

	// The moderation side loads the same children by the same key and answers a
	// wider question, so it is a second query with a filter of its own.
	moderated, err := comments.ForPost(ctx, moderating, ourPost)
	if err != nil {
		t.Fatal(err)
	}
	assertOnlyOurs(t, ours, tenantsOfComments(moderated), 1)
}

// TestAnAggregateCountsNoRowOfAnotherTenantUnderOurKey.
//
// An aggregate is the read that leaks without returning anything: CountByCategory
// hands back a number per section and not one row of anybody's data, and a
// number is still an answer about how much of it there is.
//
// The case that finds it is not a section of theirs appearing in the map -- the
// tests above already refuse that -- but a post of theirs filed under OUR
// section id. The key is ours, the row is not, and the GROUP BY has no opinion
// about the difference. Only the tenant predicate does.
func TestAnAggregateCountsNoRowOfAnotherTenantUnderOurKey(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	scopedFixture(t, db)

	repo := repositories.NewPostRepository(db)
	ours := bootstrap.Tenant()
	g := security.SystemGrant(policies.PostPublicList, ours)

	counts, err := repo.CountByCategory(ctx, g)
	if err != nil {
		t.Fatal(err)
	}

	if counts[ourCategory] != 2 {
		t.Fatalf("our section counts %d, and two posts of ours are filed in it", counts[ourCategory])
	}
	if n := postsInCategory(t, db, ourCategory); n != 3 {
		t.Fatalf("the table holds %d posts under our section id, and the fixture filed three: "+
			"the third is another tenant's, and the count above is only worth reading because it is there", n)
	}
	if len(counts) != 1 {
		t.Fatalf("the counts cover %d sections and this tenant has one", len(counts))
	}

	// The listing behind the same section id, because a count that is right and
	// a listing that is wrong is the same leak arriving one page later.
	rows, err := repo.PublishedInCategory(ctx, g, ourCategory, 50)
	if err != nil {
		t.Fatal(err)
	}
	assertOnlyOurs(t, ours, tenantsOfPosts(rows), 2)
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

// commentsOn and postsInCategory count what the table holds under one key,
// whatever tenant wrote it.
//
// They are the second half of every case about a crossing row. A thread that
// answers with one comment and a count that answers with two posts prove
// nothing on their own: the number is only a filter that ran if the row it left
// out is really there, and a repository cannot be asked about a row it is meant
// to refuse.
func commentsOn(t *testing.T, db *data.DB, postID string) int {
	t.Helper()

	var n int
	if err := db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM comments WHERE post_id = ?`, postID).Scan(&n); err != nil {
		t.Fatalf("counting the thread: %v", err)
	}
	return n
}

func postsInCategory(t *testing.T, db *data.DB, categoryID string) int {
	t.Helper()

	var n int
	if err := db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM posts WHERE category_id = ?`, categoryID).Scan(&n); err != nil {
		t.Fatalf("counting the section: %v", err)
	}
	return n
}

// commentRow reads the two columns the nested-query cases assert about: the
// article a comment hangs off, and what it says.
func commentRow(t *testing.T, db *data.DB, id string) (postID, body string) {
	t.Helper()

	if err := db.QueryRowContext(context.Background(),
		`SELECT post_id, body FROM comments WHERE id = ?`, id).Scan(&postID, &body); err != nil {
		t.Fatalf("reading the comment: %v", err)
	}
	return postID, body
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
