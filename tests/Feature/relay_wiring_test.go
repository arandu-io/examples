package feature_test

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/events"
	"github.com/arandu-io/framework/observability"
	"github.com/arandu-io/framework/security"

	"github.com/arandu-io/examples/app/Policies"
	"github.com/arandu-io/examples/bootstrap"
	"github.com/arandu-io/examples/tests"
)

// The relay THIS application wires, from the form to the listener.
//
// relay_test.go proves what a relay does, against an outbox a test filled in
// itself and a publisher it built for the occasion. This file proves the
// narrower thing that was missing: that this application has one. The rows come
// out of the registration handler a browser reaches, and the relay that
// publishes them is the value bootstrap.Build returned -- so an application that
// went back to events.NewModule() fails at the first line here, rather than
// accumulating rows in a table no process reads, which is the shape this failure
// has everywhere else and the reason it survives so long.
//
// Nothing below starts the loop, and nothing has to. Start belongs to
// kernel.Background and is called by Kernel.Run, never by Kernel.Boot, so a
// booted-but-not-served application publishes exactly when a test says so. That
// is what makes "published once" a countable claim instead of a race with a
// ticker.

func TestARegistrationReachesTheListenerThroughTheWiredRelay(t *testing.T) {
	booted := tests.Boot(t)
	if booted.App.Relay == nil {
		t.Fatal("the application wired no relay: every event it stores would sit in the outbox unread")
	}

	// Two events, both through the handlers a person drives: the form writes
	// auth.user.registered and the code in the message writes
	// auth.email.verified. Neither is stored by this test.
	register(t, booted.Client)
	code := verificationCode(t, booted.Mail)
	booted.Client.Get("/auth/verify?email=" + newReader).OK()
	booted.Client.Post("/auth/verify/confirm", map[string]string{
		"email": newReader, "email_code": code,
	}).OK().See("confirmed")

	ctx := context.Background()
	outbox := events.NewOutbox(booted.DB)

	pending, err := outbox.PendingAll(ctx, 10)
	if err != nil {
		t.Fatalf("PendingAll: %v", err)
	}
	if len(pending) != 2 {
		t.Fatalf("the registration stored %d events, want 2: %v", len(pending), storedNames(pending))
	}

	seen := &publishedEvents{}
	logged := observability.WithLogger(ctx, slog.New(seen))

	if err := booted.App.Relay.Drain(logged); err != nil {
		t.Fatalf("Drain: %v", err)
	}

	published := seen.all()
	if len(published) != 2 {
		t.Fatalf("the listener was handed %d events, want 2: %v", len(published), published)
	}

	// Order is the order they happened. An address cannot be confirmed before the
	// account it belongs to exists, and a consumer that saw the two the other way
	// round would be reacting to a user it has never heard of.
	if published[0]["event"] != "auth.user.registered" || published[1]["event"] != "auth.email.verified" {
		t.Fatalf("delivered in the wrong order: %s then %s", published[0]["event"], published[1]["event"])
	}

	// The Grant was sealed into the row inside the transaction that wrote it, and
	// this is the far end of that: it survived the commit, the poll and the
	// publish. The tenant is what scopes the event and the action is what says
	// which authorization produced it, and they are two claims rather than one.
	for _, e := range published {
		if e["tenant"] != bootstrap.Tenant() {
			t.Errorf("%s arrived under tenant %q, want %q", e["event"], e["tenant"], bootstrap.Tenant())
		}
		if e["id"] == "" {
			t.Errorf("%s arrived with no id, which is what a consumer deduplicates on", e["event"])
		}
	}

	// The two differ on purpose, and that is the sharper half of the claim: a
	// listener handed the same pair for both would be reading something other
	// than the row. Registering is authorized for a declared guest, which has no
	// id; confirming an address runs on a system Grant, which is called system.
	if got := published[0]["action"]; got != string(policies.ActionUserCreate) {
		t.Errorf("the registration arrived with action %q, want %s", got, policies.ActionUserCreate)
	}
	if got := published[0]["authorized_by"]; got != "" {
		t.Errorf("the registration arrived authorized by %q, want the guest's absent subject", got)
	}
	if got := published[1]["action"]; got != string(policies.ActionUserUpdate) {
		t.Errorf("the confirmation arrived with action %q, want %s", got, policies.ActionUserUpdate)
	}
	if got := published[1]["authorized_by"]; got != "system" {
		t.Errorf("the confirmation arrived authorized by %q, want system", got)
	}

	// Marked, so the backlog is gone. A pass that published and did not mark is
	// the failure the next assertion is about.
	if left, err := outbox.PendingAll(ctx, 10); err != nil || len(left) != 0 {
		t.Fatalf("%d events are still pending after a pass (%v)", len(left), err)
	}

	// Published once. At-least-once is what a consumer has to tolerate, not what
	// this settles for: a second pass over a marked row delivers nothing.
	if err := booted.App.Relay.Drain(logged); err != nil {
		t.Fatalf("second Drain: %v", err)
	}
	if again := seen.all(); len(again) != 2 {
		t.Errorf("a second pass delivered %d more events", len(again)-2)
	}
}

