//go:build kyse

package posts

import (
	"github.com/arandu-io/kyse/components"

	"github.com/arandu-io/framework/view"
)

@go
// PostsShowData is what PostController.Show hands this page.
type PostsShowData struct {
	// view.Page carries the title, the description and the canonical URL, which
	// is what makes a post indexable on its own terms rather than under the
	// application's name.
	view.Page

	// Post is what is being read.
	Post PostRow

	// Comments is what people answered, oldest first, because a conversation
	// reads forwards.
	Comments []CommentRow
	// CommentURL is where a new comment is posted. It is empty for somebody the
	// policy will not accept, and an empty URL draws no form -- so the rule
	// reaches the markup as data rather than as a question the view asks.
	CommentURL string

	// EditURL and DeleteURL are empty for a reader. Same reason.
	EditURL   string
	DeleteURL string
}

// Compile-time proof that this page fits the layout it extends.
var _ view.Layout = PostsShowData{}

// CommentRow is one comment, formatted for display by the controller.
type CommentRow struct {
	// ID is what a moderation action addresses.
	ID string
	// Author is the display name of whoever wrote it.
	Author string
	// Body is what they said.
	Body string
	// Created is when, already formatted.
	Created string
	// Approved says whether it is visible to everybody. A comment awaiting
	// moderation is shown to its author and to an administrator, marked.
	Approved bool
}

// Badge is the word beside a comment that is not public yet, or nothing.
func (c CommentRow) Badge() string {
	if c.Approved {
		return ""
	}
	return "awaiting review"
}
@endgo

@extends('layouts.app')

@section('content')
	<article class="mx-auto w-full max-w-2xl">
		<header>
			<h1 class="text-3xl font-semibold tracking-tight">{{ .Post.Title }}</h1>
			<p class="text-muted-foreground mt-2 text-sm">{{ .Post.Meta() }}</p>
		</header>

		{{-- The body is written by an author, so it is escaped like everything
		     else. Rendering it raw would be the one hole in a view layer whose
		     whole claim is that escaping is not a decision anybody makes. --}}
		<div class="mt-8 text-base leading-7 whitespace-pre-line">{{ .Post.Body }}</div>

		@if(.EditURL != "")
			<footer class="mt-10 flex items-center gap-3 border-t pt-6">
				<a class="btn" data-variant="outline" data-size="sm" href="{{ .EditURL }}">Edit</a>
				<button type="button" class="btn" data-variant="destructive" data-size="sm"
					onclick="deletePost.showModal()">Delete</button>
			</footer>

			{!! components.Dialog(components.DialogProps{
				ID:             "deletePost",
				Title:          "Delete this post?",
				Message:        "The post and every comment on it go with it. This cannot be undone.",
				ConfirmLabel:   "Delete it",
				ConfirmVariant: "destructive",
				Action:         .DeleteURL,
				Token:          .CSRFToken(),
			}) !!}
		@endif
	</article>

	<section class="mx-auto mt-16 w-full max-w-2xl border-t pt-10">
		<h2 class="text-lg font-semibold tracking-tight">
			Comments
			@if(len(.Comments) > 0)
				{!! components.Badge(components.BadgeProps{Label: .Post.Comments, Variant: "secondary"}) !!}
			@endif
		</h2>

		@if(len(.Comments) > 0)
			<ul class="mt-6 flex flex-col gap-6">
				@foreach(.Comments as comment)
					<li class="flex gap-3">
						{!! components.Avatar(components.AvatarProps{Name: comment.Author, Size: "sm"}) !!}
						<div class="min-w-0 flex-1">
							<div class="flex items-center gap-2">
								<span class="text-sm font-medium">{{ comment.Author }}</span>
								<span class="text-muted-foreground text-xs">{{ comment.Created }}</span>
								@if(comment.Badge() != "")
									{!! components.Badge(components.BadgeProps{Label: comment.Badge(), Variant: "outline"}) !!}
								@endif
							</div>
							<p class="mt-1 text-sm whitespace-pre-line">{{ comment.Body }}</p>
						</div>
					</li>
				@endforeach
			</ul>
		@endif

		@if(len(.Comments) == 0)
			<p class="text-muted-foreground mt-6 text-sm">Nobody has said anything yet.</p>
		@endif

		@if(.CommentURL != "")
			{{-- The form posts over HTMX and the answer is appended to the list
			     above, so writing a comment does not reload the article somebody
			     is in the middle of reading. --}}
			<form class="mt-10 flex flex-col gap-4" method="post" action="{{ .CommentURL }}"
				hx-post="{{ .CommentURL }}" hx-target="this" hx-swap="outerHTML">
				@csrf

				{!! components.Textarea(components.TextareaProps{
					Name:        "body",
					Label:       "Say something",
					Rows:        4,
					Placeholder: "Be kind.",
					Hint:        "Comments are reviewed before they appear.",
					Required:    true,
				}) !!}

				<div>
					<button type="submit" class="btn">Post the comment</button>
				</div>
			</form>
		@endif

		@if(.CommentURL == "")
			<p class="text-muted-foreground mt-10 text-sm">
				<a class="hover:underline" href="{{ .LoginLink() }}">Sign in</a> to leave a comment.
			</p>
		@endif
	</section>
@endsection
