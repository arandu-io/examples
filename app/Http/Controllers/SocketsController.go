package controllers

import (
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/arandu-io/framework/data"
	fhttp "github.com/arandu-io/framework/http"
	"github.com/arandu-io/framework/observability"
	"github.com/arandu-io/framework/security"
	"github.com/arandu-io/joaju"
	"github.com/arandu-io/kyse/components"

	policies "github.com/arandu-io/examples/app/Policies"
	admin "github.com/arandu-io/examples/storage/framework/views/admin"
)

// SocketsController answers the one screen in this application that is the
// operator's rather than a customer's.
//
// Every other controller here reads records of one tenant, through a service,
// through a policy, filtered by the tenant on the Grant. This one reads the
// process: how many sockets joaju is holding, for whom, and how many frames have
// crossed it since it started. joaju.Counter.Tenants -- the call that produces
// the list -- says in its own doc comment that it is NOT reachable from a request
// handler, because the socket server's own HTTP routes answer for the tenant on
// the Grant and that list crosses tenants.
//
// This handler reaches it anyway, and that is why it is a controller of its own
// with a policy of its own. It is the boundary written down: the read is
// authorized by policies.SocketInspectAll on policies.AllTenantSockets, an action
// and a resource that exist so the crossing has a name, and neither is reachable
// from any other screen. Adding a socket count to the moderation dashboard would
// have put a cross-tenant read behind the door that opens for a comment.
//
// It owns no data and holds no service. The numbers are memory in this process,
// there is nothing to page through and nothing to write, and a repository here
// would be a repository for something that is not in a database.
type SocketsController struct {
	Controller

	// counter is the joaju.Observer the socket server was built with. It is the
	// same value, not a copy: a second counter would count the sockets of a
	// server nobody is connected to, and read zero forever.
	counter *joaju.Counter
	// metrics is the only authority over this screen. It is a field rather than
	// a package-level value so that the tenant it compares against is the one
	// bootstrap configured, and not a constant this file decided on.
	metrics policies.SocketMetricsPolicy

	sessions *security.SessionStore
	csrf     *security.CSRF
}

// NewSocketsController returns the controller. bootstrap builds it, with the
// counter it handed to the socket server.
func NewSocketsController(counter *joaju.Counter, metrics policies.SocketMetricsPolicy, sessions *security.SessionStore, csrf *security.CSRF) *SocketsController {
	return &SocketsController{counter: counter, metrics: metrics, sessions: sessions, csrf: csrf}
}

// messageTotals is the tenant the message counts land under, and it is not a
// tenant.
//
// joaju.Counter buckets everything per tenant except the two message events,
// which it puts under this sentinel. The reason is on the events themselves:
// Observer.MessageReceived and Observer.MessageSent are handed a socket id and no
// tenant, and resolving one would mean the counter keeping its own socket-to-
// tenant map beside the server's -- a second registry of the same fact, kept in
// step by hand, wrong the first time the two disagree. joaju chose the honest
// total over the invented attribution, and this screen draws it as what it is:
// one line, labelled as the whole process, never mixed into the per-tenant table
// above it.
//
// The string is spelled here because the constant in joaju is unexported. That is
// the right way round -- it is joaju's decision to publish, not this application's
// to depend on -- and the day it becomes exported this line is the one that
// changes.
//
// As of joaju v0.2.0 this row reads zero however busy the server is: the two
// events exist on the Observer interface, joaju.Counter implements them, and
// nothing in the server or the protocol calls them yet. The card is drawn anyway,
// and the number in it is the true one -- what the counter holds. A screen that
// hid the row until the number moved would be a screen that shows nothing on the
// day the events start firing and nobody notices.
const messageTotals = "*"

// Index draws the two cards: connections per tenant, and the message totals.
func (c *SocketsController) Index(ctx *fhttp.Context) error {
	actor, err := c.sessions.Load(ctx.Ctx(), ctx.Request)
	if err != nil {
		return ctx.Redirect("/auth/login")
	}
	token, err := c.csrf.Issue(c.sessions.IDFromRequest(ctx.Request))
	if err != nil {
		return err
	}

	// The authorization, and it is not ceremony: nothing below this line touches
	// a database, so this call is the ONLY thing standing between a session and
	// the numbers of every tenant in the process. RULE 17 has no exception for
	// reads, and a read with no policy behind it is the leak with a technical
	// name.
	//
	// The Grant it issues carries the operator's own tenant, which is what marks
	// their row in a table that crosses tenants. The counter itself takes no
	// Grant -- it is a map in memory, not a repository -- so there is no second
	// check downstream to fall back on.
	grant, err := security.Authorize(ctx.Ctx(), c.metrics, actor, policies.SocketInspectAll, policies.AllTenantSockets{})
	if err != nil {
		observability.Log(ctx.Ctx()).Warn("authorization denied", "error", err)
		return ctx.Status(http.StatusForbidden)
	}

	return ctx.View("admin.sockets", admin.SocketsData{
		Chrome:   adminChrome(ctx, actor, token, "Sockets"),
		Tenants:  c.connections(data.Tenant(grant)),
		Messages: c.messages(),
		ReadAt:   time.Now().Format("2 January 2006, 15:04:05"),
	})
}

// connections is one row per tenant: open sockets and live channels.
//
// own is the tenant on the Grant, and its row is marked. The table crosses
// tenants on purpose, and the one row the reader can check against anything else
// they know is their own -- unmarked, a list of opaque tenant ids is a list
// nobody can orient themselves in.
func (c *SocketsController) connections(own string) []components.StatRow {
	tenants := c.counter.Tenants()
	// Sorted, because the counter holds a map and a map hands its keys back in a
	// different order every request. A table that reshuffles itself on refresh is
	// a table nobody can compare two readings of.
	slices.Sort(tenants)

	rows := make([]components.StatRow, 0, len(tenants))
	for _, tenant := range tenants {
		// The sentinel is not a tenant and never appears in this table. Drawn
		// here it would read as a customer with no connections and every message
		// in the process, which is exactly the wrong half of the truth.
		if tenant == messageTotals {
			continue
		}

		count := c.counter.Read(tenant)
		label := tenant
		if tenant == own {
			label += " (this deployment)"
		}

		rows = append(rows, components.StatRow{
			Label:  label,
			Values: []string{number(count.Connections), number(count.Channels)},
		})
	}

	return rows
}

// messages is the one row of frame totals, labelled as the whole process.
//
// One row and not one per tenant, and the label says so rather than leaving the
// reader to assume. See messageTotals for why the counter cannot say more.
func (c *SocketsController) messages() []components.StatRow {
	totals := c.counter.Read(messageTotals)

	return []components.StatRow{{
		Label:  "Every tenant, together",
		Values: []string{number(totals.Received), number(totals.Sent)},
	}}
}

// number is how this application writes a count.
//
// components.StatCard takes strings because it deliberately does not format:
// a thousands separator is a local decision, and a component that made it would
// make it wrong in half the places it is used. This is the decision for this
// application -- groups of three, separated by a comma -- made once, here.
func number(n int64) string {
	digits := strconv.FormatInt(n, 10)
	sign := ""
	if strings.HasPrefix(digits, "-") {
		sign, digits = "-", digits[1:]
	}

	var out strings.Builder
	for i, d := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			out.WriteByte(',')
		}
		out.WriteRune(d)
	}

	return sign + out.String()
}
