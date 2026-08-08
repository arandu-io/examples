//go:build kyse

package posts

import (
	"github.com/arandu-io/kyse/components"

	"github.com/arandu-io/framework/view"
)

@go
// PostsIndexData is what PostController.Index hands this page.
type PostsIndexData struct {
	// view.Page is the chrome the layout draws: the title, the description, the
	// CSRF token and the navigation. Embedded rather than repeated, and what
	// makes this struct fit the layout.
	view.Page

	// Posts is the page of records.
	Posts []PostRow
	// NextCursor is the keyset cursor of the following page. It is empty on the
	// last page, and the link is not rendered then.
	NextCursor string
	// NewURL is where the "write one" button goes, and it is empty for a reader
	// who cannot write. An empty URL draws no button, which is how the policy
	// reaches the markup without the markup asking it anything.
	NewURL string
}

// Compile-time proof that this page fits the layout it extends.
var _ view.Layout = PostsIndexData{}

// PostRow is one record, formatted for display by the controller.
//
// Formatted there rather than here: a date is a decision about presentation,
// and a view that formats one is a view with a locale in it.
type PostRow struct {
	// ID is what the row links to.
	ID string
	// Title is the Title column.
	Title string
	// Slug is the Slug column, and what the public URL is built from.
	Slug string
	// Body is the Body column.
	Body string
	// Excerpt is the opening of the body, cut to a line.
	Excerpt string
	// PublishedAt is the publication date, already formatted. It is empty on a
	// draft, and that is what the badge below reads.
	PublishedAt string
	// Created is the creation timestamp, already formatted.
	Created string
	// URL is where this post is read.
	URL string
	// Comments is how many comments it has.
	Comments string
}

// Status is the word on the badge: what state this post is in.
func (r PostRow) Status() string {
	if r.PublishedAt == "" {
		return "draft"
	}
	return "published"
}

// StatusVariant styles that badge. A draft is not an error, so it is the quiet
// variant rather than the loud one.
func (r PostRow) StatusVariant() string {
	if r.PublishedAt == "" {
		return "outline"
	}
	return "secondary"
}

// Meta is the grey line under the title: when it went out, and how many people
// answered.
func (r PostRow) Meta() string {
	when := r.PublishedAt
	if when == "" {
		when = "not published"
	}
	return when + " · " + r.Comments + " comments"
}
@endgo

@extends('layouts.app')

@section('content')
	<header class="flex items-end justify-between gap-4">
		<div>
			<h1 class="text-2xl font-semibold tracking-tight">Posts</h1>
			<p class="text-muted-foreground mt-1 text-sm">Everything written here, newest first.</p>
		</div>
		@if(.NewURL != "")
			<a class="btn" href="{{ .NewURL }}">Write one</a>
		@endif
	</header>

	@if(len(.Posts) > 0)
		<ul class="mt-8 flex flex-col gap-4">
			@foreach(.Posts as post)
				<li>
					{!! components.Card(components.CardProps{
						Title:        post.Title,
						Description:  post.Excerpt,
						Href:         post.URL,
						Meta:         post.Meta(),
						Badge:        post.Status(),
						BadgeVariant: post.StatusVariant(),
					}) !!}
				</li>
			@endforeach
		</ul>
	@endif

	{{-- Not @forelse: the list element belongs outside the loop, and @forelse
	     puts both halves inside it -- so the <ul> came out once per post. --}}
	@if(len(.Posts) == 0)
		<div class="mt-8">
			{!! components.Empty(components.EmptyProps{
				Title:       "Nothing here yet",
				Message:     "The first post is the hardest.",
				ActionLabel: "Write one",
				ActionURL:   .NewURL,
			}) !!}
		</div>
	@endif

	@if(.NextCursor != "")
		<nav class="mt-8 flex justify-center">
			<a class="btn" data-variant="outline" href="?cursor={{ .NextCursor }}">Older posts</a>
		</nav>
	@endif
@endsection
