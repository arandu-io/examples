package feature_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/arandu-io/hesape/session"

	"github.com/arandu-io/examples/bootstrap"
)

// TestTheSessionCookieLastsWhatSessionLifetimeSays signs in through the form
// on the wired application and reads the lifetime off the cookie the browser
// receives.
//
// SESSION_LIFETIME is minutes, read by the framework's loader, and the session
// store in bootstrap/app.go is built from that one value. The assertion is on
// the cookie rather than on the configuration, because a store built from
// anything else -- a second variable, a constant, the seconds this used to be
// written in -- loads the same configuration and hands the browser a different
// Max-Age.
func TestTheSessionCookieLastsWhatSessionLifetimeSays(t *testing.T) {
	for _, c := range []struct {
		name     string
		lifetime string
		maxAge   int
	}{
		{name: "thirty minutes", lifetime: "30", maxAge: 1800},
		{name: "unset, the framework's two hours", lifetime: "", maxAge: 7200},
	} {
		t.Run(c.name, func(t *testing.T) {
			sqliteEnv(t)
			t.Setenv("SESSION_LIFETIME", c.lifetime)
			if err := bootstrap.Dispatch("migrate", nil); err != nil {
				t.Fatalf("migrate: %v", err)
			}
			app := bootedInstance(t)

			const (
				email    = "lifetime@example.test"
				password = "a-long-enough-password"
			)
			if _, err := app.Users.Register(context.Background(), bootstrap.Tenant(), "Ana", email, password); err != nil {
				t.Fatalf("registering: %v", err)
			}

			var cookie *http.Cookie
			for _, written := range signInOn(t, app.Kernel.Handler(), email, password) {
				if written.Name == session.CookieName && written.MaxAge > 0 {
					cookie = written
				}
			}
			if cookie == nil {
				t.Fatalf("the sign-in wrote no %s cookie", session.CookieName)
			}
			if cookie.MaxAge != c.maxAge {
				t.Errorf("SESSION_LIFETIME=%q: the session cookie has Max-Age=%d, want %d",
					c.lifetime, cookie.MaxAge, c.maxAge)
			}
		})
	}
}
