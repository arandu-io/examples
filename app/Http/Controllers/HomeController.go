package controllers

import (
	fhttp "github.com/arandu-io/framework/http"
	hhttp "github.com/arandu-io/hesape/http"

	authui "github.com/arandu-io/examples/app/Http/Controllers/Auth"
)

// HomeController answers the landing page.
//
// It renders with authui.AuthPage, the struct the auth controllers own. Every
// screen of the kit names that one type, in a single line, so a field this page
// does not use stays at its zero value rather than being a struct of its own.
type HomeController struct {
	Controller

	// appName is what the page is titled. It arrives through the constructor
	// rather than through a global read: a controller that reads the
	// environment is a controller no test can pin.
	appName string

	// people and tenant are how the id in a session becomes a name to greet.
	// A session carries an id and not a name on purpose -- a name kept in one
	// stays wrong after somebody changes theirs -- so the header costs one
	// lookup by primary key, and the page greeted people with a UUID until it
	// had somewhere to make it.
	//
	// The tenant is whose rows are read. It comes from the configuration,
	// through bootstrap/app.go, and never from the request.
	people authui.UserNames
	tenant string
}

// NewHomeController returns the controller. bootstrap/app.go builds it and hands
// it to the routes.
//
// There is no session store and no CSRF issuer here: the route puts who is
// signed in on the request, and the middleware that protects forms puts the
// token there, so the page reads both off the request it is answering.
func NewHomeController(appName string, people authui.UserNames, tenant string) *HomeController {
	return &HomeController{appName: appName, people: people, tenant: tenant}
}

// Compile-time proof that this controller answers GET / the way Resource and the
// route table expect. It costs nothing and catches a renamed method.
var _ fhttp.Indexer = (*HomeController)(nil)

// Index renders the landing page.
//
// The subject and the token are read above the custom block, and deliberately:
// they are what the layout draws its navigation and its hx-headers from, so a
// regeneration that carried over an edited block would otherwise carry over a
// page that greets a signed-in visitor with a sign-in link.
//
// The route has to mount this behind a middleware that puts the subject on the
// request: here it is RequireAuth on /dashboard, in routes/web.go, and on a
// public route it would be middleware.LoadSubject. Without one nothing puts a
// subject on the request, and every visitor is drawn the guest half.
func (c *HomeController) Index(ctx *hhttp.Context) error {
	// Who is signed in, put on the request by the route's guard from the
	// session cookie and never from the request body. No subject is the
	// anonymous case -- no cookie, a forged one, or a session that expired --
	// and the guest half of the navigation is what gets drawn.
	subject, signedIn := ctx.User()

	// The token reaches the markup twice: the hidden field of the sign-out form
	// and the hx-headers attribute on <body>. A page rendered without one
	// answers 200 and then refuses the next write with 419, which reads like a
	// broken session rather than a missing field. CSRFProtect issued it for
	// this request, bound to the session or, for a visitor with none, to their
	// guest cookie, and put it on the request context.
	token, _ := hhttp.CSRFTokenFrom(ctx.Ctx())

	// arandu:begin custom
	// The header, from the one helper this application draws every header with.
	// navigation adds what the kit cannot know: the reader's own area and, only
	// for somebody the policy would let in, the moderation queue. It resolves
	// the name through the same application-owned service the kit uses, so this
	// page and the other screens greet the same person the same way.
	page := navigation{appName: c.appName, people: c.people, tenant: c.tenant}.
		page(ctx, subject, signedIn, token, c.appName)

	return ctx.View("home", authui.AuthPage{
		Page: page,

		// The reset is wired: PasswordController answers /auth/password, mails
		// a single-use code and writes the new password. The link is drawn because
		// the handler exists, which is the only reason a link is ever drawn.
		HasPasswordReset: true,
	})
	// arandu:end custom
}
