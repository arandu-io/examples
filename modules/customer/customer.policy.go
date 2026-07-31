package customer

import (
	"context"
	"fmt"

	"github.com/arandu-io/framework/security"
)

// Actions of this module. Constants rather than strings at the call site: a typo
// in an action name would silently authorize nothing, or worse, everything.
const (
	ActionView     security.Action = "customer.view"
	ActionCreate   security.Action = "customer.create"
	ActionUpdate   security.Action = "customer.update"
	ActionDelete   security.Action = "customer.delete"
	ActionViewFull security.Action = "customer.view_full_document"
)

// Roles this module understands.
const (
	RoleAdmin   = "admin"
	RoleSales   = "sales"
	RoleSupport = "support"
)

// Policy is the only authority over who does what with a Customer.
//
// It denies by default: the switch has no branch that allows without a reason,
// so an action nobody thought about is refused rather than permitted. That is the
// whole difference between a policy and a checklist.
type Policy struct{}

// Can decides whether the subject may perform the action on the customer.
func (Policy) Can(ctx context.Context, s security.Subject, a security.Action, c Customer) error {
	// Tenant isolation comes first and applies to every action. Without it every
	// check below would be pointless: an administrator of one company would be an
	// administrator of all of them.
	if c.ID != "" && c.TenantID != s.Tenant {
		return fmt.Errorf("customer belongs to another tenant")
	}

	switch a {
	case ActionView:
		if s.HasRole(RoleAdmin) || s.HasRole(RoleSales) || s.HasRole(RoleSupport) {
			return nil
		}
	case ActionCreate, ActionUpdate:
		if s.HasRole(RoleAdmin) || s.HasRole(RoleSales) {
			return nil
		}
	case ActionDelete:
		if s.HasRole(RoleAdmin) {
			return nil
		}
	case ActionViewFull:
		// Support can read a customer but never the full document: seeing a
		// record and seeing every field in it are different permissions, and
		// most systems only discover that after an incident.
		if s.HasRole(RoleAdmin) {
			return nil
		}
	}
	return fmt.Errorf("role insufficient for %s", a)
}
