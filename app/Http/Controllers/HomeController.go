package controllers

import (
	"github.com/arandu-io/framework/httpx"
	"github.com/arandu-io/framework/modules/auth"
	"github.com/arandu-io/framework/security"

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

	// sessions and csrf are what the chrome is drawn from: who is signed in,
	// and the token every write of this session carries. They arrive through
	// the constructor for the same reason appName does, and they are the same
	// two every controller `aru make:module` writes takes -- a screen is
	// allowed to know about a token and a cookie.
	sessions *security.SessionStore
	csrf     *security.CSRF

	// nav draws the header, the same way on every screen. See chrome.go.
	nav navigation
}

// NewHomeController returns the controller. bootstrap/app.go builds it and hands
// it to the routes.
func NewHomeController(appName string, sessions *security.SessionStore, csrf *security.CSRF, people *auth.Service, tenant string) *HomeController {
	return &HomeController{
		appName: appName, sessions: sessions, csrf: csrf,
		nav: navigation{appName: appName, people: people, tenant: tenant},
	}
}

// Compile-time proof that this controller answers GET / the way Resource and the
// route table expect. It costs nothing and catches a renamed method.
var _ httpx.Indexer = (*HomeController)(nil)

// Index renders the landing page.
//
// The session and the token are read above the custom block, and deliberately:
// they are what the layout draws its navigation and its hx-headers from, so a
// regeneration that carried over an edited block would otherwise carry over a
// page that greets a signed-in visitor with a sign-in link.
func (c *HomeController) Index(ctx *httpx.Context) error {
	// Who is signed in, from the session cookie and never from the request. An
	// error here is the anonymous case -- no cookie, a forged one, or a session
	// that expired -- and the guest half of the navigation is what gets drawn.
	subject, err := c.sessions.Load(ctx.Ctx(), ctx.Request)
	signedIn := err == nil

	// The token reaches the markup twice: the hidden field of the sign-out form
	// and the hx-headers attribute on <body>. A page rendered without one
	// answers 200 and then refuses the next write with 419, which reads like a
	// broken session rather than a missing field.
	token, err := c.csrf.Issue(c.sessions.IDFromRequest(ctx.Request))
	if err != nil {
		return err
	}

	// arandu:begin custom
	page := c.nav.page(ctx, subject, signedIn, token, c.appName)
	page.HomeURL = "/"

	return ctx.View("home", authui.AuthPage{
		// view.Page is the chrome the layout draws, embedded rather than
		// repeated, and it is filled by one helper so that a screen cannot draw
		// half of it. This one used to build the literal itself: it greeted the
		// signed-in person with the UUID out of their session, offered no way to
		// create an account, and hid the password reset -- all three describing
		// a kit that ships only the sign-in handler, which stopped being true
		// when this application wired the whole of it.
		Page: page,

		// The reset is wired: PasswordController answers /auth/password, mails
		// a signed link and writes the new password. The link is drawn because
		// the handler exists, which is the only reason a link is ever drawn.
		HasPasswordReset: true,
	})
	// arandu:end custom
}
