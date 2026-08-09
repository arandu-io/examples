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

	seed(t, "UserSeeder", "-e", "op@example.com", "-p", "the-first-password", "-r", "admin")

	client.Get("/auth/login").OK()
	client.Post("/auth/login", map[string]string{
		"email": "op@example.com", "password": "the-first-password",
	}).RedirectsTo("/")

	// Without -upd the password is untouched, whatever -p says.
	seed(t, "UserSeeder", "-e", "op@example.com", "-p", "ignored-entirely")
	client.Get("/auth/login").OK()
	client.Post("/auth/login", map[string]string{
		"email": "op@example.com", "password": "the-first-password",
	}).RedirectsTo("/")

	// With it, the new one works and the old one does not.
	seed(t, "UserSeeder", "-upd", "-e", "op@example.com", "-p", "the-second-password")

	client.Get("/auth/login").OK()
	client.Post("/auth/login", map[string]string{
		"email": "op@example.com", "password": "the-second-password",
	}).RedirectsTo("/")

	client.Get("/auth/login").OK()
	client.Post("/auth/login", map[string]string{
		"email": "op@example.com", "password": "the-first-password",
	}).Status(401)
}
