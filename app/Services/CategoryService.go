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

// CategoryService holds the business rules. It receives its dependencies through
// the constructor -- explicit wiring, no container.
type CategoryService struct {
	repo   *repositories.CategoryRepository
	policy policies.CategoryPolicy
}

// NewCategoryService wires the service.
func NewCategoryService(repo *repositories.CategoryRepository) *CategoryService {
	return &CategoryService{repo: repo}
}

// Create walks the mandatory path: validate, Authorize, Grant, Repository.
// There is no other order that compiles.
func (s *CategoryService) Create(ctx context.Context, actor security.Subject, in requests.StoreCategory) (models.Category, error) {
	if errs := in.Validate(); errs.Any() {
		return models.Category{}, errs
	}

	candidate := models.Category{
		Name:        in.Name,
		Slug:        in.Slug,
		Description: in.Description,
	}

	g, err := security.Authorize(ctx, s.policy, actor, policies.CategoryCreate, candidate)
	if err != nil {
		return models.Category{}, err
	}

	created, err := s.repo.Create(ctx, g, candidate)
	if err != nil {
		return models.Category{}, err
	}
	// Guarded: the entity is a struct value, and boxing it into `any` allocates
	// at the call site even though RecordEvent is a no-op on a nil Collector.
	if col := observability.FromContext(ctx); col != nil {
		col.RecordEvent("category.created", created)
	}
	return created, nil
}

// Get returns one category.
//
// The first Authorize answers whether the caller may look at categories at all.
// The second, with the row that was read, is the object-level decision — a
// policy that branches on the entity's fields is only consulted the second time.
func (s *CategoryService) Get(ctx context.Context, actor security.Subject, id string) (models.Category, error) {
	g, err := security.Authorize(ctx, s.policy, actor, policies.CategoryView, models.Category{})
	if err != nil {
		return models.Category{}, err
	}

	found, err := s.repo.Find(ctx, g, id)
	if err != nil {
		return models.Category{}, err
	}
	if _, err := security.Authorize(ctx, s.policy, actor, policies.CategoryView, found); err != nil {
		return models.Category{}, err
	}
	return found, nil
}

// List returns a page of categories.
func (s *CategoryService) List(ctx context.Context, actor security.Subject, q data.Query) ([]models.Category, error) {
	g, err := security.Authorize(ctx, s.policy, actor, policies.CategoryList, models.Category{})
	if err != nil {
		return nil, err
	}
	return s.repo.List(ctx, g, q)
}

// Update changes the mutable fields.
//
// It reads before writing, so the policy decides against the stored row rather
// than against what the client claims the row is. Skipping this is how a check
// passes on attacker-supplied data.
func (s *CategoryService) Update(ctx context.Context, actor security.Subject, in requests.UpdateCategory) (models.Category, error) {
	if errs := in.Validate(); errs.Any() {
		return models.Category{}, errs
	}

	view, err := security.Authorize(ctx, s.policy, actor, policies.CategoryView, models.Category{})
	if err != nil {
		return models.Category{}, err
	}
	stored, err := s.repo.Find(ctx, view, in.ID)
	if err != nil {
		return models.Category{}, err
	}

	g, err := security.Authorize(ctx, s.policy, actor, policies.CategoryUpdate, stored)
	if err != nil {
		return models.Category{}, err
	}
	stored.Name = in.Name
	stored.Slug = in.Slug
	stored.Description = in.Description
	return s.repo.Update(ctx, g, stored)
}

// Delete removes a category.
func (s *CategoryService) Delete(ctx context.Context, actor security.Subject, id string) error {
	view, err := security.Authorize(ctx, s.policy, actor, policies.CategoryView, models.Category{})
	if err != nil {
		return err
	}
	stored, err := s.repo.Find(ctx, view, id)
	if err != nil {
		return err
	}

	g, err := security.Authorize(ctx, s.policy, actor, policies.CategoryDelete, stored)
	if err != nil {
		return err
	}
	if err := s.repo.Delete(ctx, g, id); err != nil {
		return err
	}
	if col := observability.FromContext(ctx); col != nil {
		col.RecordEvent("category.deleted", stored)
	}
	return nil
}

// arandu:begin custom
// Business rules beyond CRUD go here, and survive regeneration.

// BySlug returns the section a reader's URL names.
func (s *CategoryService) BySlug(ctx context.Context, actor security.Subject, slug string) (models.Category, error) {
	g, err := security.Authorize(ctx, s.policy, actor, policies.CategoryView, models.Category{})
	if err != nil {
		return models.Category{}, err
	}
	return s.repo.FindBySlug(ctx, g, slug)
}

// All returns every section, for the navigation.
func (s *CategoryService) All(ctx context.Context, actor security.Subject) ([]models.Category, error) {
	g, err := security.Authorize(ctx, s.policy, actor, policies.CategoryList, models.Category{})
	if err != nil {
		return nil, err
	}
	return s.repo.All(ctx, g)
}

// arandu:end custom
