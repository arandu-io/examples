// Package routes is where this application declares what it answers.
//
// Two files: web.go for what a browser reaches, and
// console.go for what the command line does. There is no api.go -- the handler
// decides between a JSON body and an HTML fragment, and a second router for the
// same resources would be a second place to forget a policy (doc 28).
package routes

import (
	"github.com/arandu-io/framework/httpx"

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
}

// Web registers the browser-facing routes.
//
// Name() is what makes a route addressable by name,
// so a link is built from r.Table().URL("home") and a renamed path does not
// leave a dead href behind:
//
//	r.Get("/", handler).Name("home")
//	r.Action("GET", "/dashboard", ctrl.Index).Name("dashboard")
//	r.Resource("invoices", invoiceController)      // the seven REST routes
//	admin := r.Group("/admin", middleware.RequireRole("admin"))
//
// Resource registers only the actions the controller implements, so a route that
// exists is a route that answers.
func Web(r *httpx.Router, d Deps) {
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
	r.Action("GET", "/dashboard", d.Home.Index).Name("dashboard")

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

	// Moderation is its own area, behind its own middleware, and it is where an
	// administrator sees what is waiting. See routes/admin.go.
	adminRoutes(r, d)
	// arandu:end custom
}
