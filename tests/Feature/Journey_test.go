package feature_test

import (
	"context"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/arandu-io/framework/arandutest"
	"github.com/arandu-io/framework/data"

	"github.com/arandu-io/examples/bootstrap"
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
	//    session would make the verification code pointless.
	client.Get("/auth/register").OK()
	client.Post("/auth/register", map[string]string{
		"name": "Grace Hopper", "email": email,
		"password": password, "password_confirmation": password,
	}).RedirectsTo("/auth/verify")

	// 4. The message arrived, with both parts and a single-use code.
	sent, ok := box.Last()
	if !ok {
		t.Fatal("registering sent nothing")
	}
	if sent.HTML == "" || sent.Text == "" {
		t.Error("the message is missing a part: an HTML-only message is filed as spam more often")
	}
	code := emailCodePattern.FindString(sent.Text)
	if code == "" {
		t.Fatalf("no confirmation code in the message:\n%s", sent.Text)
	}

	// 5. Before confirming, they can read and cannot write.
	post := seedJourneyPost(t, db)
	unconfirmed := client.Get("/posts/" + post).OK().Body()
	if strings.Contains(unconfirmed, `name="body"`) {
		t.Error("an unconfirmed account was given a comment form the policy refuses")
	}

	client.Get("/auth/verify?email=" + email).OK()
	client.Post("/auth/verify/confirm", map[string]string{
		"email": email, "email_code": code,
	}).OK().See("confirmed")

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
	//
	// The answer is asserted rather than ignored. Signing out posts a form, so
	// it needs the CSRF token off the last page loaded -- and when there is no
	// such page the answer is 419 and the session survives. That happened here,
	// silently, for as long as the test client kept a signed-out cookie beside
	// the live one: the sign-out failed, the client stayed signed in, and
	// nothing downstream noticed.
	client.Post("/auth/logout", nil).Status(303).RedirectsTo("/auth/login")

	client.Get("/auth/password").OK()
	client.Post("/auth/password/email", map[string]string{"email": email}).
		OK().See("on its way")

	reset, _ := box.Last()
	resetCode := emailCodePattern.FindString(reset.Text)
	if resetCode == "" {
		t.Fatalf("no reset code in the message:\n%s", reset.Text)
	}

	// The response that confirms delivery is already the form that accepts the
	// code, and it keeps the address the person typed.
	form := client.Get("/auth/password/reset?email=" + email).OK().Body()
	if !strings.Contains(form, email) {
		t.Errorf("the reset form does not carry the address the code was sent to:\n%s", form)
	}

	const changed = "the-second-password"
	client.Post("/auth/password/update", map[string]string{
		"email_code": resetCode, "email": email,
		"password": changed, "password_confirmation": changed,
	}).OK().See("has been changed")

	// And the code is spent atomically. Its subject also carries the password
	// fingerprint, so any password change invalidates every older code.
	client.Get("/auth/password/reset?email=" + email).OK()
	client.Post("/auth/password/update", map[string]string{
		"email_code": resetCode, "email": email,
		"password": "a-third-password-entirely", "password_confirmation": "a-third-password-entirely",
	}).Status(422).See("not valid")

	// 9. The new password works and the old one does not.
	client.Get("/auth/login").OK()
	client.Post("/auth/login", map[string]string{"email": email, "password": changed}).
		RedirectsTo("/")

	// A rendered page first: the body of the redirect a sign-in answers with
	// carries no CSRF token, and the sign-out form is what carries one.
	client.Get("/").OK()
	client.Post("/auth/logout", nil).Status(303).RedirectsTo("/auth/login")

	client.Get("/auth/login").OK()
	client.Post("/auth/login", map[string]string{"email": email, "password": password}).
		Status(401)
}

// A reset ends every session of the account, and that is the point of having
// one: the person asking for a reset is often asking because somebody else is
// signed in as them. A reset that leaves those sessions open leaves whoever
// forced it exactly where they were, holding a cookie that still works.
//
// It is asserted from the session that did the resetting, because that one is
// covered by the same rule: the code arrives in an inbox, and there is no
// session on that request worth keeping.
func TestAPasswordResetEndsTheSessionsThatWereAlreadyOpen(t *testing.T) {
	client, db, box := tests.AppWithMailbox(t)

	const email = "grace.hopper@example.com"
	signInAs(t, client, db, "Grace Hopper", "")
	client.Get("/dashboard").OK()

	client.Get("/auth/password").OK()
	client.Post("/auth/password/email", map[string]string{"email": email}).OK()

	sent, _ := box.Last()
	code := emailCodePattern.FindString(sent.Text)
	if code == "" {
		t.Fatalf("no reset code in the message:\n%s", sent.Text)
	}

	const changed = "a-completely-new-password"
	client.Get("/auth/password/reset?email=" + email).OK()
	client.Post("/auth/password/update", map[string]string{
		"email_code": code, "email": email,
		"password": changed, "password_confirmation": changed,
	}).OK().See("has been changed")

	res := client.Get("/dashboard")
	res.Status(303)
	if got := res.Header("Location"); got != "/auth/login" {
		t.Errorf("the session that was open before the reset still opens the dashboard (sent to %q): whoever "+
			"forced the reset is still signed in", got)
	}
}

