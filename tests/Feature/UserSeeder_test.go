package feature_test

import (
	"testing"

	"github.com/arandu-io/examples/bootstrap"
	"github.com/arandu-io/examples/tests"
)

// seed runs a seeder the way the command line does, so what is proved is the
// argument parsing as well as the seeder.
func seed(t *testing.T, args ...string) {
	t.Helper()
	if err := bootstrap.Dispatch("db:seed", args); err != nil {
		t.Fatalf("aru db:seed %v: %v", args, err)
	}
}

// TestUserSeederCreatesAndReplaces proves the operator command end to end: the
// account it makes can sign in, the second run without -upd leaves the password
// alone, and -upd replaces it.
func TestUserSeederCreatesAndReplaces(t *testing.T) {
	client, _ := tests.App(t)

	// The sign-in screen is for people who are not signed in: the guest guard
	// sends anybody else to the front page. So each attempt below starts where a
	// person starts, which means leaving the previous session first -- walking
	// back to the form still signed in is not something a browser does.
	//
	// A rendered page first, because the sign-out form is what carries the CSRF
	// token and the body of a redirect carries none. The answer is asserted: a
	// sign-out posted without a token is answered 419 and the session lives on,
	// which is a helper called signOut that does not sign anybody out.
	signOut := func() {
		t.Helper()
		client.Get("/").OK()
		client.Post("/auth/logout", nil).Status(303).RedirectsTo("/auth/login")
	}

	seed(t, "UserSeeder", "-e", "op@example.com", "-p", "the-first-password", "-r", "admin")

	client.Get("/auth/login").OK()
	client.Post("/auth/login", map[string]string{
		"email": "op@example.com", "password": "the-first-password",
	}).RedirectsTo("/")
	signOut()

	// Without -upd the password is untouched, whatever -p says.
	seed(t, "UserSeeder", "-e", "op@example.com", "-p", "ignored-entirely")
	client.Get("/auth/login").OK()
	client.Post("/auth/login", map[string]string{
		"email": "op@example.com", "password": "the-first-password",
	}).RedirectsTo("/")
	signOut()

	// With it, the new one works and the old one does not.
	seed(t, "UserSeeder", "-upd", "-e", "op@example.com", "-p", "the-second-password")

	client.Get("/auth/login").OK()
	client.Post("/auth/login", map[string]string{
		"email": "op@example.com", "password": "the-second-password",
	}).RedirectsTo("/")
	signOut()

	client.Get("/auth/login").OK()
	client.Post("/auth/login", map[string]string{
		"email": "op@example.com", "password": "the-first-password",
	}).Status(401)
}
