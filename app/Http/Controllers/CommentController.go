package controllers

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/httpx"
	"github.com/arandu-io/framework/observability"
	"github.com/arandu-io/framework/security"
	"github.com/arandu-io/framework/validation"
	"github.com/arandu-io/framework/view"

	requests "github.com/arandu-io/examples/app/Http/Requests"
	models "github.com/arandu-io/examples/app/Models"
	services "github.com/arandu-io/examples/app/Services"
	views "github.com/arandu-io/examples/resources/views/comments"
)

// CommentController answers the seven routes of the comments resource.
//
// It is thin on purpose: read the request, call the service, render. There is no
// repository here and there cannot be one -- httpx.Context carries no database
// handle, so a controller that reached the data layer would be a controller that
// skipped the service, and therefore skipped the policy.
type CommentController struct {
	Controller

	svc      *services.CommentService
	sessions *security.SessionStore
	csrf     *security.CSRF
}

// NewCommentController returns the controller. bootstrap builds it and hands it to
// the routes.
//
// The session store and the CSRF issuer arrive through the constructor rather
// than through the service: a screen is allowed to know about a token and a
// cookie, and a service is not allowed to expose its own dependencies.
func NewCommentController(svc *services.CommentService, sessions *security.SessionStore, csrf *security.CSRF) *CommentController {
	return &CommentController{svc: svc, sessions: sessions, csrf: csrf}
}

// Compile-time proof of the seven actions httpx.Router.Resource looks for. It
// registers the ones the controller implements and nothing else, so a route that
// exists is a route that answers -- and a renamed method fails the build here
// rather than answering 404 in production.
var (
	_ httpx.Indexer   = (*CommentController)(nil)
	_ httpx.Creator   = (*CommentController)(nil)
	_ httpx.Storer    = (*CommentController)(nil)
	_ httpx.Shower    = (*CommentController)(nil)
	_ httpx.Editor    = (*CommentController)(nil)
	_ httpx.Updater   = (*CommentController)(nil)
	_ httpx.Destroyer = (*CommentController)(nil)
)

// commentPerPage is how many records the listing asks for when the request
// does not say. The repository has a bound of its own: this one is about the
// screen, that one is about the database.
const commentPerPage = 25

// Index renders the listing.
func (c *CommentController) Index(ctx *httpx.Context) error {
	actor, err := c.actor(ctx)
	if err != nil {
		return c.signIn(ctx)
	}

	// The page size is decided here rather than passed through blindly: asking
	// for a known number is what lets the next cursor be offered only when a
	// full page came back.
	limit := commentPerPage
	if n, err := strconv.Atoi(ctx.Query("limit")); err == nil && n > 0 {
		limit = n
	}

	found, err := c.svc.List(ctx.Ctx(), actor, data.Query{
		Limit:  limit,
		Cursor: ctx.Query("cursor"),
		Sort:   ctx.Query("sort"),
	})
	if err != nil {
		return c.fail(ctx, err)
	}

	rows := make([]views.CommentRow, 0, len(found))
	for _, co := range found {
		rows = append(rows, c.row(co))
	}

	// The listing writes nothing, but the layout around it does: the sign-out
	// form and every hx- request read the token off the page data. A listing
	// rendered without one answers 200 and then refuses the next write with
	// 419, which reads like a broken session.
	token, err := c.token(ctx)
	if err != nil {
		return err
	}

	// Keyset pagination picks up after the last id of the page. A partial page
	// is the last page, and offering a cursor there would be a link to nothing.
	next := ""
	if len(rows) == limit {
		next = rows[len(rows)-1].ID
	}

	return ctx.View("comments.index", views.CommentsIndexData{
		Page:       view.Page{Title: "Comments", Token: token},
		Comments:   rows,
		NextCursor: next,
	})
}

// Show renders one record.
func (c *CommentController) Show(ctx *httpx.Context) error {
	actor, err := c.actor(ctx)
	if err != nil {
		return c.signIn(ctx)
	}

	found, err := c.svc.Get(ctx.Ctx(), actor, ctx.Param("id"))
	if err != nil {
		return c.fail(ctx, err)
	}

	// The token is for the delete button, which sends it as a header: an
	// hx-delete carries no form body, so the hidden field a form uses would
	// never arrive and the request would be refused with 419.
	token, err := c.token(ctx)
	if err != nil {
		return err
	}

	return ctx.View("comments.show", views.CommentsShowData{
		Page:    view.Page{Title: "Comment", Token: token},
		Comment: c.row(found),
	})
}