// The confirmation screen, from the outside: it has a route, the route has a
// handler, and the form it draws posts somewhere.
//
// It had none of the three. PasswordConfirmURL was declared, read by
// auth/passwords/confirm.kyse.go and assigned nowhere, so the form rendered
// action="" and posted to itself -- into a 404 -- while the command that
// publishes it printed "every screen has a route and every route has a handler".
func TestTheConfirmationScreenHasARouteAndPostsSomewhere(t *testing.T) {
	client, db := tests.App(t)
	signInAs(t, client, db, "Grace Hopper", "")

	body := client.Get("/auth/password/confirm").OK().Body()

	if strings.Contains(body, `action=""`) {
		t.Error("the confirmation form posts to nowhere: PasswordConfirmURL is not filled in")
	}
	if !strings.Contains(body, "/auth/password/confirm") {
		t.Errorf("the confirmation form does not post to the address that answers it:\n%s", body)
	}
}

func TestConfirmingWithTheRightPasswordIsAcceptedAndTheWrongOneIsNot(t *testing.T) {
	client, db := tests.App(t)
	signInAs(t, client, db, "Grace Hopper", "")

	client.Get("/auth/password/confirm").OK()
	client.Post("/auth/password/confirm", map[string]string{"password": "not-the-password"}).
		Status(401).See("not the password")

	client.Get("/auth/password/confirm").OK()
	client.Post("/auth/password/confirm", map[string]string{"password": "a-password-that-passes"}).
		Status(303)
}

