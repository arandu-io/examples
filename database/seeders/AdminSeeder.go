package seeders

import (
	"context"
	"errors"
	"fmt"
	"os"
)

// AdminSeeder creates the first administrator of a tenant.
//
// It exists because there is no other way in: every repository call needs a
// Grant, and a Grant needs a subject, so the first user cannot be created
// through the application itself. This is the one place that breaks the circle
// with a system grant, and it does it in the open rather than at boot -- seeding
// that happens by itself is how a known password reaches production.
//
// It is the demo administrator, and it takes nothing: `aru db:seed` on a fresh
// clone has to produce somebody to sign in as, without arguments, or the first
// run ends at the sign-in screen.
//
// A specific address and password is UserSeeder's job:
//
//	aru db:seed UserSeeder -e you@example.com -p a-long-password -r admin
//
// The pair below is a fallback and it is guarded by APP_ENV and by nothing else,
// which is the only guard that matters: a default password is a hole exactly
// once, the first time this runs somewhere it was not supposed to. ARANDU_ADMIN_
// still overrides it, for a pipeline that seeds without a terminal.
type AdminSeeder struct{}

// Name is how the seeder is addressed on the command line.
func (AdminSeeder) Name() string { return "AdminSeeder" }

// The demo administrator. The address is on example.com, which RFC 2606
// reserves: it cannot be registered, so a stray message to it reaches nobody.
const (
	adminName       = "Paulo Lima"
	demoAdminEmail  = "admin@example.com"
	adminEmailVar   = "ARANDU_ADMIN_EMAIL"
	adminPasswdVar  = "ARANDU_ADMIN_PASSWORD"
	developmentOnly = "dev"
)

// Run creates the administrator, or leaves the existing one alone.
func (AdminSeeder) Run(ctx context.Context, d Deps) error {
	if d.Auth == nil {
		return errors.New("the auth service is not wired")
	}
	tenant := d.Tenant
	if tenant == "" {
		return errors.New("the tenant is not wired: seeding into an empty tenant would create a user nobody can log in as")
	}

	email := os.Getenv(adminEmailVar)
	if email == "" {
		if os.Getenv("APP_ENV") != developmentOnly {
			return errors.New("set " + adminEmailVar + " and " + adminPasswdVar + " before seeding the administrator")
		}
		email = demoAdminEmail
	}

	password, err := demoPassword(adminPasswdVar)
	if err != nil {
		return err
	}

	// An administrator is created verified. Nobody is going to click a link in
	// a mailbox that does not exist, and an administrator who cannot moderate
	// because of it is a first run that ends at the sign-in screen.
	user, err := d.Auth.EnsureUser(ctx, tenant, adminName, email, password, []string{"admin"}, true)
	if err != nil {
		return err
	}

	fmt.Printf("administrator %s ready in tenant %s\n", user.Email, user.TenantID)
	return nil
}

// compile-time proof that every seeder honors the contract. A seeder that drifts
// from the interface fails the build rather than failing when someone runs it.
var (
	_ Seeder = DatabaseSeeder{}
	_ Seeder = AdminSeeder{}
)
