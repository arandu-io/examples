//go:build kyse

package layouts

import (
	"github.com/arandu-io/kyse/components"
	"github.com/arandu-io/kyse/fonts"
	"github.com/arandu-io/kyse/icons"
)

<!doctype html>
<html lang="en">
<head>
	<meta charset="utf-8">
	<meta name="viewport" content="width=device-width, initial-scale=1">

	{{-- What a rejected form means.
	     htmx swaps a response only when its table of response handling says to,
	     and the default in the copy this framework embeds ends with
	     {"code":"[45]..","swap":false} -- so a 422 is fetched, is correct, and is
	     thrown away. The person sees the form they submitted, unchanged, with no
	     message on it, and concludes the button does nothing. That is exactly
	     what happened: a password one character short answered 422 with the
	     reason in the body, and the screen said nothing at all.
	     422 comes before the catch-all because htmx takes the first entry that
	     matches. It lives here, once: the layout is what decides what a fragment
	     answer means, and a per-page opt-in would be a second way to answer a
	     rejected form (RULE 9). A meta tag is not a script, so it costs nothing
	     against script-src 'self'. See framework/httpx/context.go. --}}
	<meta name="htmx-config" content='{"responseHandling":[{"code":"204","swap":false},{"code":"422","swap":true},{"code":"[23]..","swap":true},{"code":"[45]..","swap":false,"error":true}]}'>
	<title>{{ .PageTitle() }}</title>
	<link rel="icon" href="/favicon.ico" sizes="any">
	<link rel="icon" href="/favicon.png" type="image/png">

	{{-- What a page says about itself, written only when the page filled it in.
	     An empty description is worse than none: a search engine that finds one
	     stops looking for a better sentence in the body. --}}
	@if(.PageDescription() != "")
		<meta name="description" content="{{ .PageDescription() }}">
		<meta property="og:description" content="{{ .PageDescription() }}">
	@endif
	@if(.CanonicalURL() != "")
		<link rel="canonical" href="{{ .CanonicalURL() }}">
		<meta property="og:url" content="{{ .CanonicalURL() }}">
	@endif
	<meta property="og:title" content="{{ .PageTitle() }}">
	<meta property="og:site_name" content="{{ .BrandName() }}">
	<meta property="og:type" content="website">
	<meta name="twitter:card" content="summary_large_image">
	{{-- The share image. One for the whole site rather than one per post: a
	     per-post image is a rendering pipeline, and this is a blog. --}}
	<meta property="og:image" content="/og-cover.svg">
	<meta name="theme-color" content="#2E211A">

	{{-- The vendored faces, fetched with the page rather than two round trips
	     deep. One line for every face `aru font:add` installed, so swapping the
	     family is that command running and not this file changing. --}}
	{!! fonts.Preloads() !!}

	<link rel="stylesheet" href="{{ view.URL("app.css") }}">
	<script src="{{ view.URL("htmx.min.js") }}" defer></script>
	<script src="{{ view.URL("alpine.min.js") }}" defer></script>
	<script src="{{ view.URL("basecoat.bundle.js") }}" defer></script>
	<script src="{{ view.URL("theme.js") }}"></script>