// Nobody signed in has a password to confirm on. The screen is behind the
// session guard, so a guest is sent to sign in rather than to a form that would
// have to invent an answer.
func TestTheConfirmationScreenIsNotOpenToSomebodyWithNoSession(t *testing.T) {
	client, _ := tests.App(t)

	res := client.Get("/auth/password/confirm")

	res.Status(303)
	if got := res.Header("Location"); got != "/auth/login" {
		t.Errorf("a visitor with no session was sent to %q, want the sign-in screen", got)
	}
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

// seedJourneyPost writes one published article to have somewhere to comment.
//
// The tenant is written explicitly, and it is the one the application serves. A
// row inserted without it is a row every query filters out, so the journey would
// fail at the first page with "not found" rather than at the assertion that
// means something.
func seedJourneyPost(t *testing.T, db *data.DB) string {
	t.Helper()

	const id = "00000000-0000-4000-8000-0000000000bb"
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO posts (id, tenant_id, title, slug, body, category_id, views, published_at, created_at)
		 VALUES (?, ?, ?, ?, ?, NULL, 0, ?, ?)`,
		id, bootstrap.Tenant(), "Something to answer", "something-to-answer", "The body.",
		time.Now().Add(-time.Hour), time.Now())
	if err != nil {
		t.Fatalf("seeding a post: %v", err)
	}
	return id
}

// TestTheHeaderSaysWhoYouAre.
//
// A struct literal per controller drifts: a brand linking to href="", a "Sign
// in" button that reloads the page you were already reading, no way to create an
// account, the guest half of the bar shown to a signed-in administrator. Every
// one of those is a field somebody forgot to fill in.
//
// So this asserts the header from the three sides it has: a guest, a reader,
// and an administrator -- on a screen from each controller, because a defect
// like that is never in the layout. It is in who filled it in.
func TestTheHeaderSaysWhoYouAre(t *testing.T) {
	client, db := tests.App(t)
	post := seedJourneyPost(t, db)

	// The two screens a guest can reach. /categories asks for a session and
	// answers 303 to one, which is why the signed-in test below is the one that
	// reads it -- and /categories is where the drift was worst.
	for _, path := range []string{"/", "/posts/" + post} {
		bar := header(client.Get(path).OK().Body())
		if bar == "" {
			t.Fatalf("%s has no header at all", path)
		}
		if strings.Contains(bar, `href=""`) {
			t.Errorf("%s: the header links to nowhere -- a field nobody filled in", path)
		}
		for _, want := range []string{"/auth/login", "/auth/register"} {
			if !strings.Contains(bar, want) {
				t.Errorf("%s: a guest is not offered %s", path, want)
			}
		}
		if strings.Contains(bar, "/admin") || strings.Contains(bar, "/dashboard") {
			t.Errorf("%s: a guest is offered an address that would refuse them", path)
		}
	}
}

// A reader gets their own area and never the moderation queue; an administrator
// gets both. The header must never offer an address that answers 403 -- so the
// role is decided in the controller and the markup only ever sees a string.
func TestTheHeaderOffersOnlyWhatWouldOpen(t *testing.T) {
	for _, who := range []struct {
		role     string
		wants    []string
		refuses  []string
		greeting string
	}{
		{role: "", wants: []string{"/dashboard"}, refuses: []string{"/admin"}, greeting: "Grace Hopper"},
		{role: "admin", wants: []string{"/dashboard", "/admin"}, greeting: "Ada Lovelace"},
	} {
		t.Run("role="+who.role, func(t *testing.T) {
			client, db := tests.App(t)
			signInAs(t, client, db, who.greeting, who.role)

			// Two screens from two controllers. The layout was never the
			// defect; who filled it in was, and that is per controller.
			for _, path := range []string{"/", "/categories"} {
				bar := header(client.Get(path).OK().Body())
				if strings.Contains(bar, `href=""`) {
					t.Errorf("%s: the header links to nowhere", path)
				}
				if !strings.Contains(bar, who.greeting) {
					t.Errorf("%s: the header does not greet by name", path)
				}
			}

			bar := header(client.Get("/").OK().Body())
			for _, want := range who.wants {
				if !strings.Contains(bar, want) {
					t.Errorf("no link to %s", want)
				}
			}
			for _, refuse := range who.refuses {
				if strings.Contains(bar, refuse) {
					t.Errorf("offered %s, which would answer 403", refuse)
				}
			}
			if !strings.Contains(bar, who.greeting) {
				t.Errorf("the header does not greet by name:\n%s", bar)
			}
			if uuidInHeader.MatchString(bar) {
				t.Errorf("the header greets somebody with the id out of their session:\n%s", bar)
			}
			if strings.Contains(bar, "/auth/login") || strings.Contains(bar, "/auth/register") {
				t.Error("a signed-in person is offered the guest half of the bar")
			}
		})
	}
}

var uuidInHeader = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-`)

// The guards, from outside, on the routes this application actually registers.
//
// They have tests of their own in the framework, and those prove the middleware
// works. This one proves it is MOUNTED, which is the half that was missing: the
// route table documented middleware.RequireRole in a comment for months while
// /dashboard answered 200 to anybody who asked.
func TestTheDashboardIsNotOpenToSomebodyWithNoSession(t *testing.T) {
	client, _ := tests.App(t)

	res := client.Get("/dashboard")

	res.Status(303).DontSee("Sign out")
	if got := res.Header("Location"); got != "/auth/login" {
		t.Errorf("a visitor with no session was sent to %q, want the sign-in screen", got)
	}
}

func TestTheDashboardOpensForSomebodySignedIn(t *testing.T) {
	client, db := tests.App(t)
	signInAs(t, client, db, "Grace Hopper", "")

	client.Get("/dashboard").OK()
}

// The guest guard, on both screens that exist to bring somebody in. Rendering
// either of them to a person who has a session reads to them as having been
// signed out, and what they do next is sign in on top of a session that never
// went away.
func TestSomebodySignedInIsSentAwayFromTheScreensThatSignPeopleIn(t *testing.T) {
	client, db := tests.App(t)
	signInAs(t, client, db, "Grace Hopper", "")

	for _, path := range []string{"/auth/login", "/auth/register"} {
		res := client.Get(path)
		res.Status(303)
		if got := res.Header("Location"); got != "/" {
			t.Errorf("%s sent a signed-in person to %q, want /", path, got)
		}
	}
}

// The moderation area, to a reader. 403 and not the sign-in screen: they are
// signed in, so there is nothing for them to do there -- and not 404 either,
// because the page exists and pretending otherwise sends them hunting a typo.
//
// The Policy is what refuses them the comments themselves, and it still runs.
// This is the door, not the decision about a record.
func TestTheModerationAreaIsNotOpenToAReader(t *testing.T) {
	client, db := tests.App(t)
	signInAs(t, client, db, "Grace Hopper", "")

	client.Get("/admin/comments").Status(403)
}

func TestTheModerationAreaOpensForAnAdministrator(t *testing.T) {
	client, db := tests.App(t)
	signInAs(t, client, db, "Ada Lovelace", "admin")

	client.Get("/admin/comments").OK()
}

// signInAs registers somebody, confirms their address and opens a session.
//
// The role is written to the row directly. It is not a form field, and it must
// not become one: RegisterRequest has no field for it and the policy refuses a
// candidate carrying any, which is what stops registration from being a way to
// make yourself an administrator.
func signInAs(t *testing.T, client *arandutest.Client, db *data.DB, name, role string) {
	t.Helper()

	const password = "a-password-that-passes"
	email := strings.ToLower(strings.ReplaceAll(name, " ", ".")) + "@example.com"

	client.Get("/auth/register").OK()
	client.Post("/auth/register", map[string]string{
		"name": name, "email": email,
		"password": password, "password_confirmation": password,
	}).RedirectsTo("/auth/verify")

	// Confirmed and given the role in one write, because what is under test is
	// the header and not the verification flow -- that has its own test.
	//
	// The column holds JSON, which is how the repository writes it. A bare
	// "admin" is written happily by SQLite and then fails to unmarshal on the
	// next read, so the account simply stops being able to sign in -- silently,
	// three asserts later.
	roles := "[]"
	if role != "" {
		roles = `["` + role + `"]`
	}
	if _, err := db.ExecContext(context.Background(),
		`UPDATE users SET verified_at = ?, roles = ? WHERE email = ?`,
		time.Now(), roles, email); err != nil {
		t.Fatalf("preparing the account: %v", err)
	}

	client.Get("/auth/login").OK()
	client.Post("/auth/login", map[string]string{"email": email, "password": password}).
		RedirectsTo("/")
}

// TestSigningInLandsOnThePageTheGuardTurnedAwayFrom.
//
// Every guard writes the address it refused into a signed cookie, and the
// sign-in handler is the only thing that spends it. The starter kit's handler
// redirected to "/" instead -- while the framework's own sign-in handler, the
// one the kit REPLACES, already spent it. So publishing the kit into a project
// took the behaviour away, in the same shape the missing guest guard had: the
// guards went on remembering and nothing ever read it.
//
// From outside, it is the difference between following a link to one page,
// signing in, and being there -- and signing in, landing on the front page, and
// going to find the page again.
func TestSigningInLandsOnThePageTheGuardTurnedAwayFrom(t *testing.T) {
	client, db := tests.App(t)

	const (
		email    = "returning@example.com"
		password = "a-password-that-passes"
	)

	client.Get("/auth/register").OK()
	client.Post("/auth/register", map[string]string{
		"name": "Grace Hopper", "email": email,
		"password": password, "password_confirmation": password,
	}).RedirectsTo("/auth/verify")
	if _, err := db.ExecContext(context.Background(),
		`UPDATE users SET verified_at = ? WHERE email = ?`, time.Now(), email); err != nil {
		t.Fatalf("preparing the account: %v", err)
	}

	// Registration opens no session, so this is a guest following a link to a
	// guarded page. The guard turns them away and remembers where they meant to
	// go -- nothing else in the request knows it by the time a password is typed.
	client.Get("/dashboard").Status(303)

	client.Get("/auth/login").OK()
	res := client.Post("/auth/login", map[string]string{"email": email, "password": password})

	res.Status(303)
	if got := res.Header("Location"); got != "/dashboard" {
		t.Errorf("signing in sent them to %q, and they were on their way to /dashboard.\n"+
			"The address the guard remembered was written and never spent, so following a link "+
			"while signed out ends on the front page with the page still to find.", got)
	}
}

// TestTheScreenThatAsksForAPasswordAgainDrawsTheHeaderOfSomebodySignedIn.
//
// /auth/password/confirm sits behind the session guard: it is only ever read by
// somebody who is signed in. Every screen the kit publishes built its chrome
// through one function that left Authenticated at false, so the layout drew the
// guest half -- the screen whose entire job is asking somebody to prove they are
// still there offered them a Login button, a Register button and no way out.
func TestTheScreenThatAsksForAPasswordAgainDrawsTheHeaderOfSomebodySignedIn(t *testing.T) {
	client, db := tests.App(t)
	signInAs(t, client, db, "Grace Hopper", "")

	bar := header(client.Get("/auth/password/confirm").OK().Body())
	if bar == "" {
		t.Fatal("the confirmation screen has no header at all")
	}
	if !strings.Contains(bar, "Grace Hopper") {
		t.Errorf("the header does not greet the person it is asking for a password:\n%s", bar)
	}
	if uuidInHeader.MatchString(bar) {
		t.Errorf("the header greets them with the id out of their session:\n%s", bar)
	}
	if !strings.Contains(bar, "/auth/logout") {
		t.Errorf("a signed-in person is offered no way to sign out:\n%s", bar)
	}
	if strings.Contains(bar, "/auth/login") || strings.Contains(bar, "/auth/register") {
		t.Errorf("a signed-in person is offered the guest half of the bar:\n%s", bar)
	}
}
