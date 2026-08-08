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

// CommentService holds the business rules. It receives its dependencies through
// the constructor -- explicit wiring, no container.
type CommentService struct {
	repo   *repositories.CommentRepository
	policy policies.CommentPolicy
}

// NewCommentService wires the service.
func NewCommentService(repo *repositories.CommentRepository) *CommentService {
	return &CommentService{repo: repo}
}

// Create walks the mandatory path: validate, Authorize, Grant, Repository.
// There is no other order that compiles.
func (s *CommentService) Create(ctx context.Context, actor security.Subject, in requests.StoreComment) (models.Comment, error) {
	if errs := in.Validate(); errs.Any() {
		return models.Comment{}, errs
	}

	candidate := models.Comment{
		PostId:   in.PostId,
		Author:   in.Author,
		Body:     in.Body,
		Approved: in.Approved,
	}

	g, err := security.Authorize(ctx, s.policy, actor, policies.CommentCreate, candidate)
	if err != nil {
		return models.Comment{}, err
	}

	created, err := s.repo.Create(ctx, g, candidate)
	if err != nil {
		return models.Comment{}, err
	}
	// Guarded: the entity is a struct value, and boxing it into `any` allocates
	// at the call site even though RecordEvent is a no-op on a nil Collector.
	if col := observability.FromContext(ctx); col != nil {
		col.RecordEvent("comment.created", created)
	}
	return created, nil
}

// Get returns one comment.
func (s *CommentService) Get(ctx context.Context, actor security.Subject, id string) (models.Comment, error) {
	g, err := security.Authorize(ctx, s.policy, actor, policies.CommentView, models.Comment{})
	if err != nil {
		return models.Comment{}, err
	}
	return s.repo.Find(ctx, g, id)
}

// List returns a page of comments.
func (s *CommentService) List(ctx context.Context, actor security.Subject, q data.Query) ([]models.Comment, error) {
	g, err := security.Authorize(ctx, s.policy, actor, policies.CommentList, models.Comment{})
	if err != nil {
		return nil, err
	}
	return s.repo.List(ctx, g, q)
}

// ForPost is the thread of one post, oldest first, because a conversation reads
// forwards.
//
// It goes through Authorize like every other read. A comment thread is exactly
// the query where "it is only a listing" gets said, and docs/26 refuses it:
// a read path without a policy is a tenant leak with a technical name.
func (s *CommentService) ForPost(ctx context.Context, actor security.Subject, postID string) ([]models.Comment, error) {
	g, err := security.Authorize(ctx, s.policy, actor, policies.CommentList, models.Comment{})
	if err != nil {
		return nil, err
	}

	// Filtered in the repository rather than here: pulling every comment of
	// every post and discarding most of them is the shape of a query that works
	// in development and falls over in production.
	return s.repo.ForPost(ctx, g, postID)
}

// Approve makes a comment public.
//
// It is a separate action from Update because it is a separate permission: the
// author may correct their own words and only an administrator may publish
// them, and one method taking a boolean would put both behind one policy check.
func (s *CommentService) Approve(ctx context.Context, actor security.Subject, id string) (models.Comment, error) {
	found, err := s.Get(ctx, actor, id)
	if err != nil {
		return models.Comment{}, err
	}

	g, err := security.Authorize(ctx, s.policy, actor, policies.CommentUpdate, found)
	if err != nil {
		return models.Comment{}, err
	}

	found.Approved = true
	return s.repo.Update(ctx, g, found)
}

// Update changes the mutable fields.
//
// It reads before writing, so the policy decides against the stored row rather
// than against what the client claims the row is. Skipping this is how a check
// passes on attacker-supplied data.
func (s *CommentService) Update(ctx context.Context, actor security.Subject, in requests.UpdateComment) (models.Comment, error) {
	if errs := in.Validate(); errs.Any() {
		return models.Comment{}, errs
	}

	view, err := security.Authorize(ctx, s.policy, actor, policies.CommentView, models.Comment{})
	if err != nil {
		return models.Comment{}, err
	}
	stored, err := s.repo.Find(ctx, view, in.ID)
	if err != nil {
		return models.Comment{}, err
	}

	g, err := security.Authorize(ctx, s.policy, actor, policies.CommentUpdate, stored)
	if err != nil {
		return models.Comment{}, err
	}
	stored.PostId = in.PostId
	stored.Author = in.Author
	stored.Body = in.Body
	stored.Approved = in.Approved
	return s.repo.Update(ctx, g, stored)
}

// Delete removes a comment.
func (s *CommentService) Delete(ctx context.Context, actor security.Subject, id string) error {
	view, err := security.Authorize(ctx, s.policy, actor, policies.CommentView, models.Comment{})
	if err != nil {
		return err
	}
	stored, err := s.repo.Find(ctx, view, id)
	if err != nil {
		return err
	}

	g, err := security.Authorize(ctx, s.policy, actor, policies.CommentDelete, stored)
	if err != nil {
		return err
	}
	if err := s.repo.Delete(ctx, g, id); err != nil {
		return err
	}
	if col := observability.FromContext(ctx); col != nil {
		col.RecordEvent("comment.deleted", stored)
	}
	return nil
}

// arandu:begin custom
// Business rules beyond CRUD go here, and survive regeneration.
// arandu:end custom
