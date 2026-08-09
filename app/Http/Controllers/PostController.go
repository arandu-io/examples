package controllers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/httpx"
	"github.com/arandu-io/framework/observability"
	"github.com/arandu-io/framework/security"
	"github.com/arandu-io/framework/validation"
	"github.com/arandu-io/framework/view"

	requests "github.com/arandu-io/examples/app/Http/Requests"
	models "github.com/arandu-io/examples/app/Models"
	services "github.com/arandu-io/examples/app/Services"
	views "github.com/arandu-io/examples/resources/views/posts"
)

// PostController answers the seven routes of the posts resource.
//
// It is thin on purpose: read the request, call the service, render. There is no
// repository here and there cannot be one -- httpx.Context carries no database
// handle, so a controller that reached the data layer would be a controller that
// skipped the service, and therefore skipped the policy.
type PostController struct {
	Controller

	svc      *services.PostService
	comments *services.CommentService
	sessions *security.SessionStore
	csrf     *security.CSRF

	// appName is the brand in the navigation bar and the og:site_name, and base
	// is the origin a canonical URL is absolute against. Both come from the
	// configuration through the constructor: a canonical built from the Host
	// header is one the client chose, which is how the same page ends up
	// declaring two different canonicals to a crawler.
	appName string
	base    string
	// tenant is what a guest reads under. It is the application's, from
	// configuration, and never from the request.
	tenant string
}

// NewPostController returns the controller. bootstrap builds it and hands it to
// the routes.
//
// The session store and the CSRF issuer arrive through the constructor rather
// than through the service: a screen is allowed to know about a token and a
// cookie, and a service is not allowed to expose its own dependencies.
func NewPostController(svc *services.PostService, comments *services.CommentService, sessions *security.SessionStore, csrf *security.CSRF, appName, base, tenant string) *PostController {
	return &PostController{svc: svc, comments: comments, sessions: sessions, csrf: csrf, appName: appName, base: base, tenant: tenant}
}

// Compile-time proof of the seven actions httpx.Router.Resource looks for. It
// registers the ones the controller implements and nothing else, so a route that
// exists is a route that answers -- and a renamed method fails the build here
// rather than answering 404 in production.
var (
	_ httpx.Indexer   = (*PostController)(nil)
	_ httpx.Creator   = (*PostController)(nil)
	_ httpx.Storer    = (*PostController)(nil)
	_ httpx.Shower    = (*PostController)(nil)
	_ httpx.Editor    = (*PostController)(nil)
	_ httpx.Updater   = (*PostController)(nil)
	_ httpx.Destroyer = (*PostController)(nil)
)

// postPerPage is how many records the listing asks for when the request
// does not say. The repository has a bound of its own: this one is about the
// screen, that one is about the database.
const postPerPage = 25

