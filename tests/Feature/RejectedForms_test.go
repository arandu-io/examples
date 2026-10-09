package feature_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	harandutest "github.com/arandu-io/hesape/arandutest"

	"github.com/arandu-io/examples/tests"
)

// A rejected form is answered by the router, in the representation the request
// asked for, and by nothing else. No controller here draws the form again with
// a 422: the service returns validation.Errors, the action returns them as they
// are, and a page goes back where it came from with the messages and what was
// typed in the flash, an htmx request gets the same answer spelled as
// HX-Redirect, and a client that asked for JSON gets a 422 problem document.
//
// The three tests below are the three representations of one rejected post.
// The fourth is the comment box on an article, which is the form a reader
// actually meets: it posts to an address of its own and has to come back to
// the article rather than to a page the reader never opened.

// browserAs is a signed-in browser that can say what kind of request it makes.
//
// It is the hesape client rather than the one tests.Boot hands out, because a
// rejected form is answered by what the request says about itself -- Accept,
// HX-Request and the Referer it came from -- and only this client sends
// headers. It says it wants HTML, which is what a browser navigating says, and
// what tells the page somebody is about to read from the fragments and assets
// that page asks for: the flash is spent on the first only.
func browserAs(t *testing.T, booted tests.Booted, name, role string) *harandutest.Client {
	t.Helper()

	email := prepareAccount(t, booted.Client, booted.DB, name, role)
	browser := harandutest.NewClient(t, booted.App.Kernel.Handler()).WithHeader("Accept", "text/html")
	browser.Get("/auth/login").AssertOk()
	browser.Post("/auth/login", map[string]string{"email": email, "password": accountPassword}).
		AssertRedirect("/")
	return browser
}

// postsWithSlug is how many posts carry the slug, read past every policy.
func postsWithSlug(t *testing.T, booted tests.Booted, slug string) int {
	t.Helper()
	var n int
	if err := booted.DB.QueryRowContext(context.Background(),
		`SELECT count(*) FROM posts WHERE slug = ?`, slug).Scan(&n); err != nil {
		t.Fatalf("counting the posts: %v", err)
	}
	return n
}

// untitledPost is a post the request refuses: the title is blank once trimmed.
var untitledPost = map[string]string{"title": "  ", "slug": "kept-as-typed", "body": "Kept as typed."}

func TestARejectedPostGoesBackToTheFormWithTheMessage(t *testing.T) {
	booted := tests.Boot(t)
	browser := browserAs(t, booted, "Grace Hopper", "admin")

	browser.Get("/posts/create").AssertOk()
	rejected := browser.WithHeader("Referer", "/posts/create").Post("/posts", untitledPost)
	browser.WithHeader("Referer", "")

	// 303 and nothing else. It tells the browser to GET the address it is sent
	// to, so the entry the history keeps is that GET: a reload asks for the
	// form again instead of posting it.
	rejected.AssertStatus(http.StatusSeeOther).AssertRedirect("/posts/create")
	if got := rejected.Header("Cache-Control"); !strings.Contains(got, "no-store") {
		t.Errorf("the answer to a rejected form is cacheable (Cache-Control %q), and it carries what one person typed", got)
	}

	browser.Get("/posts/create").AssertOk().AssertSee("Kept as typed.").AssertSee("is required")

	// The reload: the same GET, and the flash has already been spent on the
	// first one.
	browser.Get("/posts/create").AssertOk().AssertDontSee("Kept as typed.").AssertDontSee("is required")
	if got := postsWithSlug(t, booted, "kept-as-typed"); got != 0 {
		t.Fatalf("a rejected post and a reload left %d posts stored, want none", got)
	}
}

// TestARejectedPostFromHTMXIsANavigationBack: the form posts with hx-post, and
// htmx discards a 4xx body by default. What it does follow is HX-Redirect, as a
// full navigation -- so the answer is the same redirect back to the form,
// spelled the way htmx reads it, with no body to swap.
func TestARejectedPostFromHTMXIsANavigationBack(t *testing.T) {
	booted := tests.Boot(t)
	browser := browserAs(t, booted, "Grace Hopper", "admin")

	browser.Get("/posts/create").AssertOk()
	rejected := browser.WithHeader("HX-Request", "true").WithHeader("Referer", "/posts/create").
		Post("/posts", untitledPost)
	browser.WithHeader("HX-Request", "").WithHeader("Referer", "")

	rejected.AssertStatus(http.StatusNoContent).AssertRedirect("/posts/create")
	if body := rejected.GetContent(); body != "" {
		t.Errorf("the htmx answer to a rejected form has a body htmx would swap in before navigating: %q", body)
	}

	// The navigation htmx makes is an ordinary GET, and it finds the messages.
	browser.Get("/posts/create").AssertOk().AssertSee("Kept as typed.").AssertSee("is required")
	if got := postsWithSlug(t, booted, "kept-as-typed"); got != 0 {
		t.Fatalf("a rejected post left %d posts stored, want none", got)
	}
}

