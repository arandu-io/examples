//go:build kyse

package posts

import (
	"github.com/arandu-io/kyse/components"
	"github.com/arandu-io/kyse/icons"

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

	// Sections is the navigation across the top of the listing: every category
	// with published posts in it. It is empty on a blog with no sections, and
	// an empty list draws no bar.
	Sections []SectionLink
	// Heading and Standfirst are what this listing says it is. A section page
	// and the front page are the same view with a different pair -- one screen
	// rather than two that drift.
	Heading    string
	Standfirst string
	// NewURL is where the "write one" button goes, and it is empty for a reader
	// who cannot write. An empty URL draws no button, which is how the policy
	// reaches the markup without the markup asking it anything.
	NewURL string
}

// Lead is the article at the top, and Rest is everything under it.
//
// Two methods rather than an index in the template, because kyse has no index
// and should not grow one for this (RULE 15). What the pair buys is the shape a
// front page has: one piece given room, the others in a list -- which is the
// difference between a blog and a table of blog.
//
// Lead returns the zero value on an empty page, and the template asks len()
// before it draws anything.
func (d PostsIndexData) Lead() PostRow {
	if len(d.Posts) == 0 {
		return PostRow{}
	}
	return d.Posts[0]
}

// Rest is everything after the lead.
func (d PostsIndexData) Rest() []PostRow {
	if len(d.Posts) < 2 {
		return nil
	}
	return d.Posts[1:]
}

// Compile-time proof that this page fits the layout it extends.
var _ view.Layout = PostsIndexData{}

// SectionLink is one entry of the section bar.
type SectionLink struct {
	// Name is what it is called and URL is where it goes.
	Name string
	URL  string
	// Count is how many published posts it holds, already formatted.
	Count string
	// Current marks the section being read, so the bar can show where you are.
	Current bool
}

// Aria is the aria-current attribute value: "page" on the section being read,
// "false" everywhere else.
//
// A method and not a conditional attribute in the template, because a
// conditional attribute has no directive and inventing one would grow the DSL
// for a single case (RULE 15). What does not fit a directive is written in Go.
func (s SectionLink) Aria() string {
	if s.Current {
		return "page"
	}
	return "false"
}

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

	// Category is the section this post is filed under, and CategoryURL is
	// where that section is read. Both are empty for a post filed nowhere, and
	// an empty name draws no chip -- which is how "unfiled" reaches the markup
	// without the markup asking what unfiled means.
	Category    string
	CategoryURL string

	// Views is how many times the article was opened, already formatted. It is
	// empty until somebody has read it: "0 reads" under a post published a
	// minute ago is a worse thing to say than nothing.
	Views string
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
func (r PostRow) When() string {
	if r.PublishedAt == "" {
		return "not published"
	}
	return r.PublishedAt
}

// HasComments says whether the count is worth drawing.
//
// "0 comments" under every card of a new blog is a column of zeroes that says
// the same thing five times, and it says it about the thing you would rather
// nobody counted. Empty and "0" are the same answer here -- the listing does not
// count the thread at all, so it sends neither.
func (r PostRow) HasComments() bool {
	return r.Comments != "" && r.Comments != "0"
}
@endgo

@extends('layouts.app')

