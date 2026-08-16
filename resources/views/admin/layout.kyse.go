//go:build kyse

package admin

import "github.com/arandu-io/framework/view"

@go
// AdminLayout is what this layout asks of the screens inside it.
//
// It is view.Layout plus the sidebar, and declaring it here is the case the
// design leaves open: a layout renders with the interface it declares, and only
// falls back to view.Layout when it declares none. The public layout declares
// none, because the chrome the framework publishes is exactly what it draws.
//
// This one draws more, so it asks for more -- and asking through an interface
// rather than through Chrome directly is what would let a second admin screen
// with different data still fit.
type AdminLayout interface {
	view.Layout

	// On reports whether a sidebar item is the current one, and ItemClass
	// styles it. Two methods rather than one because the marked item is also
	// the one that gets aria-current, and a class string cannot carry that.
	On(item string) bool
	ItemClass(item string) string

	DashboardURL() string
	PostsURL() string
	CommentsURL() string
	SocketsURL() string
}

// Chrome is what every screen of the moderation area hands this layout.
//
// It is the area's own frame, not the public one: a sidebar instead of a
// navigation bar, and no theme picker, because somebody moderating comments is
// working rather than reading.
//
// Current is which item of the sidebar is the one you are on. It arrives as
// data rather than being worked out here by comparing paths -- the router knows
// it, and a template guessing at it is a template that gets it wrong the first
// time a path changes.
type Chrome struct {
	view.Page

	// Current is the sidebar item to mark, by its label.
	Current string

	// Where the sidebar points. They are fields, and the interface asks for
	// methods, so the four below are what bridges them -- a field and a method
	// cannot share a name in Go.
	Dashboard string
	Posts     string
	Comments  string
	// Sockets is the operator's screen: what the socket server is holding right
	// now. It is in this sidebar because it answers only to the same signed-in
	// administrator every other item here does, and nowhere else because a
	// number about the process is not something a reader is shown.
	Sockets string
}

// DashboardURL, PostsURL, CommentsURL and SocketsURL are where the sidebar
// points.
func (c Chrome) DashboardURL() string { return c.Dashboard }
func (c Chrome) PostsURL() string     { return c.Posts }
func (c Chrome) CommentsURL() string  { return c.Comments }
func (c Chrome) SocketsURL() string   { return c.Sockets }

// Compile-time proof that this frame fits the layout above.
var _ AdminLayout = Chrome{}

// On reports whether an item is the current one.
func (c Chrome) On(item string) bool { return c.Current == item }

// ItemClass is the class of one sidebar item, marked or not.
func (c Chrome) ItemClass(item string) string {
	if c.On(item) {
		return "bg-accent text-accent-foreground rounded-md px-3 py-2 text-sm font-medium"
	}
	return "text-muted-foreground hover:bg-accent hover:text-accent-foreground rounded-md px-3 py-2 text-sm"
}
@endgo

<!doctype html>
<html lang="en" class="h-full">
<head>
	<meta charset="utf-8">
	<meta name="viewport" content="width=device-width, initial-scale=1">
	<title>{{ .PageTitle() }}</title>
	<link rel="icon" href="/favicon.ico" sizes="any">
	<link rel="icon" href="/favicon.png" type="image/png">

	{{-- The moderation area is not for search engines: it answers only to a
	     signed-in administrator, so a crawler that reached it would index a
	     redirect to the sign-in screen. --}}
	<meta name="robots" content="noindex, nofollow">

	<link rel="stylesheet" href="{{ view.URL("app.css") }}">
	<script src="{{ view.URL("htmx.min.js") }}" defer></script>
	<script src="{{ view.URL("alpine.min.js") }}" defer></script>
	<script src="{{ view.URL("basecoat.bundle.js") }}" defer></script>
	<script src="{{ view.URL("theme.js") }}"></script>
</head>
<body hx-boost="true" hx-headers='{"X-CSRF-Token": "{{ .CSRFToken() }}"}' class="bg-background text-foreground min-h-full antialiased">
	<div class="flex min-h-full">
		<aside class="bg-sidebar text-sidebar-foreground hidden w-64 shrink-0 border-r p-4 sm:block">
			<a class="block px-3 py-2 text-sm font-semibold tracking-tight" href="{{ .HomeLink() }}">{{ .BrandName() }}</a>

			<nav class="mt-6 flex flex-col gap-1">
				<a class="{{ .ItemClass("Dashboard") }}" href="{{ .DashboardURL() }}">Dashboard</a>
				<a class="{{ .ItemClass("Comments") }}" href="{{ .CommentsURL() }}">Comments</a>
				<a class="{{ .ItemClass("Posts") }}" href="{{ .PostsURL() }}">Posts</a>
				<a class="{{ .ItemClass("Sockets") }}" href="{{ .SocketsURL() }}">Sockets</a>
			</nav>

			<form class="mt-6" method="post" action="{{ .LogoutLink() }}">
				@csrf
				<button type="submit" class="btn w-full" data-variant="outline" data-size="sm">Sign out</button>
			</form>
		</aside>

		<main class="min-w-0 flex-1 px-6 py-10">
			<div class="mx-auto w-full max-w-3xl">
				@yield('content')
			</div>
		</main>
	</div>

	<div id="toaster" class="toaster" aria-live="polite"></div>
</body>
</html>
