package controllers

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	fhttp "github.com/arandu-io/framework/http"
	"github.com/arandu-io/framework/observability"
	"github.com/arandu-io/framework/security"
	"github.com/arandu-io/kyse/components"

	listeners "github.com/arandu-io/examples/app/Listeners"
	policies "github.com/arandu-io/examples/app/Policies"
	admin "github.com/arandu-io/examples/storage/framework/views/admin"
)

// SocketsController answers the one screen in this application that is the
// operator's rather than a customer's.
//
// Every other controller here reads records of one tenant, through a service,
// through a policy, filtered by the tenant on the Grant. This one reads the
// process: how many sockets are held, for whom, and how many frames have crossed
// since it started. The gauge registry it reads is not scoped to a tenant and
// holds every tenant the process has seen, so the read crosses them.
//
// That is why it is a controller of its own with a policy of its own. It is the
// boundary written down: the read is authorized by policies.SocketInspectAll on
// policies.AllTenantSockets, an action and a resource that exist so the crossing
// has a name, and neither is reachable from any other screen. Adding a socket
// count to the moderation dashboard would have put a cross-tenant read behind the
// door that opens for a comment.
//
// It owns no data and holds no service. The numbers are memory in this process,
// there is nothing to page through and nothing to write, and a repository here
// would be a repository for something that is not in a database.
type SocketsController struct {
	Controller

	// gauges is where the socket server's observer publishes. The controller
	// reads it and never reaches the server: a screen holding the live counter
	// would be a screen reading the process directly, and every number on it
	// would arrive by a path with no policy on it.
	gauges *observability.Gauges
	// metrics is the only authority over this screen. It is a field rather than
	// a package-level value so that the tenant it compares against is the one
	// bootstrap configured, and not a constant this file decided on.
	metrics policies.SocketMetricsPolicy

	sessions *security.SessionStore
	csrf     *security.CSRF
}

// NewSocketsController returns the controller. bootstrap builds it, with the
// registry the socket server publishes into.
func NewSocketsController(gauges *observability.Gauges, metrics policies.SocketMetricsPolicy, sessions *security.SessionStore, csrf *security.CSRF) *SocketsController {
	return &SocketsController{gauges: gauges, metrics: metrics, sessions: sessions, csrf: csrf}
}

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
	// the numbers of every tenant in the process. A read is no exception, and a
	// read with no policy behind it is the leak with a technical name.
	//
	// The Grant it issues carries the operator's own tenant, which is what marks
	// their row in a table that crosses tenants. The registry itself takes no
	// Grant -- it is a map in memory, not a repository -- so there is no second
	// check downstream to fall back on.
	grant, err := security.Authorize(ctx.Ctx(), c.metrics, actor, policies.SocketInspectAll, policies.AllTenantSockets{})
	if err != nil {
		observability.Log(ctx.Ctx()).Warn("authorization denied", "error", err)
		return ctx.Status(http.StatusForbidden)
	}

	return ctx.View("admin.sockets", admin.SocketsData{
		Chrome:   adminChrome(ctx, actor, token, "Sockets"),
		Tenants:  c.connections(security.Tenant(grant)),
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
//
// The tenants are the ones the connection gauge has been published for, and the
// registry hands its names back sorted -- so the table does not reshuffle itself
// between two readings of the same screen.
//
// The frame totals never appear here. They are published under no tenant at all,
// which is a name this loop cannot mistake for a customer, and a row for them
// would read as a customer with no connections and every message in the process.
func (c *SocketsController) connections(own string) []components.StatRow {
	names := c.gauges.Names()

	rows := make([]components.StatRow, 0, len(names))
	for _, name := range names {
		if name.Metric != listeners.SocketConnections {
			continue
		}

		label := name.Tenant
		if name.Tenant == own {
			label += " (this deployment)"
		}

		rows = append(rows, components.StatRow{
			Label: label,
			Values: []string{
				c.value(listeners.SocketConnections, name.Tenant),
				c.value(listeners.SocketChannels, name.Tenant),
			},
		})
	}

	return rows
}

// messages is the one row of frame totals, labelled as the whole process.
//
// One row and not one per tenant, and the label says so rather than leaving the
// reader to assume: the two events that carry a frame carry a socket and no
// tenant, so the numbers are published under none.
//
// This row reads zero however busy the server is, and that is a decision rather
// than a gap waiting on a release: only one of the two events could honestly be
// announced -- a frame a client sends reaches the protocol, while a frame a
// client receives is usually written by a channel delivering a broadcast, which
// the protocol never sees. Counting one side would show a server that receives a
// thousand messages and sends none, and a number wrong in a knowable direction
// is worse than the zero it replaces.
//
// The card is drawn anyway, because the number in it is the true one: what was
// published. Hiding the row would hide the decision too.
func (c *SocketsController) messages() []components.StatRow {
	return []components.StatRow{{
		Label: "Every tenant, together",
		Values: []string{
			c.value(listeners.SocketMessagesReceived, ""),
			c.value(listeners.SocketMessagesSent, ""),
		},
	}}
}

// value is one reading, formatted for the table.
//
// The registry says whether a name was ever written, and this screen drops that
// answer on purpose: a gauge sitting at zero and a gauge nothing has written yet
// are the same fact here -- nothing has happened -- and the row reads zero for
// both.
func (c *SocketsController) value(metric, tenant string) string {
	n, _ := c.gauges.Read(observability.GaugeName{Metric: metric, Tenant: tenant})
	return number(n)
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
