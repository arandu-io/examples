package seeders

import (
	"context"
	"errors"
	"fmt"

	"github.com/arandu-io/framework/security"

	models "github.com/arandu-io/examples/app/Models"
	policies "github.com/arandu-io/examples/app/Policies"
	factories "github.com/arandu-io/examples/database/factories"
)

// CategorySeeder writes the sections this blog is organised into.
//
// It runs before PostSeeder, because a post is filed under a section and a
// section that does not exist yet cannot be named. That order is DatabaseSeeder's
// and not this file's -- a seeder that reached back to run another one would be
// a seeder whose order depends on who called it.
//
// It writes through the factory, which writes through the model. That is one
// path and not two: the factory's Create is the same insert CategoryService
// makes, under a Grant, with the tenant taken off it. What the seeder adds is
// the words -- see section below.
type CategorySeeder struct{}

// Name is how the seeder is addressed on the command line.
func (CategorySeeder) Name() string { return "CategorySeeder" }

// section is one of the sections this example ships with: the words, as a state
// the factory applies over its own definition.
type section struct {
	name        string
	slug        string
	description string
}

// sections is what the blog is divided into.
//
// Four, and they are the four this example actually writes about. A seeder that
// invents twelve empty sections produces a navigation bar that is mostly dead
// ends, which is a worse first impression than no navigation at all.
var sections = []section{
	{"Architecture", "architecture", "Why the compiler carries the rules, and what that costs."},
	{"Security", "security", "Grants, tenants and the paths that do not exist."},
	{"Views", "views", "Templates that compile, and a stack with no Node in it."},
	{"Operations", "operations", "Migrations, queues and what a deploy is allowed to do."},
}

// Run writes the sections, skipping any slug that is already there.
func (CategorySeeder) Run(ctx context.Context, d Deps) error {
	if d.DB == nil {
		return errors.New("the database is not wired")
	}

	// Two grants, because a Grant carries ONE action and the Policy that issued
	// it answered about one: one minted for category.create does not become
	// permission to page through the table.
	//
	//arandu:system-grant seeding has no request behind it, so there is no subject to ask a policy about
	listing := security.SystemGrant(policies.CategoryList, d.Tenant)
	//arandu:system-grant same reason: this seeder writes the sections the example ships with
	writing := security.SystemGrant(policies.CategoryCreate, d.Tenant)

	// Read once, not once per section: the set does not change while this runs,
	// and a query inside a loop is the shape of an N+1. Pluck is the terminal
	// that answers one column, so the round trip carries the slugs and not the
	// rows behind them -- and it takes the Grant, like every other read.
	taken, err := models.Categories(d.DB).NewQuery().Pluck(ctx, listing, "slug")
	if err != nil {
		return fmt.Errorf("reading the sections: %w", err)
	}
	existing := make(map[string]bool, len(taken))
	for _, slug := range taken {
		if s, ok := slug.(string); ok {
			existing[s] = true
		}
	}

	// One state per missing section, in order. Sequence walks them as it builds
	// the rows, so the factory makes exactly as many rows as there are words to
	// put in them -- rather than four rows of which some are thrown away.
	var wanted []func(*models.Category)
	for _, s := range sections {
		if existing[s.slug] {
			continue
		}
		wanted = append(wanted, factories.Section(s.name, s.slug, s.description))
	}
	if len(wanted) == 0 {
		fmt.Printf("0 section(s) written, %d already there\n", len(sections))
		return nil
	}

	// The Grant is on Create and on nothing before it: Count, Sequence and the
	// definition build a sentence, and a sentence authorizes nothing. The rows
	// come back filed under data.Tenant(writing), whatever the definition put in
	// the tenant field.
	written, err := factories.Categories(d.DB).
		Count(len(wanted)).
		Sequence(wanted...).
		Create(ctx, writing)
	if err != nil {
		return fmt.Errorf("writing the sections: %w", err)
	}

	fmt.Printf("%d section(s) written, %d already there\n", len(written), len(sections)-len(written))
	return nil
}

var _ Seeder = CategorySeeder{}
