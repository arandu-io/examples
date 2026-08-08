package seeders

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/security"

	models "github.com/arandu-io/examples/app/Models"
	policies "github.com/arandu-io/examples/app/Policies"
	repositories "github.com/arandu-io/examples/app/Repositories"
)

// PostSeeder writes the posts this example ships with.
//
// It is the second place in the application that holds a system grant, and the
// first is AdminSeeder. Both are here for the same reason: a seeder has no
// request behind it and therefore no subject, so there is no policy to ask. The
// grant is named, it is scoped to one action, and `aru doctor` accepts it here
// because the file is under database/seeders -- the same call in a controller is
// reported.
type PostSeeder struct{}

// Name is how the seeder is addressed on the command line.
func (PostSeeder) Name() string { return "PostSeeder" }

// Run writes the posts, skipping any slug that already exists.
//
// Idempotent on purpose: `aru db:seed` is run more than once on a machine
// somebody is working on, and a seeder that fails the second time is a seeder
// people stop running.
func (PostSeeder) Run(ctx context.Context, d Deps) error {
	if d.DB == nil {
		return errors.New("the database is not wired")
	}

	repo := repositories.NewPostRepository(d.DB)
	now := time.Now().UTC()

	// Two grants, because a Grant carries ONE action and Check compares it: a
	// grant issued for post.create is refused by List, and finding that out at
	// run time is the point of it being a type rather than a boolean.
	//
	//arandu:system-grant seeding has no request behind it, so there is no subject to ask a policy about
	listing := security.SystemGrant(policies.PostList, d.Tenant)
	//arandu:system-grant same reason: this seeder writes the posts the example ships with
	writing := security.SystemGrant(policies.PostCreate, d.Tenant)

	// Read once, not once per post: the slug set does not change while this
	// loop runs, and a query inside a loop is the shape of an N+1.
	existing, err := repo.List(ctx, listing, data.Query{Limit: 200})
	if err != nil {
		return fmt.Errorf("reading the posts: %w", err)
	}

	posts := []models.Post{
		{
			Title:       "The compiler is the architecture",
			Slug:        "the-compiler-is-the-architecture",
			Body:        "A repository call takes a Grant. There is no path from a handler to the database that does not carry one, and that is not a convention the team agreed to follow -- it is a signature. Forgetting the check is not a mistake you can make here; it is code that does not build.",
			PublishedAt: now.Add(-72 * time.Hour),
		},
		{
			Title:       "A tenant is never what the caller sent",
			Slug:        "a-tenant-is-never-what-the-caller-sent",
			Body:        "Every query is scoped by a tenant read off the Grant. A header, a query string or a path segment cannot decide whose rows come back, and `aru doctor` follows the value rather than reading the name: an identifier called org that arrived from the request and reached a Grant is reported, however innocent the name.",
			PublishedAt: now.Add(-24 * time.Hour),
		},
		{
			// No PublishedAt: this one is a draft, and the policy of this
			// application says a draft is not readable without a session. It is
			// here so the example has a row that exercises that branch.
			Title: "Notes on the view layer",
			Slug:  "notes-on-the-view-layer",
			Body:  "A view compiles to Go. The data is a struct the view declares, so a field that does not exist stops the build at the line you wrote rather than rendering an empty space in production.",
		},
	}

	written := 0
	for _, p := range posts {
		if slugTaken(existing, p.Slug) {
			continue
		}
		id, err := data.NewID()
		if err != nil {
			return err
		}
		p.ID = id
		p.CreatedAt = now
		if _, err := repo.Create(ctx, writing, p); err != nil {
			return fmt.Errorf("writing %s: %w", p.Slug, err)
		}
		written++
	}

	fmt.Printf("%d post(s) written, %d already there\n", written, len(posts)-written)
	return nil
}

func slugTaken(existing []models.Post, slug string) bool {
	for _, p := range existing {
		if p.Slug == slug {
			return true
		}
	}
	return false
}

var _ Seeder = PostSeeder{}
