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
// It is one of the places in the application that holds a system grant, and
// AdminSeeder is another. Both are here for the same reason: a seeder has no
// request behind it and therefore no subject, so there is no policy to ask. The
// grant is named, it is scoped to one action, and aru doctor accepts it here
// because the file is under database/seeders -- the same call in a controller is
// reported.
type PostSeeder struct{}

// Name is how the seeder is addressed on the command line.
func (PostSeeder) Name() string { return "PostSeeder" }

// seededPost is one article and the section it belongs to.
//
// The section is a slug and not an id, because the id is generated when
// CategorySeeder runs and this list is written by hand.
type seededPost struct {
	models.Post
	Section string
	// Age is how long ago it went out. A relative offset rather than a date, so
	// the front page of a clone made next year is not a wall of 2026.
	Age time.Duration
	// Views is where the counter starts, so the example has something to show
	// on a card rather than a column of blanks.
	Views int
}

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
	categories := repositories.NewCategoryRepository(d.DB)
	now := time.Now().UTC()

	// Three grants, because a Grant carries ONE action and Check compares it: a
	// grant issued for post.create is refused by List, and finding that out at
	// run time is the point of it being a type rather than a boolean.
	//
	//arandu:system-grant seeding has no request behind it, so there is no subject to ask a policy about
	listing := security.SystemGrant(policies.PostList, d.Tenant)
	//arandu:system-grant same reason: this seeder writes the posts the example ships with
	writing := security.SystemGrant(policies.PostCreate, d.Tenant)
	//arandu:system-grant reading the sections, to file each post under one
	sectionList := security.SystemGrant(policies.CategoryList, d.Tenant)

	// Read once, not once per post: neither set changes while this loop runs,
	// and a query inside a loop is the shape of an N+1.
	existing, err := repo.List(ctx, listing, data.Query{Limit: 200})
	if err != nil {
		return fmt.Errorf("reading the posts: %w", err)
	}
	filed, err := categories.All(ctx, sectionList)
	if err != nil {
		return fmt.Errorf("reading the sections: %w", err)
	}
	sectionID := map[string]string{}
	for _, ca := range filed {
		sectionID[ca.Slug] = ca.ID
	}

	written := 0
	for _, p := range seededPosts() {
		if slugTaken(existing, p.Slug) {
			continue
		}
		id, err := data.NewID()
		if err != nil {
			return err
		}

		post := p.Post
		post.ID = id
		post.CreatedAt = now.Add(-p.Age)
		post.Views = p.Views
		// A section that does not exist leaves the post unfiled rather than
		// failing the seeder. Running PostSeeder alone, with --class, is a thing
		// people do, and it should write posts rather than complain about a
		// navigation bar.
		post.CategoryID = sectionID[p.Section]
		if !post.PublishedAt.IsZero() {
			post.PublishedAt = now.Add(-p.Age)
		}

		if _, err := repo.Create(ctx, writing, post); err != nil {
			return fmt.Errorf("writing %s: %w", post.Slug, err)
		}
		written++
	}

	total := len(seededPosts())
	fmt.Printf("%d post(s) written, %d already there\n", written, total-written)
	return nil
}

