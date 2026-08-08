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
	g, err := security.Authorize(ctx, s.policy, actor, policies.PostView, models.Post{})
	if err != nil {
		return models.Post{}, err
	}
	return s.repo.Find(ctx, g, id)
}

// List returns a page of posts.
func (s *PostService) List(ctx context.Context, actor security.Subject, q data.Query) ([]models.Post, error) {
	g, err := security.Authorize(ctx, s.policy, actor, policies.PostList, models.Post{})
	if err != nil {
		return nil, err
	}
	return s.repo.List(ctx, g, q)
}

// ListWith is List for a caller that already holds a Grant.
//
// There is one of those in this application -- the sitemap, which answers a
// crawler that has no session -- and it exists so that caller goes through the
// service like every other, rather than reaching the repository directly. The
// Grant it hands over still has to carry PostList: the repository checks it, so
// a grant issued for another action is refused here as it would be anywhere.
func (s *PostService) ListWith(ctx context.Context, g security.Grant, q data.Query) ([]models.Post, error) {
	return s.repo.List(ctx, g, q)
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
