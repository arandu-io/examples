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

// CommentSeeder writes the thread under the first article.
//
// A comment section with nothing in it does not show what a comment section
// looks like -- it shows what an empty one looks like, which every blog has
// before anybody reads it. So this seeds a short exchange, and it seeds one
// comment that is still awaiting review, because the moderation queue is the
// screen that is easiest to ship broken and hardest to notice: it is empty when
// it works and empty when it does not.
type CommentSeeder struct{}

// Name is how the seeder is addressed on the command line.
func (CommentSeeder) Name() string { return "CommentSeeder" }

// thread is what the demo reader said, under which article.
//
// The slug rather than an id, for the same reason PostSeeder files by slug: the
// ids are generated when the other seeder runs.
//
// There is no date here, and that is not an omission: CommentRepository.Create
// stamps created_at itself, which is correct -- a repository that let a caller
// choose when a row was written is a repository that can be told a comment
// arrived last year. The thread is seeded in order, so it reads in order.
var thread = []struct {
	PostSlug string
	Body     string
	Approved bool
}{
	{
		PostSlug: "the-compiler-is-the-architecture",
		Body:     "The part that convinced me was the signature. I have written the \"remember to call authorize()\" comment at the top of a controller more than once, and it has never once stopped anybody -- including me.",
		Approved: true,
	},
	{
		PostSlug: "the-compiler-is-the-architecture",
		Body:     "Follow-up after a week of using it: the rigidity is real and it shows up on day one, when you want to write a quick query and cannot. By day three it stops registering. The queries you wanted to write quickly were the ones worth writing twice.",
		Approved: true,
	},
	{
		PostSlug: "a-tenant-is-never-what-the-caller-sent",
		Body:     "We had exactly this, in production, for four months. An id that came in on a header because one internal service needed to act on behalf of another. Nobody could see it in review because the parameter was called accountID and it was, technically, an account id.",
		Approved: true,
	},
	{
		PostSlug: "migrations-do-not-run-at-boot",
		Body:     "Does this hold with a blue/green deploy, where the two versions never actually overlap? Asking because our pipeline claims they do not, and I have stopped believing it.",
		// Not approved: this is what the moderation queue is for, and an empty
		// queue is a screen nobody can tell works.
		Approved: false,
	},
}

// Run writes the thread, skipping anything already there.
func (CommentSeeder) Run(ctx context.Context, d Deps) error {
	if d.DB == nil {
		return errors.New("the database is not wired")
	}
	if d.Users == nil {
		return errors.New("the user service is not wired")
	}

	posts := repositories.NewPostRepository(d.DB)
	comments := repositories.NewCommentRepository(d.DB)

	//arandu:system-grant seeding has no request behind it, so there is no subject to ask a policy about
	postList := security.SystemGrant(policies.PostList, d.Tenant)
	//arandu:system-grant same reason: reading what is already there keeps this seeder repeatable
	commentList := security.SystemGrant(policies.CommentList, d.Tenant)
	//arandu:system-grant and writing the thread the example ships with
	writing := security.SystemGrant(policies.CommentCreate, d.Tenant)

	// The author is the seeded reader, by id. A comment signed with a name typed
	// into a seeder would be a comment attached to nobody, and the thread on the
	// article resolves its author through this column.
	author, err := readerID(ctx, d)
	if err != nil {
		return err
	}

	// Read once, not once per comment.
	articles, err := posts.List(ctx, postList, data.Query{Limit: 200})
	if err != nil {
		return fmt.Errorf("reading the posts: %w", err)
	}
	postID := map[string]string{}
	for _, p := range articles {
		postID[p.Slug] = p.ID
	}

	existing, err := comments.List(ctx, commentList, data.Query{Limit: 200})
	if err != nil {
		return fmt.Errorf("reading the comments: %w", err)
	}

	written := 0
	for _, c := range thread {
		target, ok := postID[c.PostSlug]
		if !ok {
			// The article is not there. Running this seeder alone is a thing
			// people do, and skipping is better than failing: what it cannot
			// write, it says nothing about, and the next full run writes it.
			continue
		}
		if bodyTaken(existing, c.Body) {
			continue
		}

		id, err := data.NewID()
		if err != nil {
			return err
		}
		if _, err := comments.Create(ctx, writing, models.Comment{
			ID:       id,
			PostId:   target,
			Author:   author,
			Body:     c.Body,
			Approved: c.Approved,
		}); err != nil {
			return fmt.Errorf("writing a comment on %s: %w", c.PostSlug, err)
		}
		written++
	}

	fmt.Printf("%d comment(s) written, %d already there\n", written, len(thread)-written)
	return nil
}

// readerID is the demo reader's id, which is what the author column holds.
//
// It goes through the application user service rather than querying the users
// table here, because a seeder with its own SELECT would be a second owner of
// the schema and its tenant rules.
func readerID(ctx context.Context, d Deps) (string, error) {
	u, err := d.Users.Lookup(ctx, d.Tenant, readerEmail)
	if err != nil {
		return "", fmt.Errorf("the demo reader is missing -- run ReaderSeeder first (aru db:seed --class=ReaderSeeder): %w", err)
	}
	return u.ID, nil
}

func bodyTaken(existing []models.Comment, body string) bool {
	for _, c := range existing {
		if c.Body == body {
			return true
		}
	}
	return false
}

var _ Seeder = CommentSeeder{}
