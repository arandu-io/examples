//go:build kyse

package layouts

import "github.com/arandu-io/kyse/components"

<!doctype html>
<html lang="en" class="h-full">
<head>
	<meta charset="utf-8">
	<meta name="viewport" content="width=device-width, initial-scale=1">
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

	<link rel="stylesheet" href="{{ view.URL("app.css") }}">
	<script src="{{ view.URL("htmx.min.js") }}" defer></script>
	<script src="{{ view.URL("alpine.min.js") }}" defer></script>
	<script src="{{ view.URL("basecoat.bundle.js") }}" defer></script>
	<script src="{{ view.URL("theme.js") }}"></script>
</head>
<body hx-boost="true" hx-headers='{"X-CSRF-Token": "{{ .CSRFToken() }}"}' class="bg-background text-foreground min-h-full antialiased">
	<div class="flex min-h-full flex-col">
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
					<span class="headline text-lg">{{ .BrandName() }}</span>
				</a>
				<div class="flex items-center gap-2 text-sm">
					{!! components.ThemeToggle() !!}
					@if(!.SignedIn())
						<a href="{{ .LoginLink() }}" class="btn" data-variant="ghost" data-size="sm">Sign in</a>
						@if(.RegisterLink() != "")
							<a href="{{ .RegisterLink() }}" class="btn" data-size="sm">Create account</a>
						@endif
					@endif
					@if(.SignedIn())
						<span class="text-muted-foreground hidden sm:inline">{{ .SignedInName() }}</span>
						<form method="post" action="{{ .LogoutLink() }}">
							@csrf
							<button type="submit" class="btn" data-variant="ghost" data-size="sm">Sign out</button>
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
				<a class="hover:text-foreground" href="https://github.com/arandu-io/examples">Read the source</a>
			</div>
		</footer>

		{{-- The tray flash messages land in. An endpoint that saves something
		     answers with a toast fragment and hx-swap="beforeend" on this
		     element; the vendored script arms whatever appears inside it. --}}
		<div id="toaster" class="toaster" aria-live="polite"></div>
	</div>
</body>
</html>
