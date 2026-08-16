// Package routes is where this application declares what it answers.
//
// Two files: web.go for what a browser reaches, and
// console.go for what the command line does. There is no api.go -- the handler
// decides between a JSON body and an HTML fragment, and a second router for the
// same resources would be a second place to forget a policy (doc 28).
package routes

import (
	"bufio"
	"net"
	nethttp "net/http"

	"github.com/arandu-io/framework/http"
	"github.com/arandu-io/framework/http/middleware"
	"github.com/arandu-io/framework/security"
	"github.com/arandu-io/hesape/auth"
	"github.com/arandu-io/joaju"

	controllers "github.com/arandu-io/examples/app/Http/Controllers"
	"github.com/arandu-io/examples/public"
)

// Deps carries the controllers the routes dispatch to.
//
// A struct rather than a growing parameter list, and explicit rather than
// resolved from a container: reading bootstrap/app.go tells you what every route
// was given, which is the property a dependency container costs you.
type Deps struct {
	Home     *controllers.HomeController
	Post     *controllers.PostController
	Comment  *controllers.CommentController
	Category *controllers.CategoryController
	Admin    *controllers.AdminController
	Sitemap  *controllers.SitemapController
	// Sockets answers the operator's screen: what the socket server is holding.
	// It is a controller of its own because what it reads crosses tenants -- see
	// controllers.SocketsController.
	Sockets *controllers.SocketsController

	// Socket is the socket server itself. It is not a controller: joaju owns the
	// upgrade, the frames and the address it answers on, and this application
	// mounts it rather than wrapping it.
	//
	// The concrete type and not net/http.Handler, for the reason every field
	// above is concrete: a nil interface cannot have a method taken off it, so a
	// Deps with this one left out would panic while the table was being built --
	// which is what a route test that fills in one controller does.
	Socket *joaju.Server

	// Sessions is what the route guards read. It is here rather than reached
	// through a controller because a guard runs BEFORE the controller: it is a
	// property of the route, and the route table is what says which addresses
	// have one.
	Sessions *security.SessionStore
}

// Web registers the browser-facing routes.
//
// Name() is what makes a route addressable by name,
// so a link is built from r.Table().URL("home") and a renamed path does not
// leave a dead href behind:
//
//	r.Get("/", handler).Name("home")
//	r.Action("GET", "/dashboard", ctrl.Index, middleware.RequireAuth(d.Sessions)).Name("dashboard")
//	r.Resource("invoices", invoiceController)      // the seven REST routes
//	admin := r.Group("/admin", middleware.RequireRole(d.Sessions, "admin"))
//
// Resource registers only the actions the controller implements, so a route that
// exists is a route that answers.
//
// The guard belongs on the route and not inside the handler. A check written in
// the controller is a check the next handler does not have, and it is written
// where nobody reading the table can see it -- this file is what says which
// addresses are open.
func Web(r *http.Router, d Deps) {
	// "/{$}" and not "/". This is the one place Go's router does not behave the
	// way it conventionally does: a pattern ending in a slash matches every path below
	// it, so "GET /" would answer for /anything -- including the 404s, and
	// including /_arandu/debug when the console is not mounted. The {$} anchors
	// the match to the end of the path, which is what Route::get('/') means.
	//
	// The front page of a blog is the blog. d.Home is still here and still
	// registered -- it is the screen somebody lands on after signing in -- but
	// the address a reader arrives at is the listing, because a front page that
	// says "you are logged in" to the people who are and nothing to the people
	// who are not is a front page for nobody.
	r.Action("GET", "/{$}", d.Post.Index).Name("home")

	// The screen somebody lands on after signing in, and it is for them alone.
	// It used to answer 200 to anybody: the controller reads the session, treats
	// a failure to load one as the anonymous case, and renders -- which is right
	// for a landing page and wrong for this one. The guard is what makes the
	// difference, and it is on the route because that is where a reader of this
	// file can see it.
	//
	// It decides nothing beyond "there is a session". What this person may read
	// is still the Policy's answer, on every service call the screen makes.
	r.Action("GET", "/dashboard", d.Home.Index, middleware.RequireAuth(d.Sessions)).Name("dashboard")

	// The fixed names the outside world asks for: /favicon.ico, which the layout
	// links, and /robots.txt, which a crawler fetches without being told to.
	// They are embedded in the binary and there is no document root -- see the
	// public package. Without this line the icon in the tab is a 404.
	public.Routes(r)

	// arandu:begin custom
	// The seven REST routes of the blog, named: posts.index, posts.show,
	// posts.create, posts.store, posts.edit, posts.update and posts.destroy.
	// A link is built from the name, so renaming the path does not leave a dead
	// href behind.
	r.Resource("posts", d.Post)

	// The comment thread hangs off the post, because that is where it is read
	// and where it is written. A top-level /comments would be a second address
	// for the same conversation, and the id in the path is the post's -- so the
	// route says which thread without the body having to be trusted about it.
	r.Action("POST", "/posts/{id}/comments", d.Comment.Store).Name("posts.comments")

	// The sections. Two addresses, and they are two on purpose.
	//
	// /categories is the administrator's: the seven REST routes, behind the
	// policy, for inventing and renaming sections. /c/{slug} is the reader's --
	// short, guessable, and the address that ends up in a link somebody shares.
	//
	// A slug and not an id, because this one is read by people. The id is what a
	// form posts; the slug is what a URL says.
	r.Resource("categories", d.Category)
	r.Action("GET", "/c/{slug}", d.Post.Section).Name("categories.section")

	// The sitemap, built from this table and the published posts. robots.txt
	// points at it, and it is a route rather than a file because a file would go
	// stale the first time a post was written.
	r.Action("GET", "/sitemap.xml", d.Sitemap.Index).Name("sitemap")

	// The WebSocket. One route, at the address joaju's own table expects, so a
	// Pusher client configured against this host finds it where it looks.
	//
	// The other eight routes joaju answers are its HTTP API, and they are not
	// mounted. They exist for a socket server running as a separate process,
	// which has to be told over HTTP what to broadcast; this application holds
	// the broker in memory, so publishing here is a method call. Mounting them
	// would be a second way to do the one thing (RULE 9), and it would be the
	// slower one.
	//
	// Two middlewares, and both are about what joaju needs from the pipeline
	// rather than about who may connect -- that is SocketConnectPolicy's answer,
	// and it runs inside the handler. withSubject is what makes the route work at
	// all: joaju reads the subject off the request context and answers 401 when
	// there is none. hijackable is what lets the upgrade happen through the
	// global middleware; see its own comment.
	r.Get("/app/{appKey}", d.Socket.ServeHTTP, withSubject(d.Sessions), upgradable)

	// Moderation is its own area, behind its own middleware, and it is where an
	// administrator sees what is waiting. See routes/admin.go.
	adminRoutes(r, d)
	// arandu:end custom
}