// Index renders the listing.
func (c *PostController) Index(ctx *httpx.Context) error {
	// A reader with no session sees the published listing; somebody signed in
	// sees everything, drafts included. Two queries, because they are two
	// questions -- and the guest one cannot reach a draft at all rather than
	// reaching it and discarding it.
	actor, signedIn := c.reader(ctx)

	// The page size is decided here rather than passed through blindly: asking
	// for a known number is what lets the next cursor be offered only when a
	// full page came back.
	limit := postPerPage
	if n, err := strconv.Atoi(ctx.Query("limit")); err == nil && n > 0 {
		limit = n
	}

	var found []models.Post
	var err error
	if signedIn {
		found, err = c.svc.List(ctx.Ctx(), actor, data.Query{
			Limit:  limit,
			Cursor: ctx.Query("cursor"),
			Sort:   ctx.Query("sort"),
		})
	} else {
		found, err = c.svc.Published(ctx.Ctx(), actor, limit)
	}
	if err != nil {
		return c.fail(ctx, err)
	}

	rows := make([]views.PostRow, 0, len(found))
	for _, p := range found {
		rows = append(rows, c.row(ctx, p, 0))
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

	return ctx.View("posts.index", views.PostsIndexData{
		Page:  c.chrome(ctx, signedIn, actor.ID, token, "Posts"),
		Posts: rows,
		// Empty for a guest, and an empty URL draws no button.
		NewURL:     ifSignedIn(signedIn, ctx.URL("posts.create")),
		NextCursor: next,
	})
}

// Show renders one record.
func (c *PostController) Show(ctx *httpx.Context) error {
	// A reader with no session is a guest rather than a redirect. Whether they
	// may read this post is PostPolicy's answer, not this handler's -- the
	// article is public when it is published and refused when it is a draft,
	// and both answers come from the same place every other answer does.
	actor, signedIn := c.reader(ctx)

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

	// A guest sees the article and not the thread. Comments are written by
	// people with accounts and read by them, and CommentPolicy says so -- asking
	// for them here as a guest would be a refusal that costs a query.
	var thread []models.Comment
	if signedIn {
		thread, err = c.comments.ForPost(ctx.Ctx(), actor, found.ID)
		if err != nil {
			return c.fail(ctx, err)
		}
	}

	return ctx.View("posts.show", views.PostsShowData{
		Page:     c.article(ctx, actor, signedIn, token, found),
		Post:     c.row(ctx, found, len(thread)),
		Comments: c.thread(thread),

		// The three addresses a guest does not get. An empty URL draws no
		// control, so the policy reaches the markup as data rather than as a
		// question the view asks -- and a button that is not drawn is one
		// nobody has to check twice on the way in.
		CommentURL: ifSignedIn(signedIn, ctx.URL("posts.comments", found.ID)),
		EditURL:    ifSignedIn(signedIn, ctx.URL("posts.edit", found.ID)),
		DeleteURL:  ifSignedIn(signedIn, ctx.URL("posts.destroy", found.ID)),
	})
}

// Create renders the empty form.
func (c *PostController) Create(ctx *httpx.Context) error {
	if _, err := c.actor(ctx); err != nil {
		return c.signIn(ctx)
	}
	token, err := c.token(ctx)
	if err != nil {
		return err
	}

	return ctx.View("posts.create", views.PostsCreateData{
		Page:   c.chrome(ctx, true, "", token, "New post"),
		Errors: map[string][]string{},
	})
}

// Store takes the submitted form.
func (c *PostController) Store(ctx *httpx.Context) error {
	actor, err := c.actor(ctx)
	if err != nil {
		return c.signIn(ctx)
	}

	in, form, errs := c.input(ctx)
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
	return ctx.Redirect("/posts/" + created.ID)
}

// Edit renders the form filled in.
func (c *PostController) Edit(ctx *httpx.Context) error {
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

	return ctx.View("posts.edit", views.PostsEditData{
		Page:   c.chrome(ctx, true, "", token, "Edit post"),
		Form:   c.form(found),
		Errors: map[string][]string{},
	})
}

// Update writes the submitted form onto the stored record.
func (c *PostController) Update(ctx *httpx.Context) error {
	actor, err := c.actor(ctx)
	if err != nil {
		return c.signIn(ctx)
	}

	in, form, errs := c.input(ctx)
	form.ID = ctx.Param("id")
	if !c.Validated(errs) {
		return c.rejectedEdit(ctx, form, errs)
	}

	updated, err := c.svc.Update(ctx.Ctx(), actor, requests.UpdatePost{
		ID:          ctx.Param("id"),
		Title:       in.Title,
		Slug:        in.Slug,
		Body:        in.Body,
		PublishedAt: in.PublishedAt,
	})
	if err != nil {
		var invalid validation.Errors
		if errors.As(err, &invalid) {
			return c.rejectedEdit(ctx, form, invalid)
		}
		return c.fail(ctx, err)
	}
	return ctx.Redirect("/posts/" + updated.ID)
}

// Destroy removes the record.
func (c *PostController) Destroy(ctx *httpx.Context) error {
	actor, err := c.actor(ctx)
	if err != nil {
		return c.signIn(ctx)
	}
	if err := c.svc.Delete(ctx.Ctx(), actor, ctx.Param("id")); err != nil {
		return c.fail(ctx, err)
	}
	return ctx.Redirect("/posts")
}

// actor is who is acting, from the session and never from the request body.
func (c *PostController) actor(ctx *httpx.Context) (security.Subject, error) {
	return c.sessions.Load(ctx.Ctx(), ctx.Request)
}

// signIn sends an unauthenticated visitor to the sign-in screen. Under HTMX the
// redirect becomes HX-Redirect, so the browser navigates instead of nesting the
// whole page inside a fragment.
func (c *PostController) signIn(ctx *httpx.Context) error {
	return ctx.Redirect("/auth/login")
}

// token issues a CSRF token for the session that is rendering the page.
//
// Every page needs it, including the ones that write nothing: the sign-out form
// and every hx- request read it off the page data. A page rendered without one
// answers 200 and then refuses the next write with 419, which reads like a
// broken session rather than a missing field.
func (c *PostController) token(ctx *httpx.Context) (string, error) {
	return c.csrf.Issue(c.sessions.IDFromRequest(ctx.Request))
}

// row turns the entity into what the markup renders.
//
// Formatting happens here rather than in the view: a view that formats a
// time.Time would need the time package, and what a date looks like on screen is
// a decision about presentation, which is this side of the line.
func (c *PostController) row(ctx *httpx.Context, p models.Post, comments int) views.PostRow {
	// A draft has no publication date, and formatting the zero value prints
	// 0001-01-01 -- a date that looks like data and is the absence of it. The
	// view reads the empty string as "not published", which is what the badge
	// and the meta line branch on.
	published := ""
	if !p.PublishedAt.IsZero() {
		published = p.PublishedAt.Format("2 January 2006")
	}

	return views.PostRow{
		ID:          p.ID,
		Title:       p.Title,
		Slug:        p.Slug,
		Body:        p.Body,
		Excerpt:     excerpt(p.Body),
		PublishedAt: published,
		Created:     p.CreatedAt.Format("2 January 2006"),
		// Built from the route name. "/posts/"+p.ID compiles and keeps
		// compiling after the route moves, and the card links nowhere -- which
		// is exactly what happened: every card on the listing had an empty href
		// until this line existed.
		URL:      ctx.URL("posts.show", p.ID),
		Comments: strconv.Itoa(comments),
	}
}

// article is the chrome of one post, plus what the page says about itself.
//
// The description is the excerpt because that is already the opening of the
// article: a description written separately is one of the two going stale. The
// canonical is absolute, because a relative one is ignored by every crawler that
// reads it -- which is the failure mode where the tag is present and does
// nothing.
func (c *PostController) article(ctx *httpx.Context, actor security.Subject, signedIn bool, token string, p models.Post) view.Page {
	page := c.chrome(ctx, signedIn, actor.ID, token, p.Title)
	page.Description = excerpt(p.Body)
	page.Canonical = c.base + ctx.URL("posts.show", p.ID)
	return page
}

// reader is who is asking: the signed-in subject, or a declared guest.
//
// The tenant of a guest is the application's, from configuration. A visitor
// cannot choose whose rows they read (RULE 14), and it is not suspended because
// nobody signed in.
func (c *PostController) reader(ctx *httpx.Context) (security.Subject, bool) {
	if actor, err := c.sessions.Load(ctx.Ctx(), ctx.Request); err == nil {
		return actor, true
	}
	return security.Guest(c.tenant), false
}

// ifSignedIn is the address, or nothing for a guest.
func ifSignedIn(signedIn bool, url string) string {
	if signedIn {
		return url
	}
	return ""
}

// chrome is the state the layout draws, filled the same way on every screen.
//
// One helper rather than six literals: the navigation is drawn from these
// fields, and a screen that filled four of them was a screen offering "Login" to
// somebody who had just signed in.
func (c *PostController) chrome(ctx *httpx.Context, signedIn bool, name, token, title string) view.Page {
	return view.Page{
		Title:         title,
		AppName:       c.appName,
		Token:         token,
		Authenticated: signedIn,
		UserName:      name,
		HomeURL:       ctx.URL("home"),
		LoginURL:      "/auth/login",
		LogoutURL:     "/auth/logout",
	}
}

// thread turns the stored comments into rows the markup draws.
func (c *PostController) thread(found []models.Comment) []views.CommentRow {
	rows := make([]views.CommentRow, 0, len(found))
	for _, m := range found {
		rows = append(rows, views.CommentRow{
			ID:       m.ID,
			Author:   m.Author,
			Body:     m.Body,
			Created:  m.CreatedAt.Format("2 January 2006"),
			Approved: m.Approved,
		})
	}
	return rows
}

// excerpt is the opening of a body, cut at a word boundary.
//
// It is here rather than in the view because it is a decision about
// presentation, and a view that cuts a string is a view with a rule in it.
func excerpt(body string) string {
	const limit = 160
	if len(body) <= limit {
		return body
	}
	cut := body[:limit]
	if at := strings.LastIndexByte(cut, ' '); at > 0 {
		cut = cut[:at]
	}
	return cut + "…"
}

// form fills the edit form from the stored record.
func (c *PostController) form(p models.Post) views.PostForm {
	return views.PostForm{
		ID:          p.ID,
		Title:       p.Title,
		Slug:        p.Slug,
		Body:        p.Body,
		PublishedAt: p.PublishedAt.Format("2006-01-02T15:04"),
	}
}

// input reads the submitted form.
//
// It returns three things: the typed request the service takes, the form as it
// was typed -- so a rejected submission comes back filled in rather than blank --
// and the errors parsing itself found. A number that is not a number is rejected
// here, naming the field, rather than reaching the service as a silent zero.
func (c *PostController) input(ctx *httpx.Context) (requests.StorePost, views.PostForm, validation.Errors) {
	errs := validation.Errors{}

	in := requests.StorePost{
		Title:       ctx.Input("title"),
		Slug:        ctx.Input("slug"),
		Body:        ctx.Input("body"),
		PublishedAt: c.moment(ctx, "published_at", "2006-01-02T15:04", errs),
	}

	form := views.PostForm{
		Title:       ctx.Input("title"),
		Slug:        ctx.Input("slug"),
		Body:        ctx.Input("body"),
		PublishedAt: ctx.Input("published_at"),
	}

	// arandu:begin custom
	// Anything the form carries that the fields above do not: a value composed
	// of two inputs, a default that depends on the actor.
	// arandu:end custom

	return in, form, errs
}

// rejectedCreate re-renders the creation form with its errors, as the 422
// fragment HTMX swaps back in.
func (c *PostController) rejectedCreate(ctx *httpx.Context, form views.PostForm, errs validation.Errors) error {
	token, err := c.token(ctx)
	if err != nil {
		return err
	}
	return c.Invalid(ctx, "posts.create", views.PostsCreateData{
		Page:   c.chrome(ctx, true, "", token, "New post"),
		Form:   form,
		Errors: errs,
	})
}

// rejectedEdit re-renders the edit form with its errors.
func (c *PostController) rejectedEdit(ctx *httpx.Context, form views.PostForm, errs validation.Errors) error {
	token, err := c.token(ctx)
	if err != nil {
		return err
	}
	return c.Invalid(ctx, "posts.edit", views.PostsEditData{
		Page:   c.chrome(ctx, true, "", token, "Edit post"),
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
func (c *PostController) fail(ctx *httpx.Context, err error) error {
	switch {
	case errors.Is(err, security.ErrForbidden):
		observability.Log(ctx.Ctx()).Warn("authorization denied", "error", err)
		return ctx.Status(http.StatusForbidden)
	case errors.Is(err, models.ErrPostNotFound):
		return ctx.Status(http.StatusNotFound)
	case errors.Is(err, models.ErrPostConflict):
		return ctx.Status(http.StatusConflict)
	case errors.Is(err, models.ErrPostSort):
		return ctx.Status(http.StatusBadRequest)
	default:
		return err
	}
}

// moment reads a date or a timestamp, in the layout the matching HTML input
// submits.
func (c *PostController) moment(ctx *httpx.Context, field, layout string, e validation.Errors) time.Time {
	raw := ctx.Input(field)
	if raw == "" {
		return time.Time{}
	}
	t, err := time.Parse(layout, raw)
	if err != nil {
		e.Add(field, "is not a valid date")
		return time.Time{}
	}
	return t
}

// arandu:begin custom
// Actions beyond the seven go here, and survive regeneration. Register them in
// the custom block of routes/web.go.
// arandu:end custom
