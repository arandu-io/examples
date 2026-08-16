package controllers

import (
	"github.com/arandu-io/framework/http"
	"github.com/arandu-io/framework/modules/auth"
	"github.com/arandu-io/framework/security"
	"github.com/arandu-io/framework/view"

	authui "github.com/arandu-io/examples/app/Http/Controllers/Auth"
)

// navigation is what the header is drawn from, in one place.
//
// It was a method on one controller and six struct literals on the others, and
// the literals drifted: /categories rendered a brand that linked to href="", a
// "Sign in" button that reloaded the page you were already on, no way to
// register, and the guest half of the bar shown to a signed-in administrator.
// Every one of those is a field somebody forgot, and there is no way to forget
// a field you do not fill in.
//
// It is a value each controller holds rather than something on the base
// Controller: that type carries no dependencies on purpose, and the header needs
// three. Injected, it stays as testable as the controller around it.
type navigation struct {
	// appName is the brand in the corner and the suffix of every title.
	appName string

	// people resolves the signed-in id into a name. The session carries an id,
	// deliberately -- a name in a session is a name that stays wrong after
	// somebody changes it.
	people *auth.Service

	// tenant is whose rows a guest reads, and the tenant the name lookup
	// runs in.
	tenant string
}

// page fills the chrome for one screen.
//
// The whole navigation comes from here, so a screen cannot draw half of it. The
// two links that depend on who is asking are decided here too, from the subject
// the caller already holds: the markup never asks about a role, because a second
// place that decides authorization is the place that gets it wrong.
//
// What the sign-in screens and this blog's screens share comes from
// authui.Chrome, which is the starter kit's own header. Restating those six
// fields here would be a second place that decides what a header is, and the kit
// republishes HomeController without a flag -- so the two would drift on the one
// screen where drift is most visible. What is added below is what the kit cannot
// know about: this application's own areas.
func (n navigation) page(ctx *http.Context, actor security.Subject, signedIn bool, token, title string) view.Page {
	page := authui.Chrome(authui.ChromeProps{
		AppName:       n.appName,
		Title:         title,
		Path:          ctx.Request.URL.Path,
		Token:         token,
		Authenticated: signedIn,
		UserName:      n.displayName(ctx, actor.ID),
	})

	// The named route rather than the literal the kit assumes: this application
	// registers its front page under a name, and a rename has to move the link
	// with it.
	page.HomeURL = ctx.URL("home")

	// The reader's own area, and -- only for somebody the policy would let in --
	// the moderation queue. A non-administrator gets the empty string and no
	// link, rather than a link that answers 403.
	page.PanelURL = ifSignedIn(signedIn, "/dashboard")
	page.AdminURL = ifSignedIn(signedIn && actor.HasRole("admin"), "/admin/")

	return page
}

// displayName turns the id a session carries into something to greet.
//
// Through authui.SignedInName, which is the kit's own, for the reason page goes
// through authui.Chrome: a copy here would be a second answer to "what does the
// header say when the lookup fails". Only one answer keeps the page rendering,
// and it is not the one somebody writes in a hurry.
func (n navigation) displayName(ctx *http.Context, id string) string {
	return authui.SignedInName(ctx.Ctx(), n.people, n.tenant, id)
}

// reader is who is asking: the signed-in subject, or a declared guest.
//
// The tenant of a guest is the application's, from configuration. A visitor
// cannot choose whose rows they read, and it is not suspended because nobody
// signed in.
func (n navigation) reader(ctx *http.Context, sessions *security.SessionStore) (security.Subject, bool) {
	if actor, err := sessions.Load(ctx.Ctx(), ctx.Request); err == nil {
		return actor, true
	}
	return security.Guest(n.tenant), false
}

// ifSignedIn is the address, or nothing.
func ifSignedIn(show bool, url string) string {
	if show {
		return url
	}
	return ""
}