// TestARejectedPostFromAJSONClientIsAProblemDocument: a client that asked for
// JSON has no form to go back to. It gets 422 and the messages by field, and
// nothing is left in the flash for a page nobody will load.
func TestARejectedPostFromAJSONClientIsAProblemDocument(t *testing.T) {
	booted := tests.Boot(t)
	browser := browserAs(t, booted, "Grace Hopper", "admin")

	browser.Get("/posts/create").AssertOk()
	rejected := browser.WithHeader("Accept", "application/json").Post("/posts", untitledPost)
	browser.WithHeader("Accept", "text/html")

	rejected.AssertStatus(http.StatusUnprocessableEntity)
	if got := rejected.Header("Content-Type"); !strings.HasPrefix(got, "application/problem+json") {
		t.Fatalf("Content-Type = %q, want application/problem+json", got)
	}
	var problem struct {
		Status int                 `json:"status"`
		Errors map[string][]string `json:"errors"`
	}
	if err := json.Unmarshal([]byte(rejected.GetContent()), &problem); err != nil {
		t.Fatalf("the body is not a problem document: %v\n%s", err, rejected.GetContent())
	}
	if problem.Status != http.StatusUnprocessableEntity || len(problem.Errors["title"]) == 0 {
		t.Fatalf("problem = %+v, want status 422 and a message for title", problem)
	}
	if _, typed := problem.Errors["body"]; typed {
		t.Errorf("a field that passed has a message: %v", problem.Errors["body"])
	}

	browser.Get("/posts/create").AssertOk().AssertDontSee("Kept as typed.").AssertDontSee("is required")
	if got := postsWithSlug(t, booted, "kept-as-typed"); got != 0 {
		t.Fatalf("a rejected post left %d posts stored, want none", got)
	}
}

// TestARejectedEditComesBackWithWhatWasTypedOverTheStoredRecord: the edit form
// starts at the stored record, and a rejected update has to come back with what
// was typed instead -- the stored title in the box would look like the edit
// had been thrown away.
func TestARejectedEditComesBackWithWhatWasTypedOverTheStoredRecord(t *testing.T) {
	booted := tests.Boot(t)
	browser := browserAs(t, booted, "Grace Hopper", "admin")
	post := seedJourneyPost(t, booted.DB)
	edit := "/posts/" + post + "/edit"

	browser.Get(edit).AssertOk().AssertSee("Something to answer")
	browser.WithHeader("HX-Request", "true").WithHeader("Referer", edit).
		Put("/posts/"+post, map[string]string{"title": "Retitled", "slug": "something-to-answer", "body": "  "}).
		AssertStatus(http.StatusNoContent).AssertRedirect(edit)
	browser.WithHeader("HX-Request", "").WithHeader("Referer", "")

	browser.Get(edit).AssertOk().AssertSee(`value="Retitled"`).AssertSee("is required")
	var title string
	if err := booted.DB.QueryRowContext(context.Background(),
		`SELECT title FROM posts WHERE id = ?`, post).Scan(&title); err != nil {
		t.Fatalf("reading the post: %v", err)
	}
	if title != "Something to answer" {
		t.Fatalf("a rejected edit changed the stored title to %q", title)
	}
}

// TestARefusedCommentComesBackUnderTheArticle: the comment box lives on the
// article and posts to an address of its own. A refusal sends the reader back
// to the article they were reading, with the message under the box -- not to a
// comment form on a page of its own that they never opened.
func TestARefusedCommentComesBackUnderTheArticle(t *testing.T) {
	booted := tests.Boot(t)
	browser := browserAs(t, booted, "Ada Lovelace", "")
	post := seedJourneyPost(t, booted.DB)
	article := "/posts/" + post

	browser.Get(article).AssertOk().AssertSee(`name="body"`)
	browser.WithHeader("HX-Request", "true").WithHeader("Referer", article).
		Post(article+"/comments", map[string]string{"body": "   "}).
		AssertStatus(http.StatusNoContent).AssertRedirect(article)
	browser.WithHeader("HX-Request", "").WithHeader("Referer", "")

	browser.Get(article).AssertOk().AssertSee("Something to answer").AssertSee("is required")

	var stored int
	if err := booted.DB.QueryRowContext(context.Background(),
		`SELECT count(*) FROM comments WHERE post_id = ?`, post).Scan(&stored); err != nil {
		t.Fatalf("counting the comments: %v", err)
	}
	if stored != 0 {
		t.Fatalf("a refused comment left %d comments stored, want none", stored)
	}
}
