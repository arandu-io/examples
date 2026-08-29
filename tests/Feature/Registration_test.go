package feature_test

import (
	"context"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/arandu-io/framework/arandutest"
	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/mail"

	"github.com/arandu-io/examples/bootstrap"
	"github.com/arandu-io/examples/tests"
)

// The registration flow, end to end, through the same handlers a browser
// reaches: a form, a purpose-bound code, a session, and a comment that is
// refused until the code is consumed.
//
// It is a feature test and not four unit tests because the interesting part is
// the seam between them. Every piece here has a unit test of its own -- the
// signer, the policy, the request validation -- and all three passed on the day
// the comment form was drawn for an account that could not use it.

const (
	newReader    = "newcomer@example.com"
	goodPassword = "a-long-enough-password"
)

// TestRegisteringDoesNotSignYouIn.
//
// A registration that opened a session would be a registration that made the
// verification code pointless: whoever filled in the form is already in, and the
// address is never confirmed by anybody.
func TestRegisteringDoesNotSignYouIn(t *testing.T) {
	client, _ := tests.App(t)

	client.Get("/auth/register").OK()
	res := client.Post("/auth/register", map[string]string{
		"name":                  "Grace Hopper",
		"email":                 newReader,
		"password":              goodPassword,
		"password_confirmation": goodPassword,
	})
	res.RedirectsTo("/auth/verify")

	// The notice, and it does not claim the account is ready.
	client.Get("/auth/verify").OK().See("Verify your email")
}

// TestAnUnconfirmedAccountReadsAndDoesNotWrite.
//
// The comment form is not drawn, and the reason is on the page. Drawing a form
// whose submission the policy is going to refuse is the worst of the three
// answers available -- worse than no form, and worse than a sentence saying why.
func TestAnUnconfirmedAccountReadsAndDoesNotWrite(t *testing.T) {
	client, db := tests.App(t)

	register(t, client)
	signIn(t, client)

	post := seedOnePost(t, db)

	body := client.Get("/posts/" + post).OK().Body()
	if strings.Contains(body, `name="body"`) {
		t.Error("an unconfirmed account was given a comment form the policy refuses")
	}
	if !strings.Contains(body, "Confirm your address") {
		t.Error("the page does not say why there is no form")
	}
}

// TestTheVerificationCodeIsPurposeBoundAndScoped.
//
// A code this application did not issue is refused, and a code it DID issue
// for password reset is refused by verification. Purpose, tenant, account and
// address all participate in the native CodeStore subject.
func TestTheVerificationCodeIsPurposeBoundAndScoped(t *testing.T) {
	client, _, box := tests.AppWithMailbox(t)
	register(t, client)

	client.Get("/auth/password").OK()
	client.Post("/auth/password/email", map[string]string{"email": newReader}).OK()
	resetCode := verificationCode(t, box)

	for _, code := range []string{"", "not-a-code", resetCode} {
		client.Get("/auth/verify?email=" + newReader).OK()
		// 422 and not 200. HTMX swaps the fragment of either, so a refused code
		// answered 200 leaves the browser, the log and every dashboard agreeing
		// that a forged or wrong-purpose code confirmed an address.
		body := client.Post("/auth/verify/confirm", map[string]string{
			"email": newReader, "email_code": code,
		}).
			Status(http.StatusUnprocessableEntity).Body()
		if !strings.Contains(body, "not valid") && !strings.Contains(body, "type the code") {
			t.Errorf("the code %q was not refused: %s", code, first200(body))
		}
	}
}

// TestConfirmingTheAddressOpensTheCommentForm.
//
// The whole reason the verification flow exists. A verified column that nothing
// consults is a column.
func TestConfirmingTheAddressOpensTheCommentForm(t *testing.T) {
	client, db, box := tests.AppWithMailbox(t)

	register(t, client)
	code := verificationCode(t, box)

	client.Get("/auth/verify?email=" + newReader).OK()
	client.Post("/auth/verify/confirm", map[string]string{
		"email": newReader, "email_code": code,
	}).OK().See("confirmed")
	// Signing in AFTER confirming is what puts the verified flag in the session.
	// Doing it the other way round leaves a session that is stale in the safe
	// direction, and that is deliberate -- see security.Subject.Verified.
	signIn(t, client)

	post := seedOnePost(t, db)
	body := client.Get("/posts/" + post).OK().Body()
	if !strings.Contains(body, `name="body"`) {
		t.Fatalf("confirmed account has no comment form (signed-in=%t, unverified=%t): %s",
			strings.Contains(body, "Grace Hopper"), strings.Contains(body, "Confirm your address"), first200(body))
	}
}