@section('content')
	<header class="flex items-end justify-between gap-6">
		<div>
			<h1 class="headline headline-xl">{{ .Heading }}</h1>
			@if(.Standfirst != "")
				<p class="standfirst mt-3">{{ .Standfirst }}</p>
			@endif
		</div>
		@if(.NewURL != "")
			<a class="btn shrink-0" href="{{ .NewURL }}">
				{!! icons.PencilSimple(icons.Props{}) !!} Write one
			</a>
		@endif
	</header>

	{{-- The sections. Drawn only when there are some, because a bar with one
	     chip in it is a bar that says nothing and takes a row. --}}
	@if(len(.Sections) > 0)
		<nav class="mt-8 flex flex-wrap items-center gap-2 border-y py-4" aria-label="Sections">
			<a class="section-chip" href="{{ .HomeLink() }}">All</a>
			@foreach(.Sections as section)
				<a class="section-chip" href="{{ section.URL }}" aria-current="{{ section.Aria() }}">
					{{ section.Name }}
					<span class="text-muted-foreground tabular-nums">{{ section.Count }}</span>
				</a>
			@endforeach
		</nav>
	@endif

	{{-- The lead. One article given the room a front page gives its first, and
	     the rest in a list under it. --}}
	@if(len(.Posts) > 0)
		<article class="mt-10 border-b pb-10">
			{{-- Three facts, three glyphs. They were one string joined by middots,
			     which reads as one fact in three parts -- and the eye has to
			     parse the sentence to find the date. A calendar, an eye and a
			     speech bubble are told apart without reading. --}}
			<div class="eyebrow flex flex-wrap items-center gap-x-3 gap-y-1">
				@if(.Lead().Category != "")
					<a class="section-tag" href="{{ .Lead().CategoryURL }}">
						{!! icons.Tag(icons.Props{}) !!} {{ .Lead().Category }}
					</a>
				@endif
				<span class="meta-fact">{!! icons.CalendarBlank(icons.Props{}) !!} {{ .Lead().When() }}</span>
				@if(.Lead().Views != "")
					<span class="meta-fact">{!! icons.Eye(icons.Props{}) !!} {{ .Lead().Views }}</span>
				@endif
				@if(.Lead().HasComments())
					<span class="meta-fact">{!! icons.ChatCircle(icons.Props{}) !!} {{ .Lead().Comments }}</span>
				@endif
			</div>

			<h2 class="headline headline-lg mt-3">
				<a class="hover:underline" href="{{ .Lead().URL }}">{{ .Lead().Title }}</a>
			</h2>
			<p class="standfirst mt-3">{{ .Lead().Excerpt }}</p>
			<p class="mt-4">
				<a class="inline-flex items-center gap-1.5 text-sm font-medium hover:underline" href="{{ .Lead().URL }}">
					Read it {!! icons.ArrowRight(icons.Props{}) !!}
				</a>
			</p>
		</article>
	@endif

	@if(len(.Rest()) > 0)
		<ul class="divide-y">
			@foreach(.Rest() as post)
				<li class="py-6">
					<a class="group flex flex-col gap-2" href="{{ post.URL }}">
						<div class="eyebrow flex flex-wrap items-center gap-x-3 gap-y-1">
							@if(post.Category != "")
								<span class="section-tag">{!! icons.Tag(icons.Props{}) !!} {{ post.Category }}</span>
							@endif
							<span class="meta-fact">{!! icons.CalendarBlank(icons.Props{}) !!} {{ post.When() }}</span>
							@if(post.Views != "")
								<span class="meta-fact">{!! icons.Eye(icons.Props{}) !!} {{ post.Views }}</span>
							@endif
							@if(post.HasComments())
								<span class="meta-fact">{!! icons.ChatCircle(icons.Props{}) !!} {{ post.Comments }}</span>
							@endif
						</div>
						<h3 class="headline headline-sm group-hover:underline">{{ post.Title }}</h3>
						<p class="text-muted-foreground text-sm leading-relaxed">{{ post.Excerpt }}</p>
					</a>
				</li>
			@endforeach
		</ul>
	@endif

	{{-- Not @forelse: the list element belongs outside the loop, and @forelse
	     puts both halves inside it -- so the <ul> came out once per post. --}}
	@if(len(.Posts) == 0)
		<div class="mt-10">
			{!! components.Empty(components.EmptyProps{
				Title:       "Nothing here yet",
				Message:     "The first post is the hardest.",
				ActionLabel: "Write one",
				ActionURL:   .NewURL,
			}) !!}
		</div>
	@endif

	@if(.NextCursor != "")
		<nav class="mt-10 flex justify-center">
			<a class="btn" data-variant="outline" href="?cursor={{ .NextCursor }}">
				Older posts {!! icons.ArrowRight(icons.Props{}) !!}
			</a>
		</nav>
	@endif
@endsection
