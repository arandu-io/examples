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

// Create normalizes protected fields, then walks the mandatory path: validate,
// Authorize, Grant, Repository. There is no other order that compiles.
func (s *CommentService) Create(ctx context.Context, actor security.Subject, in requests.StoreComment) (models.Comment, error) {
	// Protected fields are normalized before validation. A browser form should
	// not have to submit an author merely because UpdateComment shares this
	// validation shape, and a submitted value must never choose the answer.
	in.Author = actor.ID
	in.Approved = false
	if errs := in.Validate(); errs.Any() {
		return models.Comment{}, errs
	}

	candidate := models.Comment{
		PostId: in.PostId,
		// Authorship is an authorization fact, not form data. Keeping this in
		// the service protects every transport that can call it, not only the
		// browser handler shipped today.
		Author: in.Author,
		Body:   in.Body,
		// Publishing is the moderator's separate action. Creation always enters
		// the queue, even when a caller sets the input field directly.
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
//
// The first Authorize answers whether the caller may look at comments at all.
// The second, with the row that was read, is the object-level decision — a
// policy that branches on the entity's fields is only consulted the second time.
func (s *CommentService) Get(ctx context.Context, actor security.Subject, id string) (models.Comment, error) {
	g, err := security.Authorize(ctx, s.policy, actor, policies.CommentView, models.Comment{})
	if err != nil {
		return models.Comment{}, err
	}

	found, err := s.repo.Find(ctx, g, id)
	if err != nil {
		return models.Comment{}, err
	}
	if _, err := security.Authorize(ctx, s.policy, actor, policies.CommentView, found); err != nil {
		return models.Comment{}, err
	}
	return found, nil
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
// the query where "it is only a listing" gets said, and it is refused all the
// same: a read path without a policy is a tenant leak with a technical name.
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

// PublicForPost is the thread as a reader sees it: approved, plus their own
// while it waits for review.
//
// The reader is taken from the actor and never from an argument, so "show me my
// pending comments" cannot be asked on somebody else's behalf.
func (s *CommentService) PublicForPost(ctx context.Context, actor security.Subject, postID string) ([]models.Comment, error) {
	g, err := security.Authorize(ctx, s.policy, actor, policies.CommentPublicList, models.Comment{})
	if err != nil {
		return nil, err
	}
	return s.repo.PublicForPost(ctx, g, postID, actor.ID)
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
