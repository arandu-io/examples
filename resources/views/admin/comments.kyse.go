//go:build kyse

package admin

import "github.com/arandu-io/kyse/components"

@go
// CommentsData is the moderation queue.
type CommentsData struct {
	Chrome

	// Comments is everything written, newest state first: what is waiting is
	// what somebody came here to do.
	Comments []CommentRow
}

// CommentRow is one comment, with the two addresses that act on it.
//
// The URLs come from the controller, built from route names. A view that wrote
// "/admin/comments/"+id would keep compiling after the route moved, and the
// button would 404 in silence.
type CommentRow struct {
	ID      string
	Author  string
	Body    string
	Created string
	// Approved says whether it is public. It decides which button is drawn.
	Approved bool

	ApproveURL string
	DeleteURL  string
}

// Status is the word on the badge.
func (r CommentRow) Status() string {
	if r.Approved {
		return "public"
	}
	return "awaiting review"
}

// StatusVariant styles it. Waiting is not an error, so it is the quiet variant.
func (r CommentRow) StatusVariant() string {
	if r.Approved {
		return "secondary"
	}
	return "outline"
}
@endgo

@extends('admin.layout')

@section('content')
	<header>
		<h1 class="text-2xl font-semibold tracking-tight">Comments</h1>
		<p class="text-muted-foreground mt-1 text-sm">Nothing appears on a post until it is approved here.</p>
	</header>

	@if(len(.Comments) > 0)
		<ul class="mt-8 flex flex-col gap-4">
			@foreach(.Comments as comment)
				<li class="card p-5">
					<div class="flex items-start justify-between gap-4">
						<div class="flex min-w-0 gap-3">
							{!! components.Avatar(components.AvatarProps{Name: comment.Author, Size: "sm"}) !!}
							<div class="min-w-0">
								<div class="flex items-center gap-2">
									<span class="text-sm font-medium">{{ comment.Author }}</span>
									<span class="text-muted-foreground text-xs">{{ comment.Created }}</span>
								</div>
								<p class="mt-1 text-sm whitespace-pre-line">{{ comment.Body }}</p>
							</div>
						</div>
						{!! components.Badge(components.BadgeProps{Label: comment.Status(), Variant: comment.StatusVariant()}) !!}
					</div>

					<div class="mt-4 flex items-center gap-2 border-t pt-4">
						@if(!comment.Approved)
							<form method="post" action="{{ comment.ApproveURL }}">
								@csrf
								<button type="submit" class="btn" data-size="sm">Approve</button>
							</form>
						@endif
						<form method="post" action="{{ comment.DeleteURL }}">
							@csrf
							<button type="submit" class="btn" data-variant="ghost" data-size="sm">Delete</button>
						</form>
					</div>
				</li>
			@endforeach
		</ul>
	@endif

	@if(len(.Comments) == 0)
		<div class="mt-8">
			{!! components.Empty(components.EmptyProps{
				Title:   "Nothing to review",
				Message: "Every comment written so far has been dealt with.",
			}) !!}
		</div>
	@endif
@endsection
