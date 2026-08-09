package controllers

import (
	"errors"
	"net/http"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/httpx"
	"github.com/arandu-io/framework/observability"
	"github.com/arandu-io/framework/security"
	"github.com/arandu-io/framework/view"

	models "github.com/arandu-io/examples/app/Models"
	services "github.com/arandu-io/examples/app/Services"
	admin "github.com/arandu-io/examples/storage/framework/views/admin"
)

// AdminController answers the moderation area.
//
// An area rather than a prefix: it has its own routes, its own middleware and
// its own directory of views, and what makes it an area is that every route in
// it goes through the same check. A `/admin/...` path with the check written per
// handler is a path where the one handler somebody forgot is the whole hole.
//
// It owns no data of its own. The posts and the comments are the same records
// the public screens read, through the same services and the same policies --
// an administration screen that reaches the database another way is a second
// enforcement point, and the second one is always the one that is wrong.
type AdminController struct {
	Controller

	posts    *services.PostService
	comments *services.CommentService
	sessions *security.SessionStore
	csrf     *security.CSRF
}

// NewAdminController returns the controller. bootstrap builds it.
func NewAdminController(posts *services.PostService, comments *services.CommentService, sessions *security.SessionStore, csrf *security.CSRF) *AdminController {
	return &AdminController{posts: posts, comments: comments, sessions: sessions, csrf: csrf}
}

// actor is who is asking, from the session cookie and never from the request.
func (c *AdminController) actor(ctx *httpx.Context) (security.Subject, error) {
	return c.sessions.Load(ctx.Ctx(), ctx.Request)
}

// signIn sends an unauthenticated visitor to the sign-in screen.
func (c *AdminController) signIn(ctx *httpx.Context) error {
	return ctx.Redirect("/auth/login")
}

// token issues a CSRF token for the session rendering the page. Every form in
// the area posts, so every screen needs one.
func (c *AdminController) token(ctx *httpx.Context) (string, error) {
	return c.csrf.Issue(c.sessions.IDFromRequest(ctx.Request))
}

// fail turns a service error into a status.
//
// Forbidden is 403 and not 404: somebody signed in who is not an administrator
// asked for a page that exists, and pretending otherwise would send them
// looking for a typo.
func (c *AdminController) fail(ctx *httpx.Context, err error) error {
	if errors.Is(err, security.ErrForbidden) {
		observability.Log(ctx.Ctx()).Warn("authorization denied", "error", err)
		return ctx.Status(http.StatusForbidden)
	}
	if errors.Is(err, models.ErrCommentNotFound) {
		return ctx.Status(http.StatusNotFound)
	}
	return err
}

// Index is the dashboard: what is waiting, and what there is.
func (c *AdminController) Index(ctx *httpx.Context) error {
	actor, err := c.actor(ctx)
	if err != nil {
		return c.signIn(ctx)
	}
	token, err := c.token(ctx)
	if err != nil {
		return err
	}

	posts, err := c.posts.List(ctx.Ctx(), actor, data.Query{Limit: 200})
	if err != nil {
		return c.fail(ctx, err)
	}
	comments, err := c.comments.List(ctx.Ctx(), actor, data.Query{Limit: 200})
	if err != nil {
		return c.fail(ctx, err)
	}

	published, drafts := 0, 0
	for _, p := range posts {
		if p.PublishedAt.IsZero() {
			drafts++
			continue
		}
		published++
	}

	waiting := 0
	for _, m := range comments {
		if !m.Approved {
			waiting++
		}
	}

	return ctx.View("admin.index", admin.IndexData{
		Chrome:    c.chrome(ctx, actor, token, "Dashboard"),
		Published: published,
		Drafts:    drafts,
		Comments:  len(comments),
		Waiting:   waiting,
	})
}

// Comments is the moderation queue.
func (c *AdminController) Comments(ctx *httpx.Context) error {
	actor, err := c.actor(ctx)
	if err != nil {
		return c.signIn(ctx)
	}
	token, err := c.token(ctx)
	if err != nil {
		return err
	}

	found, err := c.comments.List(ctx.Ctx(), actor, data.Query{Limit: 200})
	if err != nil {
		return c.fail(ctx, err)
	}

	rows := make([]admin.CommentRow, 0, len(found))
	for _, m := range found {
		rows = append(rows, admin.CommentRow{
			ID:         m.ID,
			Author:     m.Author,
			Body:       m.Body,
			Created:    m.CreatedAt.Format("2 January 2006, 15:04"),
			Approved:   m.Approved,
			ApproveURL: ctx.URL("admin.comments.approve", m.ID),
			DeleteURL:  ctx.URL("admin.comments.destroy", m.ID),
		})
	}

	return ctx.View("admin.comments", admin.CommentsData{
		Chrome:   c.chrome(ctx, actor, token, "Comments"),
		Comments: rows,
	})
}

// Approve publishes one comment and returns to the queue.
func (c *AdminController) Approve(ctx *httpx.Context) error {
	actor, err := c.actor(ctx)
	if err != nil {
		return c.signIn(ctx)
	}
	if _, err := c.comments.Approve(ctx.Ctx(), actor, ctx.Param("id")); err != nil {
		return c.fail(ctx, err)
	}
	return ctx.Redirect(ctx.URL("admin.comments"))
}

// Destroy removes one comment and returns to the queue.
func (c *AdminController) Destroy(ctx *httpx.Context) error {
	actor, err := c.actor(ctx)
	if err != nil {
		return c.signIn(ctx)
	}
	if err := c.comments.Delete(ctx.Ctx(), actor, ctx.Param("id")); err != nil {
		return c.fail(ctx, err)
	}
	return ctx.Redirect(ctx.URL("admin.comments"))
}

// chrome is the frame every screen of the area shares: the layout's state plus
// which item of the sidebar is the current one.
//
// It is built here rather than in the layout because the layout is markup and
// this is a decision -- which page you are on is something the router knows and
// a template would have to guess at by comparing paths.
func (c *AdminController) chrome(ctx *httpx.Context, actor security.Subject, token, current string) admin.Chrome {
	return admin.Chrome{
		Page: view.Page{
			Title: current + " · Admin",
			// The moderation area is not for search engines. It answers only to
			// a signed-in administrator, and a crawler that reached it would
			// index a sign-in redirect.
			Token:         token,
			Path:          ctx.Request.URL.Path,
			AppName:       "Admin",
			Authenticated: true,
			UserName:      actor.ID,
			HomeURL:       ctx.URL("home"),
			LogoutURL:     "/auth/logout",
		},
		Current:   current,
		Dashboard: ctx.URL("admin.index"),
		Posts:     ctx.URL("posts.index"),
		Comments:  ctx.URL("admin.comments"),
	}
}