// Create renders the empty form.
func (c *CommentController) Create(ctx *httpx.Context) error {
	if _, err := c.actor(ctx); err != nil {
		return c.signIn(ctx)
	}
	token, err := c.token(ctx)
	if err != nil {
		return err
	}

	return ctx.View("comments.create", views.CommentsCreateData{
		Page:   view.Page{Title: "New comment", Token: token},
		Errors: map[string][]string{},
	})
}

// Store takes the submitted form.
func (c *CommentController) Store(ctx *httpx.Context) error {
	actor, err := c.actor(ctx)
	if err != nil {
		return c.signIn(ctx)
	}

	in, form, errs := c.input(ctx)

	// The post and the author come from the route and the session, never from
	// the form. A hidden field carrying either is a hidden field somebody edits:
	// one would let a comment be attached to any post, and the other would let
	// it be signed with anybody's name.
	in.PostId = ctx.Param("id")
	in.Author = actor.ID

	// A new comment is never public. Approving is the administrator's action,
	// and defaulting to true here would make the moderation queue a screen that
	// is always empty.
	in.Approved = false

	if !c.Validated(errs) {
		return c.rejectedCreate(ctx, form, errs)
	}

	created, err := c.svc.Create(ctx.Ctx(), actor, in)
	if err != nil {
		var invalid validation.Errors
		if errors.As(err, &invalid) {
			return c.rejectedCreate(ctx, form, invalid)
		}
		return c.fail(ctx, err)
	}
	// Back to the article, which is where the person is. The comment they wrote
	// is not in the thread yet -- it is waiting for review -- so the answer says
	// so rather than leaving them looking for it.
	_ = created
	return ctx.Redirect(ctx.URL("posts.show", in.PostId) + "?said=1")
}

// Edit renders the form filled in.
func (c *CommentController) Edit(ctx *httpx.Context) error {
	actor, err := c.actor(ctx)
	if err != nil {
		return c.signIn(ctx)
	}

	found, err := c.svc.Get(ctx.Ctx(), actor, ctx.Param("id"))
	if err != nil {
		return c.fail(ctx, err)
	}
	token, err := c.token(ctx)
	if err != nil {
		return err
	}

	return ctx.View("comments.edit", views.CommentsEditData{
		Page:   view.Page{Title: "Edit comment", Token: token},
		Form:   c.form(found),
		Errors: map[string][]string{},
	})
}

// Update writes the submitted form onto the stored record.
func (c *CommentController) Update(ctx *httpx.Context) error {
	actor, err := c.actor(ctx)
	if err != nil {
		return c.signIn(ctx)
	}

	in, form, errs := c.input(ctx)
	form.ID = ctx.Param("id")
	if !c.Validated(errs) {
		return c.rejectedEdit(ctx, form, errs)
	}

	updated, err := c.svc.Update(ctx.Ctx(), actor, requests.UpdateComment{
		ID:       ctx.Param("id"),
		PostId:   in.PostId,
		Author:   in.Author,
		Body:     in.Body,
		Approved: in.Approved,
	})
	if err != nil {
		var invalid validation.Errors
		if errors.As(err, &invalid) {
			return c.rejectedEdit(ctx, form, invalid)
		}
		return c.fail(ctx, err)
	}
	return ctx.Redirect("/comments/" + updated.ID)
}

// Destroy removes the record.
func (c *CommentController) Destroy(ctx *httpx.Context) error {
	actor, err := c.actor(ctx)
	if err != nil {
		return c.signIn(ctx)
	}
	if err := c.svc.Delete(ctx.Ctx(), actor, ctx.Param("id")); err != nil {
		return c.fail(ctx, err)
	}
	return ctx.Redirect("/comments")
}

// actor is who is acting, from the session and never from the request body.
func (c *CommentController) actor(ctx *httpx.Context) (security.Subject, error) {
	return c.sessions.Load(ctx.Ctx(), ctx.Request)
}

// signIn sends an unauthenticated visitor to the sign-in screen. Under HTMX the
// redirect becomes HX-Redirect, so the browser navigates instead of nesting the
// whole page inside a fragment.
func (c *CommentController) signIn(ctx *httpx.Context) error {
	return ctx.Redirect("/auth/login")
}

