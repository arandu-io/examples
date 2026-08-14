package routes

import (
	"net/http"

	fhttp "github.com/arandu-io/framework/http"
	"github.com/arandu-io/framework/http/middleware"
)

// adminRoutes registers the moderation area.
//
// An area rather than a prefix, and the difference is the group: every route
// below shares one prefix and the same middleware, so a handler somebody adds
// next month is protected by having been added here rather than by having
// remembered.
//
// It is a file of its own because that is what makes the boundary visible. A
// dozen /admin/... lines mixed into the public table is a table where nobody can
// tell at a glance what is behind a check and what is not.
func adminRoutes(r *fhttp.Router, d Deps) {
	// Two middlewares and they do two different jobs. adminOnly is about the
	// response -- crawlers and caches. RequireRole is about who: it keeps the
	// area to accounts carrying the role, and sends somebody with no session to
	// the sign-in screen instead of answering 403 at a person who has not been
	// asked to identify themselves yet.
	//
	// It still decides nothing about a comment. CommentPolicy does, on every
	// handler below, and a signed-in administrator whose policy refuses them is
	// refused by the policy -- which is the arrangement that survives somebody
	// adding a route here next month and forgetting everything but the group.
	admin := r.Group("/admin", adminOnly, middleware.RequireRole(d.Sessions, "admin"))

	admin.Action("GET", "/{$}", d.Admin.Index).Name("admin.index")
	admin.Action("GET", "/comments", d.Admin.Comments).Name("admin.comments")

	// Both change something, so both are POST. A moderation action reachable by
	// GET is one a crawler fires by following a link, and "approve" is exactly
	// the word a crawler would follow.
	admin.Action("POST", "/comments/{id}/approve", d.Admin.Approve).Name("admin.comments.approve")
	admin.Action("POST", "/comments/{id}/delete", d.Admin.Destroy).Name("admin.comments.destroy")
}

// adminOnly keeps the area out of the index.
//
// It decides nothing about a record: that is the policy's, and every handler
// below still goes through the service and the Grant, so a signed-in reader who
// reaches an address here is refused by CommentPolicy and not by this.
//
// What it does is the pair of things a policy cannot: it tells crawlers to stay
// out, at the header rather than only in the markup, and it refuses to let any
// answer from the area be cached by a proxy in front of the application. The
// count of what is waiting for review is information, and a shared cache holding
// it is a leak no policy sees.
func adminOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Robots-Tag", "noindex, nofollow")
		w.Header().Set("Cache-Control", "no-store, private")
		next.ServeHTTP(w, r)
	})
}
