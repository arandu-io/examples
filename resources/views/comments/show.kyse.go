//go:build kyse

package comments

import "github.com/arandu-io/framework/view"

@go
// CommentsShowData is what CommentController.Show hands this page.
type CommentsShowData struct {
	// Page is the state the layout draws. Its Token is also what the delete
	// button sends as a header: an hx-delete carries no form body, so the
	// hidden field a form uses would never arrive and the request would be
	// refused with 419.
	view.Page
	// Comment is the record.
	Comment CommentRow
}

// Compile-time proof that this page fits the layout it extends.
var _ view.Layout = CommentsShowData{}

// arandu:begin custom
// Anything else this page needs in Go goes here, and survives regeneration.
// arandu:end custom
@endgo

@extends('layouts.app')

@section('content')
	<nav class="text-sm text-slate-500 dark:text-slate-400">
		<a class="underline underline-offset-2 hover:text-slate-900 dark:hover:text-slate-100" href="/comments">Comments</a>
	</nav>

	<div class="mt-2 flex items-center justify-between gap-4">
		<h1 class="text-3xl font-semibold tracking-tight">{{ .Title }}</h1>
		<div class="flex items-center gap-3">
			<a class="rounded-md border border-slate-300 px-3 py-2 text-sm font-medium hover:bg-slate-50 dark:border-slate-700 dark:hover:bg-slate-900" href="/comments/{{ .Comment.ID }}/edit">Edit</a>
			<button class="rounded-md border border-red-300 px-3 py-2 text-sm font-medium text-red-700 hover:bg-red-50 dark:border-red-900 dark:text-red-400 dark:hover:bg-red-950" type="button" hx-delete="/comments/{{ .Comment.ID }}" hx-headers='{"X-CSRF-Token": "{{ .Token }}"}' hx-confirm="Delete this comment?">Delete</button>
		</div>
	</div>

	<dl class="mt-8 divide-y divide-slate-100 border-t border-slate-200 text-sm dark:divide-slate-900 dark:border-slate-800">
				<div class="grid grid-cols-3 gap-4 py-3">
			<dt class="text-slate-500 dark:text-slate-400">Post id</dt>
			<dd class="col-span-2">{{ d.Comment.PostId }}</dd>
		</div>
		<div class="grid grid-cols-3 gap-4 py-3">
			<dt class="text-slate-500 dark:text-slate-400">Author</dt>
			<dd class="col-span-2">{{ d.Comment.Author }}</dd>
		</div>
		<div class="grid grid-cols-3 gap-4 py-3">
			<dt class="text-slate-500 dark:text-slate-400">Body</dt>
			<dd class="col-span-2">{{ d.Comment.Body }}</dd>
		</div>
		<div class="grid grid-cols-3 gap-4 py-3">
			<dt class="text-slate-500 dark:text-slate-400">Approved</dt>
			<dd class="col-span-2">{{ d.Comment.Approved }}</dd>
		</div>
		<div class="grid grid-cols-3 gap-4 py-3">
			<dt class="text-slate-500 dark:text-slate-400">Created</dt>
			<dd class="col-span-2">{{ .Comment.Created }}</dd>
		</div>
	</dl>
@endsection
