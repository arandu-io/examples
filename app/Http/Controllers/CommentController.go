package controllers

import (
	"errors"
	"net/http"

	"github.com/arandu-io/hesape/auth"
	hhttp "github.com/arandu-io/hesape/http"
	"github.com/arandu-io/hesape/log"

	requests "github.com/arandu-io/examples/app/Http/Requests"
	models "github.com/arandu-io/examples/app/Models"
	services "github.com/arandu-io/examples/app/Services"
)

// CommentController answers the comment box under an article, and nothing else.
//
// It is thin on purpose: read the request, call the service, redirect. There is
// no repository here and there cannot be one -- hhttp.Context carries no
// database handle, so a controller that reached the data layer would be a
// controller that skipped the service, and therefore skipped the policy.
//
// It has one action and not the seven of a resource. A comment is read in the
// thread of the post it answers, which PostController.Show draws, and it is
// moderated in the area AdminController answers. A listing, a page and an edit
// form of their own at /comments would be a second address for the same
// conversation, reachable without the article around it -- and an edit form
// over a comment carries its post and its author as fields somebody can change.
type CommentController struct {
	Controller

	svc *services.CommentService
}

// NewCommentController returns the controller. bootstrap builds it and hands it to
// the routes.
//
// It takes no session store. Its route is behind RequireAuth in routes/web.go,
// which puts who is asking on the request, and guarded reads it there.
func NewCommentController(svc *services.CommentService) *CommentController {
	return &CommentController{svc: svc}
}

// Store takes the comment box under an article.
func (c *CommentController) Store(ctx *hhttp.Context) error {
	actor := guarded(ctx)

	// Bind reads the body and nothing else -- never the query string of a
	// write -- into the fields requests.StoreComment declares with a form tag. A
	// value it cannot convert comes back as validation.Errors naming the field,
	// and so does what the service's rules refuse. Both are returned as they
	// are: the router sends a page back to the form with the messages and what
	// was typed, and answers a client that asked for JSON with a 422.
	var in requests.StoreComment
	if err := ctx.Bind(&in); err != nil {
		return err
	}

	// The post comes from the route, never from the form. A hidden field carrying
	// it is a hidden field somebody edits and would let a comment be attached to
	// any post. Authorship and moderation state are owned by CommentService, so
	// every transport gets the same rule rather than copying this handler.
	in.PostId = ctx.Param("id")

	created, err := c.svc.Create(ctx.Ctx(), actor, in)
	if err != nil {
		return c.fail(ctx, err)
	}
	// Back to the article, which is where the person is. The comment they wrote
	// is not in the thread yet -- it is waiting for review -- so the answer says
	// so rather than leaving them looking for it.
	_ = created
	return ctx.Redirect(ctx.URL("posts.show", in.PostId) + "?said=1")
}

// fail turns a domain error into a status, in one place.
//
// Note what it does not do: it never writes the authorization error into the
// response. Why a policy said no is information about the system, and it belongs
// in the log. Anything unrecognized is returned, and the router turns it into
// the error page in development and a 500 in production.
func (c *CommentController) fail(ctx *hhttp.Context, err error) error {
	switch {
	case errors.Is(err, auth.ErrForbidden):
		log.For(ctx.Ctx()).Warn("authorization denied", "error", err)
		return ctx.Status(http.StatusForbidden)
	case errors.Is(err, models.ErrCommentNotFound):
		return ctx.Status(http.StatusNotFound)
	case errors.Is(err, models.ErrPostNotFound):
		return ctx.Status(http.StatusNotFound)
	case errors.Is(err, models.ErrCommentConflict):
		return ctx.Status(http.StatusConflict)
	default:
		return err
	}
}

// arandu:begin custom
// Actions beyond the one go here, and survive regeneration. Register them in
// the custom block of routes/web.go.
// arandu:end custom