// withSubject puts the signed-in subject on the request context.
//
// It is the front door joaju's server documents and does not provide: that
// server authenticates nobody, it reads the subject somebody else put there and
// asks a Policy about it. In this application the somebody else is this, and the
// subject comes off the session cookie -- never off a header, a query string or
// the body, which is the finding `aru doctor` calls tenant-from-request.
//
// A request with no session passes through untouched, and joaju answers it 401.
// The alternative -- a guest subject invented here -- would be this application
// deciding that an anonymous visitor may open a socket, and that decision belongs
// to policies.SocketConnectPolicy, which refuses it.
func withSubject(sessions *security.SessionStore) http.Middleware {
	return func(next nethttp.Handler) nethttp.Handler {
		return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
			subject, err := sessions.Load(r.Context(), r)
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}
			next.ServeHTTP(w, r.WithContext(auth.WithSubject(r.Context(), subject)))
		})
	}
}

// upgradable hands the socket route a ResponseWriter that can be hijacked.
//
// Without it this route answers 500 to a perfectly good handshake, with
// "this connection cannot be upgraded" in the body. joaju's upgrader reaches the
// connection through a `w.(http.Hijacker)` type assertion, and what arrives here
// is middleware.Observe's wrapper -- which records the status and the byte count
// for the access log, forwards Flush, and does not forward Hijack.
//
// The wrapper is not the mistake. It implements Unwrap, which is what Go added
// http.ResponseController for in 1.20, and its own comment says that is how
// hijacking keeps working behind it. The assertion is the pre-1.20 idiom, and it
// sees a wrapper rather than the connection. So this walks the chain the way the
// standard library intends and puts the result back behind the interface the
// upgrader asks for.
//
// It is a stopgap and it says so: the fix is one line in joaju's ws.Upgrader,
// and when that lands this middleware comes off the route rather than being
// kept as a second way to reach the connection (RULE 9). It is on this one route
// and never in the global pipeline -- every other handler in this application
// wants the wrapper exactly as it is.
//
// It is also not enough in development, and that is a second defect in a second
// place. APP_ENV=dev adds the framework's live-reload middleware, whose
// htmlRecorder wraps the writer to inject the reload script and implements no
// Unwrap at all -- so the chain this walks stops there, one link short of the
// connection, and the handshake answers 500. Nothing in this application can
// reach past a wrapper that does not offer the method, so the socket is a
// production and staging feature until framework/foundation's recorder grows the
// same Unwrap middleware.Observe's already has.
func upgradable(next nethttp.Handler) nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		next.ServeHTTP(hijackable{ResponseWriter: w}, r)
	})
}

// hijackable is the writer upgradable installs: everything the pipeline built,
// plus the Hijack the upgrader looks for.
type hijackable struct{ nethttp.ResponseWriter }

// Hijack takes over the connection, through whatever wrappers are in the way.
func (h hijackable) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return nethttp.NewResponseController(h.ResponseWriter).Hijack()
}

// Unwrap keeps the chain intact for anything else that walks it: this writer is
// one more link, not the end of the line.
func (h hijackable) Unwrap() nethttp.ResponseWriter { return h.ResponseWriter }
