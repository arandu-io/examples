package unit_test

import (
	"context"
	"testing"

	"github.com/arandu-io/framework/security"
	"github.com/arandu-io/hesape/broadcasting"
	"github.com/arandu-io/joaju"

	policies "github.com/arandu-io/examples/app/Policies"
)

// The tenant the three socket policies are built for. It is the one this
// deployment serves, and every case below that carries another one is refused
// for that reason alone.
const socketTenant = "t1"

// TestASocketIsHeldByAnAccountAndNotByAVisitor.
//
// Reading this blog is public and holding a socket against it is not. The
// distinction is worth a test because it is the one place the two halves of
// joaju's authorization could be collapsed into one: a connect policy that
// admitted a guest would put an anonymous file descriptor on the process for
// every page view, and the per-channel policy would be the only thing left
// deciding anything.
func TestASocketIsHeldByAnAccountAndNotByAVisitor(t *testing.T) {
	connect := policies.SocketConnectPolicy{Tenant: socketTenant}
	_, reader, _, admin := subjects()

	for _, c := range []struct {
		who     string
		subject security.Subject
		action  security.Action
		allowed bool
	}{
		{"a reader may hold a socket", reader, joaju.Connect, true},
		{"a moderator may hold a socket", admin, joaju.Connect, true},
		{"a visitor may not", security.Guest(socketTenant), joaju.Connect, false},
		{"somebody else's tenant may not", security.Subject{ID: "u9", Tenant: "t2"}, joaju.Connect, false},
		{"and no other action is answered here", reader, broadcasting.ChannelJoin, false},
	} {
		t.Run(c.who, func(t *testing.T) {
			err := connect.Can(context.Background(), c.subject, c.action, joaju.Handshake{Socket: "1.1"})
			if c.allowed && err != nil {
				t.Fatalf("refused: %v", err)
			}
			if !c.allowed && err == nil {
				t.Fatal("allowed")
			}
		})
	}
}

// TestOnlyTheChannelsThisBlogHasAreReachable.
//
// The policy names its channels one case each and refuses the rest, so a channel
// nobody has invented yet is refused before anybody decides it should not be.
// A reader holding a perfectly good socket reaches none of them, which is the
// half that a connect policy alone could not express.
func TestOnlyTheChannelsThisBlogHasAreReachable(t *testing.T) {
	subscribe := policies.SocketSubscribePolicy{Tenant: socketTenant}
	_, reader, _, admin := subjects()

	for _, c := range []struct {
		who     string
		subject security.Subject
		channel string
		allowed bool
	}{
		{"a moderator hears the queue", admin, policies.ModerationChannel, true},
		{"a reader does not", reader, policies.ModerationChannel, false},
		{"nobody hears a channel that was never declared", admin, "private-anything-else", false},
		{"not even a public one", admin, "updates", false},
		{"and a guest hears nothing", security.Guest(socketTenant), policies.ModerationChannel, false},
	} {
		t.Run(c.who, func(t *testing.T) {
			err := subscribe.Can(context.Background(), c.subject, broadcasting.ChannelJoin, joaju.Subscription{
				Channel: channelNamed(t, c.subject, c.channel),
				Socket:  "1.1",
			})
			if c.allowed && err != nil {
				t.Fatalf("refused: %v", err)
			}
			if !c.allowed && err == nil {
				t.Fatal("allowed")
			}
		})
	}
}

// TestTheSocketCountsAreTheOperatorsAndNotAReaders.
//
// The screen behind this policy reads across tenants, which joaju.Counter.Tenants
// says in its own doc comment is not a request handler's to do. This is the one
// place that does it, so this is the test that says who may: not a visitor, not a
// reader with a perfectly good session, and not somebody signed into another
// tenant however many roles they carry.
func TestTheSocketCountsAreTheOperatorsAndNotAReaders(t *testing.T) {
	metrics := policies.SocketMetricsPolicy{Tenant: socketTenant}
	_, reader, _, admin := subjects()

	for _, c := range []struct {
		who     string
		subject security.Subject
		action  security.Action
		allowed bool
	}{
		{"the operator reads them", admin, policies.SocketInspectAll, true},
		{"a reader does not", reader, policies.SocketInspectAll, false},
		{"a visitor does not", security.Guest(socketTenant), policies.SocketInspectAll, false},
		{"another tenant's administrator does not", security.Subject{ID: "u9", Tenant: "t2", Roles: []string{"admin"}}, policies.SocketInspectAll, false},
		{"and the action is not one another policy issues", admin, joaju.Connect, false},
	} {
		t.Run(c.who, func(t *testing.T) {
			err := metrics.Can(context.Background(), c.subject, c.action, policies.AllTenantSockets{})
			if c.allowed && err != nil {
				t.Fatalf("refused: %v", err)
			}
			if !c.allowed && err == nil {
				t.Fatal("allowed")
			}
		})
	}
}

// channelNamed builds the name the way joaju does: off a Grant, never off a
// string somebody wrote here. A test that assembled one by hand would be a test
// of a value the server cannot produce.
func channelNamed(t *testing.T, s security.Subject, requested string) joaju.ChannelName {
	t.Helper()

	g, err := security.Authorize(context.Background(), allowEverything{}, s, broadcasting.ChannelJoin, struct{}{})
	if err != nil {
		t.Fatalf("the fixture grant was refused: %v", err)
	}
	name, err := joaju.NewChannelName(g, requested)
	if err != nil {
		t.Fatalf("NewChannelName(%q): %v", requested, err)
	}
	return name
}

// allowEverything issues the Grant that a ChannelName needs a tenant from. It
// decides nothing about the policies under test -- those are asked directly, with
// the subject -- and it exists because ChannelName cannot be written as a struct
// literal, which is the property that keeps a tenant off the wire.
type allowEverything struct{}

func (allowEverything) Can(context.Context, security.Subject, security.Action, struct{}) error {
	return nil
}
