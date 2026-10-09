package feature_test

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
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
// the article rather than to a page the reader never opened. The last three are
// the sign-in, which is the published authentication screen and answers a
// wrong password through the same router, in the same three representations.

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

// guestBrowser is a signed-out browser holding the sign-in form, and the
// address of an account that exists.
func guestBrowser(t *testing.T, booted tests.Booted) (*harandutest.Client, string) {
	t.Helper()

	email := prepareAccount(t, booted.Client, booted.DB, "Grace Hopper", "")
	browser := harandutest.NewClient(t, booted.App.Kernel.Handler()).WithHeader("Accept", "text/html")
	browser.Get("/auth/login").AssertOk()
	return browser, email
}

// wrongPassword is a sign-in the handler refuses: the account exists and the
// password is not its own. The box is ticked, because a refusal that quietly
// unticks it is a refusal nothing on screen admits to.
func wrongPassword(email string) map[string]string {
	return map[string]string{"email": email, "password": "not-the-password-at-all", "remember": "1"}
}

func TestARefusedSignInGoesBackToTheFormWithTheMessage(t *testing.T) {
	booted := tests.Boot(t)
	browser, email := guestBrowser(t, booted)

	refused := browser.WithHeader("Referer", "/auth/login").Post("/auth/login", wrongPassword(email))
	browser.WithHeader("Referer", "")

	// The same 303 a rejected post gets, back to the form: a reload of what
	// follows asks for the form rather than posting the password again.
	refused.AssertStatus(http.StatusSeeOther).AssertRedirect("/auth/login")
	if got := refused.Header("Cache-Control"); !strings.Contains(got, "no-store") {
		t.Errorf("the answer to a refused sign-in is cacheable (Cache-Control %q)", got)
	}

	// The message is on the form, the address is still in its box, the box is
	// still ticked, and the password never comes back.
	form := browser.Get("/auth/login").AssertOk().
		AssertSee("invalid email or password").
		AssertSee(`value="` + email + `"`).
		AssertDontSee("not-the-password-at-all").
		GetContent()
	if !strings.Contains(form, `name="remember"`) || !regexpChecked.MatchString(form) {
		t.Errorf("the remember-me box came back unticked after a refused sign-in")
	}

	// The reload: the flash was spent on the first GET.
	browser.Get("/auth/login").AssertOk().AssertDontSee("invalid email or password")

	// And nobody was signed in: the guard still sends this browser to sign in.
	browser.Get("/dashboard").AssertStatus(http.StatusSeeOther).AssertRedirect("/auth/login")
}

// TestARefusedSignInFromHTMXIsANavigationBack: the sign-in form is boosted, so
// htmx makes the post. It is answered as every other rejected form here is --
// HX-Redirect back to the form, with no body -- which is why the layout no
// longer teaches htmx to swap a 422.
func TestARefusedSignInFromHTMXIsANavigationBack(t *testing.T) {
	booted := tests.Boot(t)
	browser, email := guestBrowser(t, booted)

	refused := browser.WithHeader("HX-Request", "true").WithHeader("Referer", "/auth/login").
		Post("/auth/login", wrongPassword(email))
	browser.WithHeader("HX-Request", "").WithHeader("Referer", "")

	refused.AssertStatus(http.StatusNoContent).AssertRedirect("/auth/login")
	if body := refused.GetContent(); body != "" {
		t.Errorf("the htmx answer to a refused sign-in has a body htmx would swap in before navigating: %q", body)
	}

	browser.Get("/auth/login").AssertOk().AssertSee("invalid email or password").AssertSee(`value="` + email + `"`)
}

// TestARefusedSignInFromAJSONClientIsAProblemDocument: a client that asked for
// JSON gets 422 and the message by field, and nothing is left in the flash.
func TestARefusedSignInFromAJSONClientIsAProblemDocument(t *testing.T) {
	booted := tests.Boot(t)
	browser, email := guestBrowser(t, booted)

	refused := browser.WithHeader("Accept", "application/json").Post("/auth/login", wrongPassword(email))
	browser.WithHeader("Accept", "text/html")

	refused.AssertStatus(http.StatusUnprocessableEntity)
	if got := refused.Header("Content-Type"); !strings.HasPrefix(got, "application/problem+json") {
		t.Fatalf("Content-Type = %q, want application/problem+json", got)
	}
	var problem struct {
		Status int                 `json:"status"`
		Errors map[string][]string `json:"errors"`
	}
	if err := json.Unmarshal([]byte(refused.GetContent()), &problem); err != nil {
		t.Fatalf("the body is not a problem document: %v\n%s", err, refused.GetContent())
	}
	if problem.Status != http.StatusUnprocessableEntity || len(problem.Errors["email"]) == 0 {
		t.Fatalf("problem = %+v, want status 422 and a message for email", problem)
	}
	if strings.Contains(refused.GetContent(), "not-the-password-at-all") {
		t.Error("the problem document carries the password that was typed")
	}

	browser.Get("/auth/login").AssertOk().AssertDontSee("invalid email or password")
}

// regexpChecked finds the remember-me box drawn ticked.
var regexpChecked = regexp.MustCompile(`name="remember"[^>]*\bchecked\b`)

// TestTheLayoutLeavesHtmxResponseHandlingAtItsDefault: nothing here answers a
// rejected form with a 422 to swap, the sign-in screens included, so the layout
// does not teach htmx to swap one. An entry that did would be a second way to
// answer the same rejection, and it would quietly keep working for any handler
// that went back to drawing its own refusal.
func TestTheLayoutLeavesHtmxResponseHandlingAtItsDefault(t *testing.T) {
	client, _ := tests.App(t)

	for _, page := range []string{"/", "/auth/login"} {
		body := client.Get(page).OK().See(`name="htmx-config"`).Body()
		if strings.Contains(body, "responseHandling") {
			t.Errorf("%s configures htmx's response handling; a rejected form is a redirect, not a 422 to swap", page)
		}
	}
}
