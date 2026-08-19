package feature_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/arandu-io/framework/data"

	"github.com/arandu-io/examples/bootstrap"
	"github.com/arandu-io/examples/tests"
)

// TestAReaderWithNoAccountSeesThePublishedPosts is what this blog exists to
// demonstrate, written the way an application should write it.
//
// It is a feature test: it boots the application and makes requests, and what it
// proves is a decision that lives in a policy rather than in this file.
func TestAReaderWithNoAccountSeesThePublishedPosts(t *testing.T) {
	guest, _ := tests.App(t)

	// The listing answers, and it is the published one.
	guest.Get("/posts").OK().
		See("Posts").
		// The button belongs to somebody with an account. An empty URL draws no
		// control, which is how the policy reaches the markup as data.
		DontSee("Write one")

	// The comment form is not drawn for them, and the invitation is.
	//
	// DontSee is the half that matters here: a form that renders for a guest is
	// a form that posts and fails, and "it looked fine" is what a test that only
	// asserts presence reports.
	guest.Get("/posts").OK().DontSee("Say something")
}

// TestTheSignInScreenCarriesAToken.
//
// Every write in this application is refused without one, and a screen rendered
// without a token answers 200 and then refuses the submission with 419 -- which
// reads like a broken session rather than a missing field.
func TestTheSignInScreenCarriesAToken(t *testing.T) {
	c, _ := tests.App(t)

	c.Get("/auth/login").OK().See(`name="_csrf"`)
}

// TestAWriteWithoutATokenIsRefused is the other half, and it is the one that
// proves the protection is on rather than that the field is drawn.
func TestAWriteWithoutATokenIsRefused(t *testing.T) {
	// A fresh client has loaded no page, so it holds no token -- which is
	// exactly the request an attacker makes.
	c, _ := tests.App(t)
	c.Post("/auth/login", map[string]string{"email": "you@example.test"}).
		// 419, not 403, and the distinction is worth keeping: 403 is "you may
		// not", 419 is "your page is stale, load it again".
		Status(419)
}

// TestAGuestIsRefusedTheDraftBehindAKnownAddress is the refusal that the rest of
// this suite cannot prove.
//
// Reading one post asks the policy twice. The first question is about the zero
// value -- permission to look at posts at all -- and it is what produces the
// Grant the repository needs, before any row has been read. The second is about
// the row that came back, and it is the only one that can see PublishedAt: a
// guest may read a published post and not a draft, and that sentence is only
// true if somebody asks it about the post.
//
// Delete the second question and everything else here still passes. The listing
// hides the draft, because the published listing is a query of its own. The
// repository still refuses a grant issued for another action. The published post
// still renders. The only thing that changes is that the draft renders too, for
// anybody who knows its address -- which is what a search engine, a shared link
// or a sequential guess supplies.
//
// So the draft is asserted next to the published post rather than instead of it.
// The published one is the control: two refusals would read the same as a policy
// that refuses everybody, and that policy would pass a test with one case in it.
func TestAGuestIsRefusedTheDraftBehindAKnownAddress(t *testing.T) {
	guest, db := tests.App(t)

	published := seedPostForReading(t, db,
		"00000000-0000-4000-8000-0000000000c1",
		"Out in the open", "out-in-the-open", time.Now().Add(-time.Hour))
	draft := seedPostForReading(t, db,
		"00000000-0000-4000-8000-0000000000c2",
		"Still being written", "still-being-written", time.Time{})

	guest.Get("/posts/" + published).OK().See("Out in the open")

	// 403 and not 404, and the difference is the whole point: 404 would mean the
	// address resolved to nothing, which the seed above rules out. The row was
	// read, and the policy refused it.
	guest.Get("/posts/" + draft).Status(http.StatusForbidden).
		DontSee("Still being written")
}

// TestAPostOfAnotherTenantIsNeitherListedNorReadable.
//
// Every query in this application filters on the tenant carried by the Grant,
// and the filter is invisible while there is only one tenant in the database.
// This test puts a second one there.
//
// The address is asked for directly as well as through the listing, because the
// two are scoped by different queries -- Find and Published -- and a filter
// dropped from one of them is a filter the other still has. A test that only
// read the listing would pass against a Find that reads every tenant's rows,
// which is the query somebody reaches by a shared link.
//
// The answer is 404 and not 403, and that is the design rather than an accident:
// a row of another tenant answers exactly as an id that was never written does.
// Telling the two apart tells the asker that the id exists, which is the fact
// they were trying to learn.
func TestAPostOfAnotherTenantIsNeitherListedNorReadable(t *testing.T) {
	guest, db := tests.App(t)

	ours := seedPostForReading(t, db,
		"00000000-0000-4000-8000-0000000000d1",
		"Ours to read", "ours-to-read", time.Now().Add(-time.Hour))
	// The title carries no apostrophe, and that is not a detail. The assertion
	// below reads the rendered HTML, where a template escapes one into &#39; --
	// so a DontSee written with the punctuation a person would type looks for a
	// string the page cannot contain, and passes however much it leaked.
	theirs := seedPostForTenant(t, db,
		"00000000-0000-4000-8000-0000000000d2",
		"22222222-2222-4222-8222-222222222222",
		"Belonging to another tenant", "belonging-to-another-tenant", time.Now().Add(-time.Hour))

	// The listing, which is the query a reader arrives through.
	guest.Get("/posts").OK().
		See("Ours to read").
		DontSee("Belonging to another tenant")

	// And the address, which is the one a link supplies. The control above it
	// matters: without a post that does answer, a 404 here would be just as
	// consistent with an application that serves no posts at all.
	guest.Get("/posts/" + ours).OK().See("Ours to read")
	guest.Get("/posts/" + theirs).Status(http.StatusNotFound)
}

// seedPostForReading writes one post of the tenant this application serves.
//
// A zero published_at is a draft -- the column is a timestamp rather than a
// nullable one, so "not published" is the zero value and not NULL.
func seedPostForReading(t *testing.T, db *data.DB, id, title, slug string, published time.Time) string {
	t.Helper()
	return seedPostForTenant(t, db, id, bootstrap.Tenant(), title, slug, published)
}

// seedPostForTenant writes one post under the tenant it is given, and answers
// with its id.
//
// The tenant is a parameter rather than a constant because that is the only way
// to put a row in the database that the application must not return. Every seed
// written under bootstrap.Tenant proves what the application shows; a row under
// any other tenant is what proves what it hides.
//
// Straight SQL, which is the one place this project allows it: a test fixture is
// not an application code path, and going through the repository would mean
// building a Grant -- and a Grant only ever carries one tenant, so the row this
// test needs could not be written through one at all.
func seedPostForTenant(t *testing.T, db *data.DB, id, tenant, title, slug string, published time.Time) string {
	t.Helper()

	_, err := db.ExecContext(context.Background(),
		`INSERT INTO posts (id, tenant_id, title, slug, body, category_id, views, published_at, created_at)
		 VALUES (?, ?, ?, ?, ?, NULL, 0, ?, ?)`,
		id, tenant, title, slug, "The body.", published, time.Now())
	if err != nil {
		t.Fatalf("seeding a post: %v", err)
	}
	return id
}
