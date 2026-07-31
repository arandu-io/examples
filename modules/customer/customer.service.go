package customer

import (
	"context"
	"fmt"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/observability"
	"github.com/arandu-io/framework/security"
)

// Service holds the business rules. It receives its dependencies through the
// constructor -- explicit wiring, no container.
type Service struct {
	repo   *Repo
	policy Policy
}

// NewService wires the module.
func NewService(repo *Repo) *Service { return &Service{repo: repo} }

// Create walks the mandatory path once, and this is the method to read first:
// validate, Authorize, Grant, Repository. Every generated module will look like
// this, because there is no other order that compiles.
func (s *Service) Create(ctx context.Context, actor security.Subject, in CreateRequest) (Customer, error) {
	if errs := in.Validate(); errs.Any() {
		return Customer{}, errs
	}

	candidate := Customer{
		TenantID: actor.Tenant,
		Name:     in.Name,
		Email:    in.Email,
		Document: in.Document,
	}

	// No Grant, no database. This line is the only way to get one.
	g, err := security.Authorize(ctx, s.policy, actor, ActionCreate, candidate)
	if err != nil {
		return Customer{}, err
	}

	created, err := s.repo.Create(ctx, g, candidate)
	if err != nil {
		return Customer{}, err
	}

	// The event carries the entity, and the entity redacts itself: the document
	// never reaches the debug page even though it is right there in the struct.
	observability.FromContext(ctx).RecordEvent("customer.created", created)
	return created, nil
}

// Get returns one customer.
func (s *Service) Get(ctx context.Context, actor security.Subject, id string) (Customer, error) {
	g, err := security.Authorize(ctx, s.policy, actor, ActionView, Customer{})
	if err != nil {
		return Customer{}, err
	}
	return s.repo.Find(ctx, g, id)
}

// List returns a page of customers.
func (s *Service) List(ctx context.Context, actor security.Subject, q data.Query) ([]Customer, error) {
	g, err := security.Authorize(ctx, s.policy, actor, ActionView, Customer{})
	if err != nil {
		return nil, err
	}
	return s.repo.List(ctx, g, q)
}

// Update changes the mutable fields.
func (s *Service) Update(ctx context.Context, actor security.Subject, in UpdateRequest) (Customer, error) {
	if errs := in.Validate(); errs.Any() {
		return Customer{}, errs
	}

	// Read before write, so the policy decides against the stored row rather than
	// against what the client claims the row is. Skipping this is how a check
	// passes on data the attacker supplied.
	view, err := security.Authorize(ctx, s.policy, actor, ActionView, Customer{})
	if err != nil {
		return Customer{}, err
	}
	stored, err := s.repo.Find(ctx, view, in.ID)
	if err != nil {
		return Customer{}, err
	}

	g, err := security.Authorize(ctx, s.policy, actor, ActionUpdate, stored)
	if err != nil {
		return Customer{}, err
	}

	stored.Name, stored.Email, stored.Document = in.Name, in.Email, in.Document
	return s.repo.Update(ctx, g, stored)
}

// Delete removes a customer.
func (s *Service) Delete(ctx context.Context, actor security.Subject, id string) error {
	view, err := security.Authorize(ctx, s.policy, actor, ActionView, Customer{})
	if err != nil {
		return err
	}
	stored, err := s.repo.Find(ctx, view, id)
	if err != nil {
		return err
	}

	g, err := security.Authorize(ctx, s.policy, actor, ActionDelete, stored)
	if err != nil {
		return err
	}
	if err := s.repo.Delete(ctx, g, id); err != nil {
		return err
	}
	observability.FromContext(ctx).RecordEvent("customer.deleted", stored)
	return nil
}

// FullDocument returns the unmasked registration number.
//
// It is a separate method with a separate action on purpose. Reading a customer
// and reading every field of that customer are different permissions, and a
// system that treats them as one discovers the difference during an incident.
func (s *Service) FullDocument(ctx context.Context, actor security.Subject, id string) (string, error) {
	view, err := security.Authorize(ctx, s.policy, actor, ActionView, Customer{})
	if err != nil {
		return "", err
	}
	stored, err := s.repo.Find(ctx, view, id)
	if err != nil {
		return "", err
	}

	if _, err := security.Authorize(ctx, s.policy, actor, ActionViewFull, stored); err != nil {
		return "", err
	}

	// Reading a full document is an audit event, always -- that is the whole
	// point of the extra permission.
	observability.Log(ctx).Info("full document read", "customer", stored, "actor", actor.ID)
	observability.FromContext(ctx).RecordEvent("customer.document_read", stored)
	return stored.Document, nil
}

// ListAs lists using a Grant the caller already holds, instead of a subject.
//
// It exists for the two places that legitimately have no subject: seeding, and
// the demonstration that needs an id from another tenant to try. Everything else
// goes through List, which starts from a subject and ends at a policy.
//
// The signature is what keeps this honest: the caller has to produce a Grant, and
// outside the security package the only way to do that is security.Authorize or
// security.SystemGrant -- both auditable, neither forgeable.
func (s *Service) ListAs(ctx context.Context, g security.Grant, q data.Query) ([]Customer, error) {
	return s.repo.List(ctx, g, q)
}

// EnsureSeed creates a customer during seeding, where there is no subject yet.
//
// It uses SystemGrant, which is auditable by design: `aru doctor --strict` lists
// every call site, and the tenant is required, so this cannot reach across
// tenants even by accident.
func (s *Service) EnsureSeed(ctx context.Context, tenant string, c Customer) (Customer, error) {
	g := security.SystemGrant(ActionCreate, tenant)
	created, err := s.repo.Create(ctx, g, c)
	if err != nil {
		return Customer{}, fmt.Errorf("seeding customer %s: %w", c.Email, err)
	}
	return created, nil
}
