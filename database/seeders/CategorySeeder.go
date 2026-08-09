package seeders

import (
	"context"
	"errors"
	"fmt"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/security"

	models "github.com/arandu-io/examples/app/Models"
	policies "github.com/arandu-io/examples/app/Policies"
	repositories "github.com/arandu-io/examples/app/Repositories"
)

// CategorySeeder writes the sections this blog is organised into.
//
// It runs before PostSeeder, because a post is filed under a section and a
// section that does not exist yet cannot be named. That order is DatabaseSeeder's
// and not this file's -- a seeder that reached back to run another one would be
// a seeder whose order depends on who called it.
type CategorySeeder struct{}

// Name is how the seeder is addressed on the command line.
func (CategorySeeder) Name() string { return "CategorySeeder" }

// sections is what the blog is divided into.
//
// Four, and they are the four this example actually writes about. A seeder that
// invents twelve empty sections produces a navigation bar that is mostly dead
// ends, which is a worse first impression than no navigation at all.
var sections = []models.Category{
	{
		Name: "Architecture", Slug: "architecture",
		Description: "Why the compiler carries the rules, and what that costs.",
	},
	{
		Name: "Security", Slug: "security",
		Description: "Grants, tenants and the paths that do not exist.",
	},
	{
		Name: "Views", Slug: "views",
		Description: "Templates that compile, and a stack with no Node in it.",
	},
	{
		Name: "Operations", Slug: "operations",
		Description: "Migrations, queues and what a deploy is allowed to do.",
	},
}

// Run writes the sections, skipping any slug that is already there.
func (CategorySeeder) Run(ctx context.Context, d Deps) error {
	if d.DB == nil {
		return errors.New("the database is not wired")
	}

	repo := repositories.NewCategoryRepository(d.DB)

	// Two grants, because a Grant carries ONE action and Check compares it: one
	// issued for category.create is refused by List, and finding that out at run
	// time is the point of it being a type rather than a boolean.
	//
	//arandu:system-grant seeding has no request behind it, so there is no subject to ask a policy about
	listing := security.SystemGrant(policies.CategoryList, d.Tenant)
	//arandu:system-grant same reason: this seeder writes the sections the example ships with
	writing := security.SystemGrant(policies.CategoryCreate, d.Tenant)

	// Read once, not once per section: the set does not change while this loop
	// runs, and a query inside a loop is the shape of an N+1.
	existing, err := repo.All(ctx, listing)
	if err != nil {
		return fmt.Errorf("reading the sections: %w", err)
	}

	written := 0
	for _, ca := range sections {
		if categorySlugTaken(existing, ca.Slug) {
			continue
		}
		id, err := data.NewID()
		if err != nil {
			return err
		}
		ca.ID = id
		if _, err := repo.Create(ctx, writing, ca); err != nil {
			return fmt.Errorf("writing %s: %w", ca.Slug, err)
		}
		written++
	}

	fmt.Printf("%d section(s) written, %d already there\n", written, len(sections)-written)
	return nil
}

func categorySlugTaken(existing []models.Category, slug string) bool {
	for _, ca := range existing {
		if ca.Slug == slug {
			return true
		}
	}
	return false
}

var _ Seeder = CategorySeeder{}
