package controllers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	fhttp "github.com/arandu-io/framework/http"
	"github.com/arandu-io/hesape/auth"
	"github.com/arandu-io/hesape/database"
	hhttp "github.com/arandu-io/hesape/http"
	"github.com/arandu-io/hesape/log"
	"github.com/arandu-io/hesape/view"

	requests "github.com/arandu-io/examples/app/Http/Requests"
	models "github.com/arandu-io/examples/app/Models"
	services "github.com/arandu-io/examples/app/Services"
	views "github.com/arandu-io/examples/storage/framework/views/posts"
)

// PostController answers the seven routes of the posts resource.
//
// It is thin on purpose: read the request, call the service, render. There is no
// repository here and there cannot be one -- hhttp.Context carries no database
// handle, so a controller that reached the data layer would be a controller that
// skipped the service, and therefore skipped the policy.
type PostController struct {
	Controller

	svc      *services.PostService
	comments *services.CommentService
	// categories is what fills the section bar and resolves /c/{slug}. The same
	// service the administrator's CRUD uses, because there is one policy over
	// categories and a second door to them would be a second place to forget it.
	categories *services.CategoryService
	// people resolves the author of a comment to a name through the application-
	// owned user service. Keeping that projection behind one interface avoids a
	// second controller-owned query over the users schema.
	people UserNames

	// nav draws the header, the same way on every screen. See chrome.go.
	nav navigation

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
// It takes no session store. Who is asking is put on the request by the
// route's guard -- RequireAuth on a write, LoadSubject on a public read, both in
// routes/web.go -- and read here with ctx.User(), so this controller is not a
// second place that loads a session and decides what an expired one answers.
// Nor does it issue the CSRF token: csrfToken reads the one CSRFProtect put on
// the request.
func NewPostController(svc *services.PostService, comments *services.CommentService, categories *services.CategoryService, people UserNames, appName, base, tenant string) *PostController {
	return &PostController{
		svc: svc, comments: comments, categories: categories, people: people,
		appName: appName, base: base, tenant: tenant,
		nav: navigation{appName: appName, people: people, tenant: tenant},
	}
}

// Compile-time proof of the seven actions fhttp.Router.Resource looks for. It
// registers the ones the controller implements and nothing else, so a route that
// exists is a route that answers -- and a renamed method fails the build here
// rather than answering 404 in production.
var (
	_ fhttp.Indexer   = (*PostController)(nil)
	_ fhttp.Creator   = (*PostController)(nil)
	_ fhttp.Storer    = (*PostController)(nil)
	_ fhttp.Shower    = (*PostController)(nil)
	_ fhttp.Editor    = (*PostController)(nil)
	_ fhttp.Updater   = (*PostController)(nil)
	_ fhttp.Destroyer = (*PostController)(nil)
)

// postPerPage is how many records the listing asks for when the request
// does not say. The repository has a bound of its own: this one is about the
// screen, that one is about the database.
const postPerPage = 25

// Index renders the listing.
func (c *PostController) Index(ctx *hhttp.Context) error {
	// A reader with no session sees the published listing; somebody signed in
	// sees everything, drafts included. Two queries, because they are two
	// questions -- and the guest one cannot reach a draft at all rather than
	// reaching it and discarding it.
	actor, signedIn := c.nav.reader(ctx)

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
		found, err = c.svc.List(ctx.Ctx(), actor, database.Query{
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

	// The sections, and the name of the one each post is in. One query for the
	// whole page rather than one per row: eight cards in eight sections would
	// otherwise be eight lookups, which is the N+1 an ORM gets blamed for and
	// that hand-written SQL reproduces just as easily.
	sections, byID := c.sections(ctx, actor, "")

	rows := make([]views.PostRow, 0, len(found))
	for _, p := range found {
		rows = append(rows, c.row(ctx, p, 0, byID))
	}

	// The listing writes nothing, but the layout around it does: the sign-out
	// form and every hx- request read the token off the page data. A listing
	// rendered without one answers 200 and then refuses the next write with
	// 419, which reads like a broken session.
	token := csrfToken(ctx)

	// Keyset pagination picks up after the last id of the page. A partial page
	// is the last page, and offering a cursor there would be a link to nothing.
	next := ""
	if len(rows) == limit {
		next = rows[len(rows)-1].ID
	}

	return ctx.View("posts.index", views.PostsIndexData{
		Page:  c.nav.page(ctx, actor, signedIn, token, "Posts"),
		Posts: rows,
		// Empty for a guest, and an empty URL draws no button.
		NewURL:     ifSignedIn(signedIn, ctx.URL("posts.create")),
		NextCursor: next,
		Sections:   sections,
		Heading:    "Everything we have written",
		Standfirst: "Notes on building with Arandu: the decisions, what they cost, and what they bought.",
	})
}

// Section is the public listing narrowed to one category.
//
// It reuses posts.index rather than having a view of its own. The two pages
// differ by a heading and a filter, and a second template would be a second
// place to fix the card the next time a card changes.
func (c *PostController) Section(ctx *hhttp.Context) error {
	actor, signedIn := c.nav.reader(ctx)

	category, err := c.categories.BySlug(ctx.Ctx(), actor, ctx.Param("slug"))
	if err != nil {
		// A slug nobody recognises is a 404 and not a 500. It arrives from a
		// URL bar and from every crawler that ever indexed a section that has
		// since been renamed.
		return c.fail(ctx, err)
	}

	found, err := c.svc.PublishedInCategory(ctx.Ctx(), actor, category.ID, postPerPage)
	if err != nil {
		return c.fail(ctx, err)
	}

	sections, byID := c.sections(ctx, actor, category.ID)

	rows := make([]views.PostRow, 0, len(found))
	for _, p := range found {
		rows = append(rows, c.row(ctx, p, 0, byID))
	}

	token := csrfToken(ctx)

	page := c.nav.page(ctx, actor, signedIn, token, category.Name)
	page.Description = category.Description
	// The canonical is this section's own address. Without it the section pages
	// and the front page look like the same page with the same posts to a
	// crawler, and one of them is dropped -- usually not the one you wanted.
	page.Canonical = c.base + ctx.URL("categories.section", category.Slug)

	return ctx.View("posts.index", views.PostsIndexData{
		Page:       page,
		Posts:      rows,
		NewURL:     ifSignedIn(signedIn, ctx.URL("posts.create")),
		Sections:   sections,
		Heading:    category.Name,
		Standfirst: category.Description,
	})
}

// sections builds the navigation bar and the id-to-name map the cards read.
//
// A failure is not fatal and does not propagate: the section bar is navigation,
// and a page that refuses to render because the navigation could not be built is
// a page that disappears over a nicety. It is logged, and the article is served.
func (c *PostController) sections(ctx *hhttp.Context, actor auth.Subject, current string) ([]views.SectionLink, map[string]models.Category) {
	all, err := c.categories.All(ctx.Ctx(), actor)
	if err != nil {
		log.For(ctx.Ctx()).Warn("the section bar could not be built", "error", err)
		return nil, nil
	}

	counts, err := c.svc.CountByCategory(ctx.Ctx(), actor)
	if err != nil {
		log.For(ctx.Ctx()).Warn("the section counts could not be read", "error", err)
	}

	byID := make(map[string]models.Category, len(all))
	links := make([]views.SectionLink, 0, len(all))
	for _, ca := range all {
		byID[ca.ID] = ca
		// A section with nothing published in it is not shown. It exists, an
		// administrator can see it, and a reader following a link to an empty
		// page is a reader who thinks the site is broken.
		if counts[ca.ID] == 0 {
			continue
		}
		links = append(links, views.SectionLink{
			Name:    ca.Name,
			URL:     ctx.URL("categories.section", ca.Slug),
			Count:   strconv.Itoa(counts[ca.ID]),
			Current: ca.ID == current,
		})
	}
	return links, byID
}

// Show renders one record.
func (c *PostController) Show(ctx *hhttp.Context) error {
	// A reader with no session is a guest rather than a redirect. Whether they
	// may read this post is PostPolicy's answer, not this handler's -- the
	// article is public when it is published and refused when it is a draft,
	// and both answers come from the same place every other answer does.
	actor, signedIn := c.nav.reader(ctx)

	found, err := c.svc.Get(ctx.Ctx(), actor, ctx.Param("id"))
	if err != nil {
		return c.fail(ctx, err)
	}

	// The token is for the delete button, which sends it as a header: an
	// hx-delete carries no form body, so the hidden field a form uses would
	// never arrive and the request would be refused with 419.
	token := csrfToken(ctx)

	// The thread, and everybody reads it -- a blog where you have to sign in to
	// see what people said is not a blog. What is NOT public is a comment
	// waiting for review, and that is the query's answer rather than this
	// handler's: PublicForPost returns what is approved, plus this reader's own.
	thread, err := c.comments.PublicForPost(ctx.Ctx(), actor, found.ID)
	if err != nil {
		return c.fail(ctx, err)
	}

	// One more read on the counter, and it is deliberately after everything that
	// could have failed: a page that did not render is not a page anybody read.
	// It never fails the request -- see PostService.Read.
	_, byID := c.sections(ctx, actor, found.CategoryID)
	c.svc.Read(ctx.Ctx(), actor, found)

	return ctx.View("posts.show", views.PostsShowData{
		Page:     c.article(ctx, actor, signedIn, token, found),
		Post:     c.row(ctx, found, len(thread), byID),
		Comments: c.thread(ctx, actor, thread),

		// The three addresses a guest does not get. An empty URL draws no
		// control, so the policy reaches the markup as data rather than as a
		// question the view asks -- and a button that is not drawn is one
		// nobody has to check twice on the way in.
		// Signed in AND verified. The policy refuses an unverified account, and
		// drawing a form that is going to be refused is the worst of the three
		// answers available -- worse than no form, and worse than a sentence
		// saying why.
		CommentURL: ifSignedIn(signedIn && actor.Verified, ctx.URL("posts.comments", found.ID)),
		Unverified: signedIn && !actor.Verified,
		ResendURL:  "/auth/verify/resend",
		EditURL:    ifSignedIn(signedIn, ctx.URL("posts.edit", found.ID)),
		DeleteURL:  ifSignedIn(signedIn, ctx.URL("posts.destroy", found.ID)),
	})
}

// Create renders the empty form.
func (c *PostController) Create(ctx *hhttp.Context) error {
	// The subject, not just the fact that there is one. The header greets by
	// name and decides whether to offer the moderation queue, and both were
	// drawn from an empty id here -- so the author writing a post got the header
	// of a stranger. See guarded for where it comes from.
	actor := guarded(ctx)
	token := csrfToken(ctx)

	return ctx.View("posts.create", views.PostsCreateData{
		Page: c.nav.page(ctx, actor, true, token, "New post"),
	})
}

// Store takes the submitted form.
func (c *PostController) Store(ctx *hhttp.Context) error {
	actor := guarded(ctx)

	// Bind reads the body and nothing else -- never the query string of a
	// write -- into the fields requests.StorePost declares with a form tag. A
	// value it cannot convert comes back as validation.Errors naming the field,
	// so a date that is not a date is refused rather than reaching the service
	// as the zero time, which this application reads as a draft. What the
	// service's rules refuse comes back the same way. Both are returned as they
	// are: the router sends a page back to the form with the messages and what
	// was typed, and answers a client that asked for JSON with a 422.
	var in requests.StorePost
	if err := ctx.Bind(&in); err != nil {
		return err
	}

	created, err := c.svc.Create(ctx.Ctx(), actor, in)
	if err != nil {
		return c.fail(ctx, err)
	}
	return ctx.RedirectRoute("posts.show", created.ID)
}

// Edit renders the form filled in.
func (c *PostController) Edit(ctx *hhttp.Context) error {
	actor := guarded(ctx)

	found, err := c.svc.Get(ctx.Ctx(), actor, ctx.Param("id"))
	if err != nil {
		return c.fail(ctx, err)
	}
	token := csrfToken(ctx)

	return ctx.View("posts.edit", views.PostsEditData{
		Page: c.nav.page(ctx, actor, true, token, "Edit post"),
		Form: c.form(found),
	})
}

// Update writes the submitted form onto the stored record.
func (c *PostController) Update(ctx *hhttp.Context) error {
	actor := guarded(ctx)

	var in requests.StorePost
	if err := ctx.Bind(&in); err != nil {
		return err
	}

	updated, err := c.svc.Update(ctx.Ctx(), actor, requests.UpdatePost{
		ID:          ctx.Param("id"),
		Title:       in.Title,
		Slug:        in.Slug,
		Body:        in.Body,
		PublishedAt: in.PublishedAt,
	})
	if err != nil {
		return c.fail(ctx, err)
	}
	return ctx.RedirectRoute("posts.show", updated.ID)
}

// Destroy removes the record.
func (c *PostController) Destroy(ctx *hhttp.Context) error {
	actor := guarded(ctx)
	if err := c.svc.Delete(ctx.Ctx(), actor, ctx.Param("id")); err != nil {
		return c.fail(ctx, err)
	}
	return ctx.RedirectRoute("posts.index")
}

// row turns the entity into what the markup renders.
//
// Formatting happens here rather than in the view: a view that formats a
// time.Time would need the time package, and what a date looks like on screen is
// a decision about presentation, which is this side of the line.
func (c *PostController) row(ctx *hhttp.Context, p models.Post, comments int, sections map[string]models.Category) views.PostRow {
	// A draft has no publication date, and formatting the zero value prints
	// 0001-01-01 -- a date that looks like data and is the absence of it. The
	// view reads the empty string as "not published", which is what the badge
	// and the meta line branch on.
	published := ""
	if !p.PublishedAt.IsZero() {
		published = p.PublishedAt.Format("2 January 2006")
	}

	// The section, when the post is in one and the map has it. A post filed
	// under a category that was deleted keeps its id and shows no chip, which is
	// the honest answer -- inventing a name for a row that is gone would be
	// worse than saying nothing.
	section, sectionURL := "", ""
	if ca, ok := sections[p.CategoryID]; ok {
		section = ca.Name
		sectionURL = ctx.URL("categories.section", ca.Slug)
	}

	// Nothing rather than "0 reads". A count of zero under a post published a
	// minute ago says something worse than saying nothing.
	reads := ""
	if p.Views > 0 {
		reads = strconv.Itoa(p.Views)
	}

	return views.PostRow{
		ID:          p.ID,
		Title:       p.Title,
		Slug:        p.Slug,
		Body:        p.Body,
		Excerpt:     excerpt(p.Body),
		PublishedAt: published,
		Created:     p.CreatedAt.Format("2 January 2006"),
		Category:    section,
		CategoryURL: sectionURL,
		Views:       reads,
		// Built from the route name. "/posts/"+p.ID compiles and keeps
		// compiling after the route moves, and every card on the listing then
		// links nowhere.
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
func (c *PostController) article(ctx *hhttp.Context, actor auth.Subject, signedIn bool, token string, p models.Post) view.Page {
	page := c.nav.page(ctx, actor, signedIn, token, p.Title)
	page.Description = excerpt(p.Body)
	page.Canonical = c.base + ctx.URL("posts.show", p.ID)
	return page
}

// thread turns the stored comments into rows the markup draws.
func (c *PostController) thread(ctx *hhttp.Context, reader auth.Subject, found []models.Comment) []views.CommentRow {
	// The author column holds a subject id, and a thread signed with UUIDs is a
	// thread that looks broken. The names are resolved in ONE query for the
	// whole page -- twenty comments would otherwise be twenty lookups, on the
	// page most likely to have twenty rows.
	ids := make([]string, 0, len(found))
	for _, m := range found {
		ids = append(ids, m.Author)
	}
	// PublicNames authorizes against the reader, and the reader is whoever is
	// looking at the page. The tenant on the subject is what scopes the lookup,
	// which is why it is the controller's and not a parameter.
	names, err := c.people.PublicNames(ctx.Ctx(), reader, ids)
	if err != nil {
		// Not fatal. An article is worth rendering with a thread that names
		// people badly; it is not worth failing over one.
		log.For(ctx.Ctx()).Warn("the comment authors could not be named", "error", err)
	}

	rows := make([]views.CommentRow, 0, len(found))
	for _, m := range found {
		author := names[m.Author]
		if author == "" {
			// An account that was deleted. Saying so is better than printing an
			// id nobody can look up, and better than printing nothing next to a
			// comment that is still there.
			author = "a former reader"
		}
		rows = append(rows, views.CommentRow{
			ID:       m.ID,
			Author:   author,
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

// fail turns a domain error into a status, in one place.
//
// Note what it does not do: it never writes the authorization error into the
// response. Why a policy said no is information about the system, and it belongs
// in the log. Anything unrecognized is returned, and the router turns it into
// the error page in development and a 500 in production.
func (c *PostController) fail(ctx *hhttp.Context, err error) error {
	switch {
	case errors.Is(err, auth.ErrForbidden):
		log.For(ctx.Ctx()).Warn("authorization denied", "error", err)
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

// arandu:begin custom
// Actions beyond the seven go here, and survive regeneration. Register them in
// the custom block of routes/web.go.
// arandu:end custom
