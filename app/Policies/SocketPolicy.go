package policies

import (
	"context"
	"fmt"

	"github.com/arandu-io/framework/security"
	"github.com/arandu-io/hesape/broadcasting"
	"github.com/arandu-io/joaju"
)

// ModerationChannel is the one channel this blog broadcasts on.
//
// A comment arriving for review is the only thing here that somebody is waiting
// to be told about, and the somebody is a moderator. The name carries the
// "private-" prefix the Pusher protocol reads a guarded channel off, so a
// subscription to it reaches SocketSubscribePolicy rather than being waved
// through as a public one.
//
// The tenant is NOT in this string. joaju puts it there, from the Grant, before
// any policy sees the name -- a channel name written with a tenant in it would be
// a client choosing whose events it hears.
const ModerationChannel = broadcasting.PrivateChannelPrefix + "moderation"

// AdminRole is the role the moderation area is kept to.
//
// It is the same string CommentPolicy and CategoryPolicy compare against, named
// here because the socket policies are the first pair to compare it twice in one
// decision -- and a role spelled by hand in five places is a role that is
// eventually spelled "admins" in one of them.
const AdminRole = "admin"

// SocketConnectPolicy is the only authority over who may hold a socket on this
// application.
//
// It answers the first of the two questions joaju asks, and it is deliberately
// the smaller one: whether this subject may be on this server at all. What they
// may then HEAR is SocketSubscribePolicy's answer, asked again for every channel
// they name. Answering both here would be one decision doing the work of two --
// joaju.Connect issues a Grant to hold a socket, not a Grant to listen.
//
// The subject arrives from the session and from nowhere else: joaju reads it off
// the request context, and the only thing in this application that puts one
// there is the middleware in routes/web.go, which loads the session cookie. The
// handshake carries an Origin, and it is not weighed here -- joaju's upgrader
// already refuses an Origin naming another host before the socket exists, and a
// policy can only narrow that, never widen it. A list of allowed origins in this
// file would be a second place that decides the same thing, and the second one
// is always the one that is wrong.
type SocketConnectPolicy struct {
	// Tenant is the tenant this deployment serves. A subject carrying another
	// one is a wiring mistake and not a customer, because nothing in this
	// application signs anybody into a second tenant.
	Tenant string
}

// Compile-time proof that the policy answers about the handshake and no other
// resource.
var _ security.Policy[joaju.Handshake] = SocketConnectPolicy{}

// Can decides one handshake.
//
// A reader with a session is admitted, and today there is nothing for them to
// hear: the one channel below is the moderator's. That is on purpose. Refusing
// the socket here would answer a reader with 403 on the upgrade, which is a
// browser-visible failure for something that is not a refusal -- they may be on
// this server, they simply have no channel yet. The counter on /admin/sockets is
// where an operator sees those sockets, and a socket held by nobody who can hear
// anything is worth seeing rather than worth hiding.
func (p SocketConnectPolicy) Can(_ context.Context, s security.Subject, a security.Action, _ joaju.Handshake) error {
	if a != joaju.Connect {
		return fmt.Errorf("no rule allows %s on a socket", a)
	}

	// A reader with no session arrives as security.Guest, which is a subject
	// somebody built on purpose, and a guest is refused here. Reading this blog
	// is public and listening to it is not: a socket is a connection held open
	// against this process, and holding one is something an account does.
	if s.IsGuest() {
		return fmt.Errorf("sign in before opening a socket")
	}

	// The tenant is compared and not read: whoever this is, they signed into
	// this deployment, and a subject from anywhere else reached this policy by a
	// path nobody wrote.
	if s.Tenant != p.Tenant {
		return fmt.Errorf("this application serves the tenant %q and the subject carries %q", p.Tenant, s.Tenant)
	}

	return nil
}

// SocketSubscribePolicy is the only authority over which channels a socket may
// hear.
//
// It runs for every channel, on every subscription, including the resubscription
// a client makes after a reconnect. Subscribing is a read, and a read is not an
// exception -- a channel no policy ever saw is a channel whose contents nobody
// decided anybody could have.
//
// # The signature the Pusher protocol carries is ignored, and that is the design
//
// joaju.Subscription.Auth holds the HMAC a browser offers for a guarded channel.
// This application does not read it. It has a front door of its own -- the
// session cookie, loaded before joaju sees the request -- so the subject on this
// call was already identified once, and a signature that could ALSO allow a
// subscription would be a second way to prove who is asking. The signature is
// the fallback for a socket server standing alone with no session to read, which
// this application is not.
type SocketSubscribePolicy struct {
	// Tenant is checked again here, and not inherited: a Grant issued for
	// joaju.Connect is a Grant to hold a socket, never a Grant to listen, and a
	// subject that reached this policy by another path is still refused.
	Tenant string
}

// Compile-time proof that the policy answers about the subscription and no other
// resource.
var _ security.Policy[joaju.Subscription] = SocketSubscribePolicy{}

// Can decides one subscription.
//
// It names the channels this application has, one case each, and refuses
// everything else -- including a channel nobody has invented yet. Written the
// other way round, with a default that allows what it does not recognise, the
// next channel added to this blog would be readable before anybody decided it
// should be.
func (p SocketSubscribePolicy) Can(_ context.Context, s security.Subject, a security.Action, sub joaju.Subscription) error {
	if a != broadcasting.ChannelJoin {
		return fmt.Errorf("no rule allows %s on a channel", a)
	}
	if s.IsGuest() {
		return fmt.Errorf("sign in before subscribing to a channel")
	}
	if s.Tenant != p.Tenant {
		return fmt.Errorf("this application serves the tenant %q and the subject carries %q", p.Tenant, s.Tenant)
	}
	if sub.Channel.IsZero() {
		return fmt.Errorf("the subscription named no channel")
	}

	// Requested and not String: the tenant is the first half of the published
	// name and it was never the client's to send, so comparing against the full
	// name would compare against a value this application would have to rebuild
	// -- and rebuild identically, forever.
	switch sub.Channel.Requested() {
	case ModerationChannel:
		// What is waiting for review is the moderator's, exactly as the queue
		// behind CommentList is. The two answers have to agree: a comment nobody
		// may list is a comment nobody may be told about the moment it arrives.
		if s.HasRole(AdminRole) {
			return nil
		}
	}

	// A presence channel would need one more comparison than any of the above:
	// joaju.Subscription.Member is what the CLIENT offered, so a policy that
	// allowed a presence channel without checking Member.UserID against the
	// subject would let a subscriber announce themselves as somebody else. There
	// is no presence channel here, and the day there is, that check belongs in
	// the case above it and not in the handler that broadcasts.
	return fmt.Errorf("no rule allows %s to hear %s", s.ID, sub.Channel.Requested())
}
