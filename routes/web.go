// Package routes is where this application declares what it answers.
//
// Two files: web.go for what a browser reaches, and
// console.go for what the command line does. There is no api.go -- the handler
// decides between a JSON body and an HTML fragment, and a second router for the
// same resources would be a second place to forget a policy (doc 28).
package routes

import (
	"github.com/arandu-io/framework/http"
	"github.com/arandu-io/framework/http/middleware"
	"github.com/arandu-io/framework/security"
	"github.com/arandu-io/hesape/routing"
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
	//
	// LoadSubject because the listing is public and still wants to know who is
	// looking: a signed-in reader sees the drafts too. It sends nobody anywhere,
	// and a request without a session reaches the controller as a guest.
	r.Action("GET", "/{$}", d.Post.Index, middleware.LoadSubject(d.Sessions)).Name("home")

	// The screen somebody lands on after signing in, and it is for them alone.
	// Without the guard it answers 200 to anybody: the controller reads the
	// subject off the request, treats its absence as the anonymous case, and
	// renders -- which is right for a landing page and wrong for this one. The
	// guard is what makes the difference, and it is on the route because that is
	// where a reader of this file can see it.
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
	//
	// The listing and the article are read by anybody, so they only load who
	// is looking; the form, the write and the delete need somebody signed in.
	// The guard is on each route rather than in the controller, which reads
	// whoever the guard put on the request and loads no session of its own.
	guard(r.Resource("posts", d.Post), d.Sessions, "posts.index", "posts.show")

	// The comment thread hangs off the post, because that is where it is read
	// and where it is written. A top-level /comments would be a second address
	// for the same conversation, and the id in the path is the post's -- so the
	// route says which thread without the body having to be trusted about it.
	r.Action("POST", "/posts/{id}/comments", d.Comment.Store, middleware.RequireAuth(d.Sessions)).Name("posts.comments")

	// The sections. Two addresses, and they are two on purpose.
	//
	// /categories is the administrator's: the seven REST routes, behind the
	// policy, for inventing and renaming sections. /c/{slug} is the reader's --
	// short, guessable, and the address that ends up in a link somebody shares.
	//
	// A slug and not an id, because this one is read by people. The id is what a
	// form posts; the slug is what a URL says.
	guard(r.Resource("categories", d.Category), d.Sessions)
	r.Action("GET", "/c/{slug}", d.Post.Section, middleware.LoadSubject(d.Sessions)).Name("categories.section")

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
	// would be a second way to do the one thing, and it would be the slower
	// one.
	//
	// One middleware, and it is about what joaju needs from the pipeline rather
	// than about who may connect -- that is SocketConnectPolicy's answer, and it
	// runs inside the handler. joaju authenticates nobody: it reads the subject
	// off the request context and answers 401 when there is none, so the route
	// loads it from the session cookie, the way every public page here does.
	// LoadSubject and not RequireAuth, because RequireAuth answers a visitor
	// with a redirect to the sign-in screen, and a socket client cannot follow
	// one; a 401 from joaju is the answer it can read.
	//
	// The upgrade itself needs nothing from this file. Every global middleware
	// wraps the writer -- middleware.Observe to record the status, and under
	// APP_ENV=dev the live-reload recorder -- and each wrapper implements
	// Unwrap, which is the chain joaju's upgrader follows to the connection.
	// tests/Feature/Socket_test.go opens a socket through the whole pipeline in
	// both environments and expects 101.
	r.Get("/app/{appKey}", d.Socket.ServeHTTP, middleware.LoadSubject(d.Sessions))

	// Moderation is its own area, behind its own middleware, and it is where an
	// administrator sees what is waiting. See routes/admin.go.
	adminRoutes(r, d)
	// arandu:end custom
}

// guard puts every route a Resource call registered behind the guard it needs,
// and is how the table says which of the seven are open.
//
// The names in public are read by anybody and only want to know who is
// looking, so they get LoadSubject, which sends nobody anywhere. Every other
// action needs somebody signed in and gets RequireAuth, which sends a visitor
// with no session to sign in and remembers where they were going. Either way
// the subject is on the request when the controller runs, and the controller
// reads it with ctx.User() rather than loading the session a second time.
//
// Neither guard decides anything about a record. Whether this person may read
// this post or change that section is still the Policy's answer, on every
// service call the action makes.
func guard(routes []*routing.Route, sessions *security.SessionStore, public ...string) {
	for _, route := range routes {
		if len(public) > 0 && route.Named(public...) {
			route.Middleware(middleware.LoadSubject(sessions))
			continue
		}
		route.Middleware(middleware.RequireAuth(sessions))
	}
}