// TestTheVerificationCodeIsSingleUse.
//
// A code copied from the message must stop authorizing the instant it succeeds.
func TestTheVerificationCodeIsSingleUse(t *testing.T) {
	client, _, box := tests.AppWithMailbox(t)

	register(t, client)
	code := verificationCode(t, box)

	client.Get("/auth/verify?email=" + newReader).OK()
	client.Post("/auth/verify/confirm", map[string]string{
		"email": newReader, "email_code": code,
	}).OK().See("confirmed")
	client.Get("/auth/verify?email=" + newReader).OK()
	client.Post("/auth/verify/confirm", map[string]string{
		"email": newReader, "email_code": code,
	}).Status(http.StatusUnprocessableEntity).See("not valid")
}

// TestARegistrationCannotAskForARole.
//
// RegisterRequest has no Roles field, and the policy refuses a candidate that
// carries any. This proves the pair from outside: a form field named roles is
// ignored, and the account that comes out is ordinary.
func TestARegistrationCannotAskForARole(t *testing.T) {
	client, db := tests.App(t)

	client.Get("/auth/register").OK()
	client.Post("/auth/register", map[string]string{
		"name":                  "Mallory",
		"email":                 "mallory@example.com",
		"password":              goodPassword,
		"password_confirmation": goodPassword,
		// The field a naive handler would read.
		"roles": `["admin"]`,
	}).RedirectsTo("/auth/verify")

	var roles string
	if err := db.QueryRowContext(context.Background(),
		`SELECT roles FROM users WHERE email = ?`, "mallory@example.com").Scan(&roles); err != nil {
		t.Fatalf("the account was not created: %v", err)
	}
	if strings.Contains(roles, "admin") {
		t.Fatalf("a registration form minted an administrator: roles = %s", roles)
	}
}

// TestABadFormComesBackWithTheMessagesAndNotA200.
//
// HTMX swaps the fragment either way, so a 200 would leave the browser, the log
// and every dashboard agreeing that the registration succeeded.
func TestABadFormComesBackWithTheMessagesAndNotA200(t *testing.T) {
	client, _ := tests.App(t)

	client.Get("/auth/register").OK()
	res := client.Post("/auth/register", map[string]string{
		"name":                  "Ada",
		"email":                 "ada@example.com",
		"password":              "short",
		"password_confirmation": "different",
	})

	res.Status(http.StatusUnprocessableEntity)
	// What was typed comes back, except the passwords.
	res.See("ada@example.com").DontSee("different")
}

// The helpers below. They are here rather than in tests/testcase.go because they
// are about this flow: a project that does not have a comment section has no use
// for seedOnePost.

// register fills in the form with a valid account and follows the redirect.
func register(t *testing.T, client *arandutest.Client) {
	t.Helper()

	client.Get("/auth/register").OK()
	client.Post("/auth/register", map[string]string{
		"name":                  "Grace Hopper",
		"email":                 newReader,
		"password":              goodPassword,
		"password_confirmation": goodPassword,
	}).RedirectsTo("/auth/verify")
}

// signIn opens a session for the account register created.
func signIn(t *testing.T, client *arandutest.Client) {
	t.Helper()

	client.Get("/auth/login").OK()
	client.Post("/auth/login", map[string]string{
		"email":    newReader,
		"password": goodPassword,
	}).RedirectsTo("/")
}

// verificationCode is the purpose-bound code out of the last message.
//
// Read from the array transport rather than minted beside the application, so
// it is the exact code a person receives through the production path.
func verificationCode(t *testing.T, box *mail.Array) string {
	t.Helper()

	sent, ok := box.Last()
	if !ok {
		t.Fatal("no message was sent: the flow produced no verification code")
	}
	// The text part, because a message with none is filed as spam and this is
	// the assertion that notices when one stops being produced.
	if sent.Text == "" {
		t.Error("the message has no plain-text part")
	}
	found := emailCodePattern.FindString(sent.Text)
	if found == "" {
		t.Fatalf("no six-digit code in the message: %s", first200(sent.Text))
	}
	return found
}

// seedOnePost writes a published article to have a page to comment on.
//
// Straight SQL, which is the one place this project allows it: a test fixture is
// not an application code path, and going through the repository would mean
// building a Grant to prove something that has nothing to do with grants.
//
// The tenant is still written, and it is the one the application serves. Straight
// SQL skips the Grant, not the schema: a row inserted without a tenant is a row
// every query filters out, so the page under test answers 404.
func seedOnePost(t *testing.T, db *data.DB) string {
	t.Helper()

	const id = "00000000-0000-4000-8000-0000000000aa"
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO posts (id, tenant_id, title, slug, body, category_id, views, published_at, created_at)
		 VALUES (?, ?, ?, ?, ?, NULL, 0, ?, ?)`,
		id, bootstrap.Tenant(), "A post to answer", "a-post-to-answer", "The body.",
		time.Now().Add(-time.Hour), time.Now())
	if err != nil {
		t.Fatalf("seeding a post: %v", err)
	}
	return id
}

func first200(s string) string {
	if len(s) > 200 {
		return s[:200]
	}
	return s
}

// emailCodePattern finds the native single-use code in a text message.
var emailCodePattern = regexp.MustCompile(`\b[0-9]{6}\b`)
