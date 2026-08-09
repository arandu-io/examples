package controllers

import (
	"github.com/arandu-io/framework/httpx"
	"github.com/arandu-io/framework/modules/auth"
	"github.com/arandu-io/framework/security"
	"github.com/arandu-io/framework/view"
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

	// tenant is whose rows a guest reads (RULE 14), and the tenant the name
	// lookup runs in.
	tenant string
}

// page fills the chrome for one screen.
//
// The whole navigation comes from here, so a screen cannot draw half of it. The
// two links that depend on who is asking are decided here too, from the subject
// the caller already holds: the markup never asks about a role, because a second
// place that decides authorization is the place that gets it wrong.
func (n navigation) page(ctx *httpx.Context, actor security.Subject, signedIn bool, token, title string) view.Page {
	return view.Page{
		Title:         title,
		AppName:       n.appName,
		Token:         token,
		Authenticated: signedIn,
		UserName:      n.displayName(ctx, actor.ID),
		Path:          ctx.Request.URL.Path,
		HomeURL:       ctx.URL("home"),
		LoginURL:      "/auth/login",
		LogoutURL:     "/auth/logout",
		// Registration is open on this blog, so the layout draws the button. An
		// application that closes it leaves this empty, and the header stops
		// offering an address that refuses everybody.
		RegisterURL: "/auth/register",

		// The reader's own area, and -- only for somebody the policy would let
		// in -- the moderation queue. A non-administrator gets the empty string
		// and no link, rather than a link that answers 403.
		PanelURL: ifSignedIn(signedIn, "/dashboard"),
		AdminURL: ifSignedIn(signedIn && actor.HasRole("admin"), "/admin/"),
	}
}

// displayName turns the id a session carries into something to greet.
//
// One lookup per page for the person who is signed in, by primary key. It falls
// back to the id rather than failing: a header is not worth a 500.
func (n navigation) displayName(ctx *httpx.Context, id string) string {
	if id == "" || n.people == nil {
		return ""
	}
	names, err := n.people.Names(ctx.Ctx(), n.tenant, []string{id})
	if err != nil || names[id] == "" {
		return id
	}
	return names[id]
}

// reader is who is asking: the signed-in subject, or a declared guest.
//
// The tenant of a guest is the application's, from configuration. A visitor
// cannot choose whose rows they read (RULE 14), and it is not suspended because
// nobody signed in.
func (n navigation) reader(ctx *httpx.Context, sessions *security.SessionStore) (security.Subject, bool) {
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
