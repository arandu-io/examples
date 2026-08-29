package seeders

import (
	"context"
	"errors"
	"fmt"
	"os"
)

// ReaderSeeder creates the demo account that comments.
//
// The blog needs two people to be worth looking at: somebody who writes and
// somebody who answers. AdminSeeder makes the first; this makes the second, and
// it is deliberately an ordinary account with no roles -- so opening the example
// signed in as this one shows what a reader sees, which is the half that is easy
// to get wrong and impossible to check while signed in as an administrator.
//
// It is created already verified. CommentPolicy refuses an unverified account,
// and a seeded reader waiting for a code in a mailbox that does not exist is a
// demo that demonstrates nothing.
type ReaderSeeder struct{}

// Name is how the seeder is addressed on the command line.
func (ReaderSeeder) Name() string { return "ReaderSeeder" }

// The demo reader. The address is on example.com, which RFC 2606 reserves: it
// cannot be registered, so a stray message to it reaches nobody.
const (
	readerName  = "Ada Ferreira"
	readerEmail = "reader@example.com"
)

// Run creates the reader, or leaves the existing one alone.
func (ReaderSeeder) Run(ctx context.Context, d Deps) error {
	if d.Users == nil {
		return errors.New("the user service is not wired")
	}
	if d.Tenant == "" {
		return errors.New("the tenant is not wired: seeding into an empty tenant would create a user nobody can log in as")
	}

	password, err := demoPassword("ARANDU_READER_PASSWORD")
	if err != nil {
		return err
	}

	u, err := d.Users.EnsureUser(ctx, d.Tenant, readerName, readerEmail, password, nil, true)
	if err != nil {
		return err
	}

	fmt.Printf("reader %s ready, verified, no roles\n", u.Email)
	return nil
}

// demoPassword reads the variable, or falls back to a known one in development.
//
// The fallback is guarded by APP_ENV and by nothing else, which is the only
// guard that matters: a default password is a hole exactly once -- the first
// time this runs somewhere it was not supposed to. Outside development the
// variable is required and the seeder refuses without it.
//
// It prints what it did. A credential nobody was told about is a credential
// nobody can use and nobody can remove.
func demoPassword(variable string) (string, error) {
	if password := os.Getenv(variable); password != "" {
		return password, nil
	}
	if os.Getenv("APP_ENV") != "dev" {
		return "", fmt.Errorf("set %s: this seeder only falls back to a known password when APP_ENV=dev", variable)
	}

	const known = "arandu-demo-password"
	fmt.Printf("  using the development password %q -- set %s to change it\n", known, variable)
	return known, nil
}

var _ Seeder = ReaderSeeder{}
