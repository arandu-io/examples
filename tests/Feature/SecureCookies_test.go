package feature_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/arandu-io/hesape/auth"
	"github.com/arandu-io/hesape/otp"

	"github.com/arandu-io/examples/bootstrap"
)

// TestEveryCookieTheApplicationWritesFollowsTheOneSecureDecision boots the
// application the way `aru serve` does, under the three ways a deployment
// answers whether its cookies are HTTPS-only, and reads the answer off the
// cookies the wired application writes.
//
// Three of them are built in bootstrap/app.go -- the CSRF guest cookie a page
// sets for a visitor with no session, the session cookie a sign-in sets, and
// the authentication kit's pending second-factor cookie -- and all three must
// carry what the framework's loader decided: a guest cookie that disagreed with
// the session cookie is a site where signing in works and every guest form
// answers 419, or the other way round. The flash cookie is the kernel's own
// and follows the same value there.
func TestEveryCookieTheApplicationWritesFollowsTheOneSecureDecision(t *testing.T) {
	for _, c := range []struct {
		name   string
		appEnv string
		secure string
		want   bool
	}{
		{name: "dev", appEnv: "dev", want: false},
		{name: "prod, nothing declared", appEnv: "prod", want: true},
		{name: "prod, declared false to serve over http", appEnv: "prod", secure: "false", want: false},
	} {
		t.Run(c.name, func(t *testing.T) {
			sqliteEnv(t)
			if err := bootstrap.Dispatch("migrate", nil); err != nil {
				t.Fatalf("migrate: %v", err)
			}
			// After the migration: outside development migrate refuses a lock
			// no other replica can see, and the schema is not what this test is
			// about.
			t.Setenv("APP_ENV", c.appEnv)
			t.Setenv("APP_DEBUG", "")
			t.Setenv("APP_URL", "http://app.internal:8080")
			t.Setenv("SESSION_SECURE_COOKIE", c.secure)
			app := bootedInstance(t)

			page := httptest.NewRecorder()
			app.Kernel.Handler().ServeHTTP(page, httptest.NewRequest(http.MethodGet, "/", nil))
			guest := page.Result().Cookies()
			if len(guest) == 0 {
				t.Fatalf("GET / answered %d and set no cookie, want the CSRF guest cookie", page.Code)
			}
			for _, cookie := range guest {
				if cookie.Secure != c.want {
					t.Errorf("the guest cookie %s is written with Secure=%t, want %t", cookie.Name, cookie.Secure, c.want)
				}
			}

			signIn := httptest.NewRecorder()
			subject := auth.Subject{ID: "33333333-3333-4333-8333-333333333333", Tenant: bootstrap.Tenant()}
			if _, err := app.Sessions.Start(context.Background(), signIn, subject); err != nil {
				t.Fatalf("starting a session: %v", err)
			}
			session := signIn.Result().Cookies()
			if len(session) != 1 {
				t.Fatalf("a started session wrote %d cookies, want exactly the session cookie", len(session))
			}
			for _, cookie := range session {
				if cookie.Secure != c.want {
					t.Errorf("the session cookie %s is written with Secure=%t, want %t", cookie.Name, cookie.Secure, c.want)
				}
			}

			// The third is the authentication kit's own: the short signed cookie
			// that carries a password-checked sign-in to the second factor. The
			// kit is handed the value in bootstrap/app.go, and an account with
			// two-factor authentication is what makes it write the cookie.
			const (
				email    = "two-factor@example.test"
				password = "a-long-enough-password"
			)
			ctx := context.Background()
			user, err := app.Users.Register(ctx, bootstrap.Tenant(), "Ana", email, password)
			if err != nil {
				t.Fatalf("registering: %v", err)
			}
			provisioning, err := app.TwoFactor.Begin(ctx, user.Subject(), "examples")
			if err != nil {
				t.Fatalf("beginning two-factor setup: %v", err)
			}
			code, err := otp.Default().Generate(provisioning.Secret, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			if _, err := app.TwoFactor.Confirm(ctx, user.Subject(), code); err != nil {
				t.Fatalf("confirming two-factor setup: %v", err)
			}
			var pending *http.Cookie
			for _, cookie := range signInOn(t, app.Kernel.Handler(), email, password) {
				if cookie.Name == "two-factor-pending" && cookie.MaxAge > 0 {
					pending = cookie
				}
			}
			if pending == nil {
				t.Fatal("a password-checked sign-in to an account with two-factor authentication wrote no pending cookie")
			}
			if pending.Secure != c.want {
				t.Errorf("the pending cookie %s is written with Secure=%t, want %t", pending.Name, pending.Secure, c.want)
			}
		})
	}
}
