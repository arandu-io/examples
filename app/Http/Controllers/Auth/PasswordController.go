package authui

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/arandu-io/framework/observability"
	"github.com/arandu-io/framework/view"
)

// The password reset, wired.
//
// The kit publishes the three screens and stops there, on purpose (ADR 0022):
// the handlers write to your users table, send through your mailer and decide
// your rules, so they are the application's. This is that application's.
//
// # What it does and does not promise
//
// The token is random, single-use and expires. It is stored hashed, so the
// store holding it is not a store of live reset links, and it is compared with
// hmac.Equal rather than == -- the comparison is against a secret and the timing
// of a byte-by-byte one leaks its prefix.
//
// It is held in memory. That is right for one instance and wrong for two, in
// exactly the way the session store is: behind a load balancer the reset link
// works only on the replica that issued it. Moving it is one type -- the same
// swap docs/11 describes for sessions -- and saying so here is better than a
// store that looks distributed and is not.
//
// # Why the answer is always the same
//
// Asking for a link answers with the same message whether the address is
// registered or not. The alternative is an oracle: a form that says "no such
// account" is a form that confirms which addresses exist, one request at a time.

// resetTTL is how long a link works. Long enough to reach an inbox and read it,
// short enough that a link forwarded by accident has usually expired.
const resetTTL = time.Hour

// resets is the store. See the note above about one instance.
var resets = struct {
	sync.Mutex
	byHash map[string]reset
}{byHash: map[string]reset{}}

type reset struct {
	email   string
	expires time.Time
}

// showPasswordRequest draws the "send me a link" form.
func (m *Module) showPasswordRequest(w http.ResponseWriter, r *http.Request) {
	m.screen(w, r, "auth.passwords.email", AuthPage{})
}

// sendPasswordLink issues a token and, in this example, prints the link.
//
// A real application sends it through its mailer. Printing it is what keeps this
// example runnable with nothing installed -- and it is marked in the log as what
// it is, so nobody ships it by accident.
func (m *Module) sendPasswordLink(w http.ResponseWriter, r *http.Request) {
	email := strings.TrimSpace(r.PostFormValue("email"))

	if email != "" {
		raw := make([]byte, 32)
		if _, err := rand.Read(raw); err != nil {
			observability.Log(r.Context()).Error("generating a reset token", "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		token := base64.RawURLEncoding.EncodeToString(raw)

		sum := sha256.Sum256([]byte(token))
		resets.Lock()
		resets.byHash[string(sum[:])] = reset{email: email, expires: time.Now().Add(resetTTL)}
		resets.Unlock()

		observability.Log(r.Context()).Info("password reset link issued -- this example prints it instead of sending it",
			"email", email, "link", "/auth/password/reset?token="+token)
	}

	// The same answer either way. A form that says "no such account" is an
	// oracle for which addresses are registered.
	m.screen(w, r, "auth.passwords.email", AuthPage{
		Status: "If that address is registered, a link is on its way.",
	})
}

// showPasswordReset draws the new-password form, carrying the token.
func (m *Module) showPasswordReset(w http.ResponseWriter, r *http.Request) {
	m.screen(w, r, "auth.passwords.reset", AuthPage{
		ResetToken: r.URL.Query().Get("token"),
	})
}

// updatePassword consumes the token and changes the password.
func (m *Module) updatePassword(w http.ResponseWriter, r *http.Request) {
	token := r.PostFormValue("token")
	password := r.PostFormValue("password")
	confirmation := r.PostFormValue("password_confirmation")

	if password != confirmation {
		m.screen(w, r, "auth.passwords.reset", AuthPage{
			ResetToken:                token,
			PasswordConfirmationError: "the two passwords do not match",
		})
		return
	}

	email, err := consume(token)
	if err != nil {
		m.screen(w, r, "auth.passwords.reset", AuthPage{
			EmailError: "that link is not valid any more. Ask for another.",
		})
		return
	}

	// What is left to do, and it is the one thing this example cannot do for
	// you: write the new password. The framework's auth service owns the users
	// table and exposes no SetPassword, deliberately -- changing a credential is
	// a decision about your rules (a minimum length, a history, a notification),
	// and a framework that made it would be making it for everybody.
	//
	// The token has been consumed by this point, so the link cannot be replayed
	// while you wire the write.
	observability.Log(r.Context()).Warn("password reset reached the end of the flow, and this example does not write the password",
		"email", email, "next", "call your own service here")

	redirect(w, r, "/auth/login")
}

// consume checks a token and removes it, so a link works once.
//
// The stored key is the hash, so what is held is not a live reset link. The
// comparison is hmac.Equal because it is against a secret, and a byte-by-byte
// one leaks the prefix through its timing.
func consume(token string) (string, error) {
	if token == "" {
		return "", errors.New("no token")
	}
	sum := sha256.Sum256([]byte(token))

	resets.Lock()
	defer resets.Unlock()

	for key, entry := range resets.byHash {
		if !hmac.Equal([]byte(key), sum[:]) {
			continue
		}
		delete(resets.byHash, key)
		if time.Now().After(entry.expires) {
			return "", errors.New("expired")
		}
		return entry.email, nil
	}
	return "", errors.New("unknown token")
}

// screen renders one of the kit's pages with a fresh token.
//
// Every screen here needs one: they all post, and a page rendered without a
// token answers 200 and then refuses the submission with 419 -- which reads like
// a broken session rather than a missing field.
func (m *Module) screen(w http.ResponseWriter, r *http.Request, name string, data AuthPage) {
	token, err := m.csrf.Issue(m.sessions.IDFromRequest(r))
	if err != nil {
		observability.Log(r.Context()).Error("issuing csrf token", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	title := data.Page.Title
	data.Page = view.Page{
		Title:     title,
		AppName:   "arandu-io/examples",
		Token:     token,
		HomeURL:   "/",
		LoginURL:  "/auth/login",
		LogoutURL: "/auth/logout",
	}
	if data.Page.Title == "" {
		data.Page.Title = "Password"
	}
	data.HasPasswordReset = true
	data.PasswordEmailURL = "/auth/password/email"
	data.PasswordRequestURL = "/auth/password"
	data.PasswordUpdateURL = "/auth/password/update"

	if err := view.NewRenderer().Render(r.Context(), w, http.StatusOK, name, data); err != nil {
		observability.Log(r.Context()).Error("rendering "+name, "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}