// token issues a CSRF token for the session that is rendering the page.
//
// Every page needs it, including the ones that write nothing: the sign-out form
// and every hx- request read it off the page data. A page rendered without one
// answers 200 and then refuses the next write with 419, which reads like a
// broken session rather than a missing field.
func (c *CommentController) token(ctx *httpx.Context) (string, error) {
	return c.csrf.Issue(c.sessions.IDFromRequest(ctx.Request))
}

// row turns the entity into what the markup renders.
//
// Formatting happens here rather than in the view: a view that formats a
// time.Time would need the time package, and what a date looks like on screen is
// a decision about presentation, which is this side of the line.
func (c *CommentController) row(co models.Comment) views.CommentRow {
	return views.CommentRow{
		ID:       co.ID,
		PostId:   co.PostId,
		Author:   co.Author,
		Body:     co.Body,
		Approved: co.Approved,
		Created:  co.CreatedAt.Format("2006-01-02 15:04"),
	}
}

// form fills the edit form from the stored record.
func (c *CommentController) form(co models.Comment) views.CommentForm {
	return views.CommentForm{
		ID:       co.ID,
		PostId:   co.PostId,
		Author:   co.Author,
		Body:     co.Body,
		Approved: co.Approved,
	}
}

// input reads the submitted form.
//
// It returns three things: the typed request the service takes, the form as it
// was typed -- so a rejected submission comes back filled in rather than blank --
// and the errors parsing itself found. A number that is not a number is rejected
// here, naming the field, rather than reaching the service as a silent zero.
func (c *CommentController) input(ctx *httpx.Context) (requests.StoreComment, views.CommentForm, validation.Errors) {
	errs := validation.Errors{}

	in := requests.StoreComment{
		PostId:   ctx.Input("post_id"),
		Author:   ctx.Input("author"),
		Body:     ctx.Input("body"),
		Approved: ctx.Input("approved") != "",
	}

	form := views.CommentForm{
		PostId:   ctx.Input("post_id"),
		Author:   ctx.Input("author"),
		Body:     ctx.Input("body"),
		Approved: ctx.Input("approved") != "",
	}

	// arandu:begin custom
	// Anything the form carries that the fields above do not: a value composed
	// of two inputs, a default that depends on the actor.
	// arandu:end custom

	return in, form, errs
}

// rejectedCreate re-renders the creation form with its errors, as the 422
// fragment HTMX swaps back in.
func (c *CommentController) rejectedCreate(ctx *httpx.Context, form views.CommentForm, errs validation.Errors) error {
	token, err := c.token(ctx)
	if err != nil {
		return err
	}
	return c.Invalid(ctx, "comments.create", views.CommentsCreateData{
		Page:   view.Page{Title: "New comment", Token: token},
		Form:   form,
		Errors: errs,
	})
}

// rejectedEdit re-renders the edit form with its errors.
func (c *CommentController) rejectedEdit(ctx *httpx.Context, form views.CommentForm, errs validation.Errors) error {
	token, err := c.token(ctx)
	if err != nil {
		return err
	}
	return c.Invalid(ctx, "comments.edit", views.CommentsEditData{
		Page:   view.Page{Title: "Edit comment", Token: token},
		Form:   form,
		Errors: errs,
	})
}

// fail turns a domain error into a status, in one place.
//
// Note what it does not do: it never writes the authorization error into the
// response. Why a policy said no is information about the system, and it belongs
// in the log. Anything unrecognized is returned, and the router turns it into
// the error page in development and a 500 in production.
func (c *CommentController) fail(ctx *httpx.Context, err error) error {
	switch {
	case errors.Is(err, security.ErrForbidden):
		observability.Log(ctx.Ctx()).Warn("authorization denied", "error", err)
		return ctx.Status(http.StatusForbidden)
	case errors.Is(err, models.ErrCommentNotFound):
		return ctx.Status(http.StatusNotFound)
	case errors.Is(err, models.ErrCommentConflict):
		return ctx.Status(http.StatusConflict)
	case errors.Is(err, models.ErrCommentSort):
		return ctx.Status(http.StatusBadRequest)
	default:
		return err
	}
}

// arandu:begin custom
// Actions beyond the seven go here, and survive regeneration. Register them in
// the custom block of routes/web.go.
// arandu:end custom