// TestTheProbeFailsWhileNothingIsDrainingTheOutbox.
//
// The relay is registered WITH the module and not merely built beside it, and
// this is what tells those two apart from outside the process. Health and
// Diagnose both answer nothing when the module holds no relay, so an application
// that built one and still registered events.NewModule() would report itself
// healthy with a backlog behind it -- which is precisely the state the probe
// exists to catch, because a relay that stopped looks exactly like a relay with
// nothing to do.
func TestTheProbeFailsWhileNothingIsDrainingTheOutbox(t *testing.T) {
	booted := tests.Boot(t)

	booted.Client.Get("/_arandu/health").OK()

	// Older than the threshold, and stored rather than inserted: OccurredAt is a
	// field of the event, so the age is the fixture and the write is still the
	// one production makes.
	//arandu:system-grant a fixture needs a Grant with no request behind it, to store an event nobody submitted
	g := security.SystemGrant("invoice.pay", bootstrap.Tenant())
	err := data.Transaction(context.Background(), booted.DB, func(ctx context.Context) error {
		return events.NewOutbox(booted.DB).Store(ctx, g, []events.Event{{
			Name:        "invoice.paid",
			Aggregate:   "invoice",
			AggregateID: "i-1",
			OccurredAt:  time.Now().UTC().Add(-5 * time.Minute),
		}})
	})
	if err != nil {
		t.Fatalf("storing: %v", err)
	}

	body := booted.Client.Get("/_arandu/health").Status(http.StatusServiceUnavailable).Body()
	if !strings.Contains(body, "events") {
		t.Errorf("the probe does not name the module holding the backlog: %q", body)
	}

	seen := &publishedEvents{}
	if err := booted.App.Relay.Drain(observability.WithLogger(context.Background(), slog.New(seen))); err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if len(seen.all()) != 1 {
		t.Fatalf("the relay delivered %d events, want 1", len(seen.all()))
	}

	booted.Client.Get("/_arandu/health").OK()
}

// storedNames is the event names of a batch, which is what a failure message
// about the wrong batch is actually asking for.
func storedNames(list []events.Stored) []string {
	out := make([]string, 0, len(list))
	for _, e := range list {
		out = append(out, e.Name)
	}
	return out
}

// publishedEvents keeps the lines the listener wrote.
//
// Writing the event down is what listeners.EventLog does, so this is how a test
// watches the application's own publisher work rather than substituting one of
// its own. It reads the attributes and not the rendered text: a check against a
// formatted sentence passes over a missing tenant and fails over a reworded
// message, which is the wrong answer both ways round.
//
// Nothing else logs on this path -- the relay writes only on a failure, and the
// data layer writes nothing -- so an unexpected record is a surprise the counts
// above are meant to report rather than hide.
type publishedEvents struct {
	mu      sync.Mutex
	records []map[string]string
}

// Enabled accepts every level. A handler that filtered would be a second place
// for an assertion to come up empty without saying why.
func (p *publishedEvents) Enabled(context.Context, slog.Level) bool { return true }

// Handle keeps one record's attributes, flattened to strings.
func (p *publishedEvents) Handle(_ context.Context, r slog.Record) error {
	attrs := make(map[string]string, r.NumAttrs())
	r.Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value.String()
		return true
	})

	p.mu.Lock()
	defer p.mu.Unlock()
	p.records = append(p.records, attrs)
	return nil
}

// WithAttrs returns the handler unchanged: the listener passes its attributes to
// Info inline, so there is nothing to accumulate, and a copy would be a second
// place the records could land.
func (p *publishedEvents) WithAttrs([]slog.Attr) slog.Handler { return p }

// WithGroup returns the handler unchanged, for the reason WithAttrs does.
func (p *publishedEvents) WithGroup(string) slog.Handler { return p }

// all is what was recorded, oldest first.
//
// A copy under the lock, because the relay publishes on the caller's goroutine
// here and on its own under Run, and a test that read the slice directly would
// be the one place in this file that stopped being true in production.
func (p *publishedEvents) all() []map[string]string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]map[string]string(nil), p.records...)
}
