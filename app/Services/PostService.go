package services

import (
	"context"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/observability"
	"github.com/arandu-io/framework/security"

	requests "github.com/arandu-io/examples/app/Http/Requests"
	models "github.com/arandu-io/examples/app/Models"
	policies "github.com/arandu-io/examples/app/Policies"
	repositories "github.com/arandu-io/examples/app/Repositories"
)

// PostService holds the business rules. It receives its dependencies through
// the constructor -- explicit wiring, no container.
type PostService struct {
	repo   *repositories.PostRepository
	policy policies.PostPolicy
}

// NewPostService wires the service.
func NewPostService(repo *repositories.PostRepository) *PostService {
	return &PostService{repo: repo}
}

// Create walks the mandatory path: validate, Authorize, Grant, Repository.
// There is no other order that compiles.
func (s *PostService) Create(ctx context.Context, actor security.Subject, in requests.StorePost) (models.Post, error) {
	if errs := in.Validate(); errs.Any() {
		return models.Post{}, errs
	}

	candidate := models.Post{
		Title:       in.Title,
		Slug:        in.Slug,
		Body:        in.Body,
		PublishedAt: in.PublishedAt,
	}

	g, err := security.Authorize(ctx, s.policy, actor, policies.PostCreate, candidate)
	if err != nil {
		return models.Post{}, err
	}

	created, err := s.repo.Create(ctx, g, candidate)
	if err != nil {
		return models.Post{}, err
	}
	// Guarded: the entity is a struct value, and boxing it into `any` allocates
	// at the call site even though RecordEvent is a no-op on a nil Collector.
	if col := observability.FromContext(ctx); col != nil {
		col.RecordEvent("post.created", created)
	}
	return created, nil
}

// Get returns one post.
func (s *PostService) Get(ctx context.Context, actor security.Subject, id string) (models.Post, error) {
	// Asked twice, and the second time is the one that matters.
	//
	// The first call is about the zero value, because the row has not been read
	// yet -- it answers "may this subject look at posts at all", which is what
	// produces the Grant the repository needs. The second is about the row that
	// came back, and it is where a rule that depends on the record is decided:
	// a guest may read a published post and not a draft.
	//
	// Deciding only once, on the zero value, is how a policy that reads
	// p.PublishedAt is written and never consulted -- the field it branches on
	// is always empty, so the rule silently means something else.
	g, err := security.Authorize(ctx, s.policy, actor, policies.PostView, models.Post{})
	if err != nil {
		return models.Post{}, err
	}

	found, err := s.repo.Find(ctx, g, id)
	if err != nil {
		return models.Post{}, err
	}
	if _, err := security.Authorize(ctx, s.policy, actor, policies.PostView, found); err != nil {
		return models.Post{}, err
	}
	return found, nil
}

// List returns a page of posts.
func (s *PostService) List(ctx context.Context, actor security.Subject, q data.Query) ([]models.Post, error) {
	g, err := security.Authorize(ctx, s.policy, actor, policies.PostList, models.Post{})
	if err != nil {
		return nil, err
	}
	return s.repo.List(ctx, g, q)
}

// Published is the public listing, for a reader with or without an account.
//
// It goes through Authorize like everything else. A guest reaches PostPolicy and
// is allowed; somebody signed in is allowed too, and gets the same rows -- the
// published listing is the published listing, and having it mean two things
// depending on the reader is how a page starts disagreeing with its own sitemap.
func (s *PostService) Published(ctx context.Context, actor security.Subject, limit int) ([]models.Post, error) {
	g, err := security.Authorize(ctx, s.policy, actor, policies.PostPublicList, models.Post{})
	if err != nil {
		return nil, err
	}
	return s.repo.Published(ctx, g, limit)
}

// PublishedInCategory is the public listing narrowed to one section.
//
// The same PostPublicList as Published, because it is the same question asked
// about fewer rows. A permission of its own would be one nobody could grant
// without granting the wider one anyway.
func (s *PostService) PublishedInCategory(ctx context.Context, actor security.Subject, categoryID string, limit int) ([]models.Post, error) {
	g, err := security.Authorize(ctx, s.policy, actor, policies.PostPublicList, models.Post{})
	if err != nil {
		return nil, err
	}
	return s.repo.PublishedInCategory(ctx, g, categoryID, limit)
}

// CountByCategory is how many published posts each section holds.
func (s *PostService) CountByCategory(ctx context.Context, actor security.Subject) (map[string]int, error) {
	g, err := security.Authorize(ctx, s.policy, actor, policies.PostPublicList, models.Post{})
	if err != nil {
		return nil, err
	}
	return s.repo.CountByCategory(ctx, g)
}

// Read records that an article was opened, and never fails a page over it.
//
// A counter is not worth a 500. The read that matters already happened by the
// time this is called, so a failure here is logged and the article is served --
// which is also why it is not part of Get: a caller that wanted the row and got
// an error about a counter would have no way to tell the two apart.
func (s *PostService) Read(ctx context.Context, actor security.Subject, p models.Post) {
	g, err := security.Authorize(ctx, s.policy, actor, policies.PostView, p)
	if err != nil {
		return
	}
	if err := s.repo.IncrementViews(ctx, g, p.ID); err != nil {
		observability.Log(ctx).Warn("the read counter was not incremented", "error", err, "post", p.ID)
	}
}

// Update changes the mutable fields.
//
// It reads before writing, so the policy decides against the stored row rather
// than against what the client claims the row is. Skipping this is how a check
// passes on attacker-supplied data.
func (s *PostService) Update(ctx context.Context, actor security.Subject, in requests.UpdatePost) (models.Post, error) {
	if errs := in.Validate(); errs.Any() {
		return models.Post{}, errs
	}

	view, err := security.Authorize(ctx, s.policy, actor, policies.PostView, models.Post{})
	if err != nil {
		return models.Post{}, err
	}
	stored, err := s.repo.Find(ctx, view, in.ID)
	if err != nil {
		return models.Post{}, err
	}

	g, err := security.Authorize(ctx, s.policy, actor, policies.PostUpdate, stored)
	if err != nil {
		return models.Post{}, err
	}
	stored.Title = in.Title
	stored.Slug = in.Slug
	stored.Body = in.Body
	stored.PublishedAt = in.PublishedAt
	return s.repo.Update(ctx, g, stored)
}

// Delete removes a post.
func (s *PostService) Delete(ctx context.Context, actor security.Subject, id string) error {
	view, err := security.Authorize(ctx, s.policy, actor, policies.PostView, models.Post{})
	if err != nil {
		return err
	}
	stored, err := s.repo.Find(ctx, view, id)
	if err != nil {
		return err
	}

	g, err := security.Authorize(ctx, s.policy, actor, policies.PostDelete, stored)
	if err != nil {
		return err
	}
	if err := s.repo.Delete(ctx, g, id); err != nil {
		return err
	}
	if col := observability.FromContext(ctx); col != nil {
		col.RecordEvent("post.deleted", stored)
	}
	return nil
}

// arandu:begin custom
// Business rules beyond CRUD go here, and survive regeneration.
// arandu:end custom