</head>
<body hx-boost="true" hx-headers='{"X-CSRF-Token": "{{ .CSRFToken() }}"}' class="bg-background text-foreground antialiased">
	{{-- min-h-dvh, not min-h-full.
	     A percentage min-height resolves against the parent's HEIGHT, and body
	     had min-height rather than height -- so its computed height was auto,
	     the percentage resolved against nothing, and the footer stopped
	     wherever the content did. On a short page it floated in the middle of
	     the viewport.

	     dvh is the dynamic viewport height: it follows the mobile browser's URL
	     bar showing and hiding, which svh and lvh each get wrong in one of the
	     two states. --}}
	<div class="flex min-h-dvh flex-col">
		{{-- The masthead. The mark is an <img> and not an inline SVG: it is a
		     drawing with forty colours in it, and the version of this that
		     pastes it into every page would put twelve kilobytes on the wire
		     before any content, on every request, uncacheable.

		     width and height are on the tag so the row does not jump when the
		     image arrives -- a masthead that moves after paint is the cheapest
		     layout shift there is to avoid and the one most often left in. --}}
		<header class="border-b">
			<nav class="mx-auto flex h-16 max-w-5xl items-center justify-between gap-4 px-6">
				<a href="{{ .HomeLink() }}" class="flex items-center gap-2.5">
					<img src="/logo.png" alt="" width="32" height="32" class="brand-mark" loading="eager" decoding="async">
					<span class="brand-word">{{ .BrandName() }}</span>
				</a>
				<div class="flex items-center gap-2 text-sm">
					{!! components.ThemeToggle() !!}
					{{-- The icons here name the action rather than decorate it, which
					     is the only reason any of them is in this file. A door you
					     go in and a door you come out of look alike in words and do
					     not look alike in a glyph. --}}
					{{-- The page you are on is not offered as a link.
					     A header with "Sign in" on the sign-in page links to what
					     you are reading, and the one control that would help --
					     the way across to registering -- is the one it hides.
					     So the current page drops out and the other one is
					     promoted from ghost to solid. --}}
					@if(!.SignedIn())
						@if(!.IsCurrent(.LoginLink()))
							<a href="{{ .LoginLink() }}" class="btn" data-variant="ghost" data-size="sm">
								{!! icons.SignIn(icons.Props{}) !!} Sign in
							</a>
						@endif
						@if(.RegisterLink() != "" && !.IsCurrent(.RegisterLink()))
							<a href="{{ .RegisterLink() }}" class="btn" data-size="sm">
								{!! icons.UserPlus(icons.Props{}) !!} Create account
							</a>
						@endif
					@endif
					@if(.SignedIn())
						{{-- Both are drawn only when the controller filled them, and the
						     controller fills the admin one only for somebody the policy
						     would let in. The markup never asks about a role: a second
						     place that decides authorization is the one that gets it
						     wrong. An empty string draws nothing. --}}
						@if(.PanelLink() != "" && !.IsCurrent(.PanelLink()))
							<a href="{{ .PanelLink() }}" class="btn" data-variant="ghost" data-size="sm">
								{!! icons.SquaresFour(icons.Props{}) !!} Dashboard
							</a>
						@endif
						@if(.AdminLink() != "" && !.IsCurrent(.AdminLink()))
							<a href="{{ .AdminLink() }}" class="btn" data-variant="ghost" data-size="sm">
								{!! icons.ShieldCheck(icons.Props{}) !!} Moderation
							</a>
						@endif
						<span class="text-muted-foreground hidden sm:inline">{{ .SignedInName() }}</span>
						<form method="post" action="{{ .LogoutLink() }}">
							@csrf
							<button type="submit" class="btn" data-variant="ghost" data-size="sm">
								{!! icons.SignOut(icons.Props{}) !!} Sign out
							</button>
						</form>
					@endif
				</div>
			</nav>
			{{-- The mark's own palette, three pixels of it. See .brand-rule. --}}
			<div class="brand-rule" aria-hidden="true"></div>
		</header>

		<main class="mx-auto w-full max-w-5xl grow px-6 py-10">
			@yield('content')
		</main>

		{{-- The footer says what this is. An example that does not say it is an
		     example is an example somebody deploys. --}}
		<footer class="border-t">
			<div class="text-muted-foreground mx-auto flex max-w-5xl flex-col gap-2 px-6 py-8 text-sm sm:flex-row sm:items-center sm:justify-between">
				<p>An example application. Everything here is seeded.</p>
				<a class="hover:text-foreground inline-flex items-center gap-1.5" href="https://github.com/arandu-io/examples">
					{!! icons.GithubLogo(icons.Props{}) !!} Read the source
				</a>
			</div>
		</footer>

		{{-- The tray flash messages land in. An endpoint that saves something
		     answers with a toast fragment and hx-swap="beforeend" on this
		     element; the vendored script arms whatever appears inside it. --}}
		<div id="toaster" class="toaster" aria-live="polite"></div>
	</div>
</body>
</html>
