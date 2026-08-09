package feature_test

import (
	"context"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/arandu-io/framework/data"

	"github.com/arandu-io/examples/tests"
)

// The whole product, in the order a person meets it.
//
// Every piece here has a test of its own. This one exists because the pieces
// were each correct and the seam between two of them was not: the sign-in screen
// rendered through a path that skipped the wiring, so it had no link to register
// and no link to recover a password -- and the markup for both was in the view,
// behind a condition that was never true.
//
// A test per handler cannot see that. A person following the product can.
func TestSomebodyArrivesAndEndsUpCommenting(t *testing.T) {
	client, db, box := tests.AppWithMailbox(t)

	const (
		email    = "newcomer@example.com"
		password = "the-first-password"
	)

	// 1. They land on the front page, signed out, and it offers a way in.
	front := client.Get("/").OK().Body()
	if !strings.Contains(front, "/auth/register") {
		t.Fatal("the front page offers no way to create an account")
	}

	// 2. The sign-in screen offers the two things somebody who cannot sign in
	//    needs: a way to register, and a way to recover a password.
	login := client.Get("/auth/login").OK().Body()
	for _, want := range []string{"/auth/register", "/auth/password"} {
		if !strings.Contains(login, want) {
			t.Fatalf("the sign-in screen has no link to %s -- the two things somebody who cannot sign in needs", want)
		}
	}

	// 3. They register. It does not sign them in: a registration that opened a
	//    session would make the verification link pointless.
	client.Get("/auth/register").OK()
	client.Post("/auth/register", map[string]string{
		"name": "Grace Hopper", "email": email,
		"password": password, "password_confirmation": password,
	}).RedirectsTo("/auth/verify")

	// 4. The message arrived, with both parts and a link.
	sent, ok := box.Last()
	if !ok {
		t.Fatal("registering sent nothing")
	}
	if sent.HTML == "" || sent.Text == "" {
		t.Error("the message is missing a part: an HTML-only message is filed as spam more often")
	}
	link := journeyLink.FindString(sent.Text)
	if link == "" {
		t.Fatalf("no confirmation link in the message:\n%s", sent.Text)
	}

	// 5. Before confirming, they can read and cannot write.
	post := seedJourneyPost(t, db)
	unconfirmed := client.Get("/posts/" + post).OK().Body()
	if strings.Contains(unconfirmed, `name="body"`) {
		t.Error("an unconfirmed account was given a comment form the policy refuses")
	}

	client.Get(link).OK().See("confirmed")

	// 6. Now they sign in, and the comment form is there.
	client.Get("/auth/login").OK()
	client.Post("/auth/login", map[string]string{"email": email, "password": password}).
		RedirectsTo("/")

	article := client.Get("/posts/" + post).OK().Body()
	if !strings.Contains(article, `name="body"`) {
		t.Fatal("a confirmed, signed-in reader has no comment form")
	}

	// 7. They comment. It is not public yet -- that is the moderation queue --
	//    and the answer says so rather than losing it silently.
	client.Post("/posts/"+post+"/comments", map[string]string{"body": "This is the first thing I have said here."}).
		RedirectsTo("/posts/" + post + "?said=1")

	own := client.Get("/posts/" + post).OK().Body()
	if !strings.Contains(own, "This is the first thing I have said here.") {
		t.Error("the author cannot see their own comment while it waits, so it looks lost")
	}

	// 8. They sign out, forget the password, and reset it.
	client.Post("/auth/logout", nil)

	client.Get("/auth/password").OK()
	client.Post("/auth/password/email", map[string]string{"email": email}).
		OK().See("on its way")

	reset, _ := box.Last()
	token := resetToken.FindStringSubmatch(reset.Text + reset.HTML)
	if token == nil {
		t.Fatalf("no reset token in the message:\n%s", reset.Text)
	}

	const changed = "the-second-password"
	client.Get("/auth/password/reset?token=" + token[1]).OK()
	client.Post("/auth/password/update", map[string]string{
		"token": token[1], "password": changed, "password_confirmation": changed,
	}).OK().See("has been changed")

	// 9. The new password works and the old one does not. This is what used to
	//    be a log line saying the example did not write the password.
	client.Get("/auth/login").OK()
	client.Post("/auth/login", map[string]string{"email": email, "password": changed}).
		RedirectsTo("/")

	client.Post("/auth/logout", nil)
	client.Get("/auth/login").OK()
	client.Post("/auth/login", map[string]string{"email": email, "password": password}).
		Status(401)
}

// TestTheHeaderDoesNotLinkToThePageYouAreOn.
//
// A header offering "Sign in" on the sign-in page links to what you are reading,
// and the one control that would help -- the way across to registering -- is the
// one it hides.
func TestTheHeaderDoesNotLinkToThePageYouAreOn(t *testing.T) {
	client, _ := tests.App(t)

	login := header(client.Get("/auth/login").OK().Body())
	if strings.Contains(login, "/auth/login") {
		t.Error("the sign-in page links to itself in the header")
	}
	if !strings.Contains(login, "/auth/register") {
		t.Error("the sign-in page does not offer the way across to registering")
	}

	register := header(client.Get("/auth/register").OK().Body())
	if strings.Contains(register, "/auth/register") {
		t.Error("the registration page links to itself in the header")
	}
	if !strings.Contains(register, "/auth/login") {
		t.Error("the registration page does not offer the way back to signing in")
	}
}

// header is the markup between <header> and </header>, which is where the
// navigation is. The rest of the page links to both of those addresses on
// purpose.
func header(body string) string {
	start := strings.Index(body, "<header")
	end := strings.Index(body, "</header>")
	if start < 0 || end < 0 {
		return ""
	}
	return body[start:end]
}

var (
	journeyLink = regexp.MustCompile(`https?://[^\s]*?/auth/verify/confirm\?token=[A-Za-z0-9_.\-]+`)
	resetToken  = regexp.MustCompile(`/auth/password/reset\?token=([A-Za-z0-9_\-]+)`)
)

// seedJourneyPost writes one published article to have somewhere to comment.
func seedJourneyPost(t *testing.T, db *data.DB) string {
	t.Helper()

	const id = "00000000-0000-4000-8000-0000000000bb"
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO posts (id, title, slug, body, category_id, views, published_at, created_at)
		 VALUES (?, ?, ?, ?, NULL, 0, ?, ?)`,
		id, "Something to answer", "something-to-answer", "The body.",
		time.Now().Add(-time.Hour), time.Now())
	if err != nil {
		t.Fatalf("seeding a post: %v", err)
	}
	return id
}
