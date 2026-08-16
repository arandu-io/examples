package policies

import (
	"context"
	"fmt"

	"github.com/arandu-io/framework/security"
)

// SocketInspectAll is reading the socket counts of EVERY tenant at once.
//
// The ".all" is the whole of what distinguishes it, and it is not decoration. A
// tenant's own numbers are a different question with a different answer -- joaju
// answers that one itself, through Server.Connections, filtered by the tenant on
// the Grant. This action is the question that has no tenant in it, and it exists
// as a name of its own so that a handler cannot ask the tenant's question and be
// handed the operator's answer.
const SocketInspectAll security.Action = "socket.inspect.all"

// AllTenantSockets is what SocketInspectAll is asked about: the socket counts
// this process is holding, across every tenant in it.
//
// It is an empty struct, and the name is the entire content. joaju.Counter.Tenants
// says in its own doc comment that it is not reachable from a request handler --
// the HTTP routes of a socket server answer for the tenant on the Grant, and that
// list crosses tenants. Reaching it from a handler anyway is allowed exactly once,
// under a resource whose name cannot be mistaken for a tenant's, so that the
// crossing is written down at the point the decision is made instead of being
// implied by a function call three files away.
type AllTenantSockets struct{}

// SocketMetricsPolicy is the only authority over the operator's read of the
// socket counts.
//
// It is a policy of its own, in a file of its own, and that separation is the
// point rather than tidiness. Every other policy in this package answers about a
// record of one tenant, reached through a repository that filters by
// data.Tenant(g). This one answers about the process: how many sockets it holds,
// for whom, and how many frames have crossed it. Those numbers are the operator's
// -- somebody looking at a deployment -- and they are not a screen a customer
// sees. Filing it with CommentPolicy would put a cross-tenant read behind the
// same door as a comment, and the door would eventually be opened for the comment.
//
// # Who the operator is here, and what has to change when that stops being true
//
// This deployment serves one tenant: config.DefaultTenant, or whatever
// ARANDU_TENANT_ID replaced it with, fixed at boot by auth.FixedTenant. With one
// tenant, its administrator IS the operator of the process -- there is no second
// customer whose numbers they could be reading, so the crossing crosses nothing.
//
// The moment a second tenant exists that stops being true, and the line that has
// to change is the role comparison below. It is one line, it is here, and it is
// alone: an administrator of one customer must not be told how busy another one
// is, which is the fact joaju.TenantCount's own doc calls a business fact rather
// than a metric. What replaces it is a role no tenant can grant itself -- and
// until this application has such a role, this policy is honest about standing in
// for it rather than pretending the question does not arise.
type SocketMetricsPolicy struct {
	// Tenant is the tenant this deployment serves, and the comparison against it
	// is what makes the paragraph above enforceable rather than a promise: a
	// subject signed into anything else is refused here, whatever roles it
	// carries.
	Tenant string
}

// Compile-time proof that the policy answers about the process's counts and no
// record of anybody's.
var _ security.Policy[AllTenantSockets] = SocketMetricsPolicy{}

// Can decides one read of the counts.
func (p SocketMetricsPolicy) Can(_ context.Context, s security.Subject, a security.Action, _ AllTenantSockets) error {
	if a != SocketInspectAll {
		return fmt.Errorf("no rule allows %s on the socket counts", a)
	}

	// A guest is refused before anything else is asked. How many people are
	// connected, and how much traffic a process is carrying, is not something a
	// deployment tells whoever asks.
	if s.IsGuest() {
		return fmt.Errorf("the socket counts are not public")
	}
	if s.Tenant != p.Tenant {
		return fmt.Errorf("this application serves the tenant %q and the subject carries %q", p.Tenant, s.Tenant)
	}
	if !s.HasRole(AdminRole) {
		return fmt.Errorf("reading the socket counts of every tenant is the operator's, and %q is not one", s.ID)
	}

	return nil
}
