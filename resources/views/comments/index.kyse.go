//go:build kyse

package comments

import "github.com/arandu-io/framework/view"

@go
// CommentsIndexData is what CommentController.Index hands this page.
type CommentsIndexData struct {
	// view.Page is the chrome the layout draws: the title, the description, the
	// CSRF token and the navigation. Embedded rather than repeated, and what
	// makes this struct fit the layout.
	view.Page
	// Comments is the page of records.
	Comments []CommentRow
	// NextCursor is the keyset cursor of the following page. It is empty on the
	// last page, and the link is not rendered then.
	NextCursor string
}

// Compile-time proof that this page fits the layout it extends.
var _ view.Layout = CommentsIndexData{}

// CommentRow is one record, formatted for display by the controller.
type CommentRow struct {
	// ID is what the row links to.
	ID string
	// PostId is the Post id column.
	PostId string
	// Author is the Author column.
	Author string
	// Body is the Body column.
	Body string
	// Approved is the Approved column.
	Approved bool
	// Created is the creation timestamp, already formatted.
	Created string
}

// arandu:begin custom
// Anything else these pages need in Go goes here, and survives regeneration.
// arandu:end custom
@endgo

@extends('layouts.app')

@section('content')
	<div class="flex items-center justify-between gap-4">
		<h1 class="text-3xl font-semibold tracking-tight">{{ .Title }}</h1>
		<a class="rounded-md border border-slate-300 px-3 py-2 text-sm font-medium hover:bg-slate-50 dark:border-slate-700 dark:hover:bg-slate-900" href="/comments/create">New comment</a>
	</div>

	@if(len(d.Comments) == 0)
		<p class="mt-8 text-sm text-slate-500 dark:text-slate-400">
			No comment yet. <a class="underline underline-offset-2" href="/comments/create">Add the first one</a>.
		</p>
	@endif

	@if(len(d.Comments) > 0)
		<div class="mt-8 overflow-x-auto">
			<table class="w-full border-collapse text-left text-sm">
				<thead class="border-b border-slate-200 text-slate-500 dark:border-slate-800 dark:text-slate-400">
					<tr>
						<th class="py-2 pr-4 font-medium">Post id</th>
						<th class="py-2 pr-4 font-medium">Author</th>
						<th class="py-2 pr-4 font-medium">Body</th>
						<th class="py-2 pr-4 font-medium">Approved</th>
						<th class="py-2 font-medium">Created</th>
					</tr>
				</thead>
				<tbody>
					@foreach(.Comments as comment)
						<tr class="border-b border-slate-100 dark:border-slate-900">
							<td class="py-2 pr-4">
								<a class="font-medium underline underline-offset-2" href="/comments/{{ comment.ID }}">{{ comment.PostId }}</a>
							</td>
												<td class="py-2 pr-4">{{ comment.Author }}</td>
					<td class="py-2 pr-4">{{ comment.Body }}</td>
					<td class="py-2 pr-4">{{ comment.Approved }}</td>
							<td class="py-2 text-slate-500 dark:text-slate-400">{{ comment.Created }}</td>
						</tr>
					@endforeach
				</tbody>
			</table>
		</div>
	@endif

	@if(d.NextCursor != "")
		<a class="mt-6 inline-block text-sm underline underline-offset-2" href="/comments?cursor={{ .NextCursor }}">Next page</a>
	@endif
@endsection
