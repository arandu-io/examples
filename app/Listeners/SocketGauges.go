// Package listeners holds what this application is told after the fact.
//
// Nothing here decides anything, and every method returns nothing on purpose: a
// listener that could refuse would be a second authorization path beside the
// policies in app/Policies, and there is one.
package listeners

import (
	"context"

	"github.com/arandu-io/framework/observability"
	"github.com/arandu-io/joaju"
)

// The names this application publishes its socket numbers under.
//
// A constant each rather than a string at both ends: SocketGauges writes them
// and the screen reads them, and two literals that have to match are two chances
// to change one and not the other.
const (
	// SocketConnections is how many sockets a tenant holds open right now. It
	// goes down.
	SocketConnections = "sockets.connections"

	// SocketChannels is how many channels a tenant has alive right now. It goes
	// down.
	SocketChannels = "sockets.channels"

	// SocketMessagesReceived is how many frames arrived from clients since the
	// process started. It is filed under no tenant, for the reason
	// SocketGauges.publishTotals gives.
	SocketMessagesReceived = "sockets.messages.received"

	// SocketMessagesSent is how many frames were written to clients since the
	// process started, filed the same way as SocketMessagesReceived.
	SocketMessagesSent = "sockets.messages.sent"
)

// SocketGauges counts what the realtime server does and publishes the current
// numbers, so that the screen drawing them reads a registry instead of the
// server.
//
// It owns the counter it keeps them in, and nothing outside can reach it. That
// is the whole point: a screen holding the live counter reads the process
// directly, and then everything that wants one of these numbers has to be handed
// the thing that produces it. Here the producer writes and the readers read, and
// they never meet.
//
// Every method writes the value that is true after the event it was told about,
// so no reading is ever older than the last event. There is no goroutine
// sampling on a timer and nothing to expire: the registry holds one reading per
// name and replaces it.
//
// Safe for concurrent use, which it has to be -- the server calls these from
// every connection's goroutine. It holds a counter and a registry, and both are.
type SocketGauges struct {
	counts *joaju.Counter
	gauges *observability.Gauges
}

// NewSocketGauges returns the observer, publishing into gauges.
//
// It builds its own counter rather than accepting one, because a counter a
// caller also holds is a second way to read these numbers.
func NewSocketGauges(gauges *observability.Gauges) *SocketGauges {
	return &SocketGauges{counts: joaju.NewCounter(), gauges: gauges}
}

// ChannelCreated records the first subscription to a channel that did not exist.
func (s *SocketGauges) ChannelCreated(ctx context.Context, name joaju.ChannelName) {
	s.counts.ChannelCreated(ctx, name)
	s.publish(name.Tenant())
}

// ChannelRemoved records the last subscriber leaving a channel.
func (s *SocketGauges) ChannelRemoved(ctx context.Context, name joaju.ChannelName) {
	s.counts.ChannelRemoved(ctx, name)
	s.publish(name.Tenant())
}

// ConnectionOpened records a socket that finished the upgrade and was
// authorized.
func (s *SocketGauges) ConnectionOpened(ctx context.Context, id joaju.SocketID, tenant string) {
	s.counts.ConnectionOpened(ctx, id, tenant)
	s.publish(tenant)
}

// ConnectionClosed records a socket that went away, whatever ended it.
func (s *SocketGauges) ConnectionClosed(ctx context.Context, id joaju.SocketID, tenant, reason string) {
	s.counts.ConnectionClosed(ctx, id, tenant, reason)
	s.publish(tenant)
}

// MessageReceived records a frame that arrived from a client.
func (s *SocketGauges) MessageReceived(ctx context.Context, id joaju.SocketID, message []byte) {
	s.counts.MessageReceived(ctx, id, message)
	s.publishTotals()
}

// MessageSent records a frame written to a client.
func (s *SocketGauges) MessageSent(ctx context.Context, id joaju.SocketID, message []byte) {
	s.counts.MessageSent(ctx, id, message)
	s.publishTotals()
}

// publish writes one tenant's connection and channel counts as they stand now.
func (s *SocketGauges) publish(tenant string) {
	count := s.counts.Read(tenant)
	s.gauges.Set(observability.GaugeName{Metric: SocketConnections, Tenant: tenant}, count.Connections)
	s.gauges.Set(observability.GaugeName{Metric: SocketChannels, Tenant: tenant}, count.Channels)
}

// publishTotals writes the frame counts, which belong to the process rather than
// to any tenant.
//
// The two message events carry a socket id and no tenant. Resolving one would
// mean keeping a socket-to-tenant map beside the server's, kept in step by hand
// and wrong the first time the two disagree, so the counter files them under a
// sentinel instead of inventing an attribution. The registry has a place for
// exactly that case: the empty tenant, which is what a number that cannot
// honestly be attributed to one reads as.
func (s *SocketGauges) publishTotals() {
	totals := s.counts.Read(counterAllTenants)
	s.gauges.Set(observability.GaugeName{Metric: SocketMessagesReceived}, totals.Received)
	s.gauges.Set(observability.GaugeName{Metric: SocketMessagesSent}, totals.Sent)
}

// counterAllTenants is where the counter files the two message events.
//
// The string is spelled here because the constant in joaju is unexported, which
// is the right way round: publishing it is joaju's decision to make, not this
// application's to depend on. The day it becomes exported, this line is the one
// that changes.
const counterAllTenants = "*"

// The server is built with this value, so a method that stops matching the
// interface has to fail here rather than at the line that wires it.
var _ joaju.Observer = (*SocketGauges)(nil)