// seededPosts is the content.
//
// A function and not a package variable, because PublishedAt is set from a
// non-zero sentinel below and Run overwrites it with a real time -- a shared
// slice would carry the previous run's dates into the next one.
func seededPosts() []seededPost {
	// Any non-zero time marks a post as published; Run replaces it with one
	// relative to now. The zero value is a draft, and that is the whole
	// distinction the policy reads.
	published := time.Unix(1, 0)

	return []seededPost{
		{
			Section: "architecture", Age: 21 * 24 * time.Hour, Views: 1284,
			Post: models.Post{
				Title:       "The compiler is the architecture",
				Slug:        "the-compiler-is-the-architecture",
				PublishedAt: published,
				Body: "A repository call takes a Grant. There is no path from a handler to the database that does not carry one, and that is not a convention the team agreed to follow -- it is a signature.\n\n" +
					"Forgetting the check is not a mistake you can make here. It is code that does not build. That single sentence is the whole thesis of this framework, and everything else in it exists to keep the sentence true as an application grows past the point where anybody can hold it in their head.\n\n" +
					"The usual objection is that this is rigid. It is. The question worth asking is what the flexible version buys, and the honest answer is that it buys the ability to skip the check -- which nobody wants, and which every codebase does, quietly, in the file nobody reviewed.",
			},
		},
		{
			Section: "security", Age: 14 * 24 * time.Hour, Views: 906,
			Post: models.Post{
				Title:       "A tenant is never what the caller sent",
				Slug:        "a-tenant-is-never-what-the-caller-sent",
				PublishedAt: published,
				Body: "Every query is scoped by a tenant read off the Grant. A header, a query string or a path segment cannot decide whose rows come back.\n\n" +
					"aru doctor follows the value rather than reading the name: an identifier called org that arrived from the request and reached a Grant is reported, however innocent the name looks. Names are how this bug hides -- nobody writes tenantFromAttacker, they write orgID, and it reads fine in review.\n\n" +
					"The rule is one line long and it is worth memorising: data.Tenant(g), always, and nothing else.",
			},
		},
		{
			Section: "views", Age: 9 * 24 * time.Hour, Views: 742,
			Post: models.Post{
				Title:       "A template that does not compile does not ship",
				Slug:        "a-template-that-does-not-compile-does-not-ship",
				PublishedAt: published,
				Body: "A view is written in kyse and compiled to Go. The data it draws from is a struct the view declares, so a field that does not exist stops the build at the line you wrote.\n\n" +
					"The comparison is not with a faster template engine. It is with the class of bug where a rename lands everywhere except one partial, and the partial renders an empty space that nobody notices until a customer asks why the invoice has no total.\n\n" +
					"There is no Node anywhere in this. No package.json, no lockfile, no node_modules. git clone && aru dev and the stylesheet is already built.",
			},
		},
		{
			Section: "operations", Age: 5 * 24 * time.Hour, Views: 511,
			Post: models.Post{
				Title:       "Migrations do not run at boot",
				Slug:        "migrations-do-not-run-at-boot",
				PublishedAt: published,
				Body: "aru migrate is a step in a pipeline and never a call in the start of the process. With N replicas coming up together, N migrations race, and the one that loses is not always the one that fails.\n\n" +
					"The second half of the rule is the one people skip: every migration is compatible with the PREVIOUS version of the binary, because during a rollout both are running. A new column is nullable or has a default. Dropping one takes two releases.\n\n" +
					"None of this is new. What is new is that it is written down in the framework rather than in the runbook nobody reads.",
			},
		},
		{
			Section: "security", Age: 2 * 24 * time.Hour, Views: 288,
			Post: models.Post{
				Title:       "Reading needs a policy too",
				Slug:        "reading-needs-a-policy-too",
				PublishedAt: published,
				Body: "List, Find, the read model, the projection, the report, the dashboard and the export all take a Grant and all filter by tenant.\n\n" +
					"\"Optional policy on reads\" is a data leak between customers with a technical name. It is also the most common shape of the real thing: the write path is reviewed carefully because it changes something, and the report that joins four tables for a CSV is written on a Friday.",
			},
		},
		{
			// No PublishedAt: this one is a draft, and the policy of this
			// application says a draft is not readable without a session. It is
			// here so the example has a row that exercises that branch -- open
			// the front page signed out and it is not there.
			Section: "views", Age: 12 * time.Hour,
			Post: models.Post{
				Title: "Notes on components, and why they are functions",
				Slug:  "notes-on-components-and-why-they-are-functions",
				Body:  "A component is a Go function that returns HTML and takes a struct. There is no custom tag, no web component and no runtime registry -- which means the arguments are checked, the editor completes them, and a component nobody calls is a compile error rather than dead markup.",
			},
		},
	}
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
