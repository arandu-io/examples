//go:build kyse

package comments

import (
	"github.com/arandu-io/kyse/components"

	"github.com/arandu-io/hesape/view"
)

@go
// CommentsCreateData is what CommentController.Create hands this page.
// A rejected submission comes back to it too: the router sends the browser
// back here, and the page carries the messages and what was typed from the
// flash. The controller passes nothing for that.
type CommentsCreateData struct {
	// Page is the state the layout draws. Its Token is what @csrf writes into
	// the hidden field, through Page.CSRFToken -- it comes from the page data
	// rather than from a global, because a template that reaches for request
	// state outside the data it was given is how a form ends up carrying
	// another session's token under load. It is also what every input asks for
	// its message and for what was typed on a rejected attempt.
	view.Page
	// Form is what the inputs start at: empty here, and the stored record on the
	// edit screen, which shares this type. What was typed on a rejected attempt
	// wins over it, through the Page.
	Form CommentForm
}

// Compile-time proof that this page fits the layout it extends.
var _ view.Layout = CommentsCreateData{}

// CommentForm is the form as text, which is what a form carries.
//
// What comes back after a rejection is exactly what was typed, from the flash,
// including a value that failed to parse -- retyping a whole form because one
// field was wrong is how a screen becomes unpleasant.
type CommentForm struct {
	// ID is empty on creation and set on edit, where it addresses the record.
	ID string
	// PostId is the Post id input.
	PostId string
	// Author is the Author input.
	Author string
	// Body is the Body input.
	Body string
	// Approved is the Approved input.
	Approved bool
}

// arandu:begin custom
// Anything else these forms need in Go goes here, and survives regeneration.
// arandu:end custom
@endgo

@extends('layouts.app')

@section('content')
	<nav class="text-sm text-slate-500 dark:text-slate-400">
		<a class="underline underline-offset-2 hover:text-slate-900 dark:hover:text-slate-100" href="/comments">Comments</a>
	</nav>

	<h1 class="mt-2 text-3xl font-semibold tracking-tight">{{ .Title }}</h1>

	<form class="mt-8 space-y-6" method="post" action="/comments" hx-post="/comments">
		@csrf
		
		{!! components.Field(components.FieldProps{
			Name:  "post_id",
			Label: "Post id",
			Type:  "text",
			Value: .Form.PostId,
			Page: .,
			Required: true,
		}) !!}

		{!! components.Field(components.FieldProps{
			Name:  "author",
			Label: "Author",
			Type:  "text",
			Value: .Form.Author,
			Page: .,
			Required: true,
		}) !!}

		{!! components.Textarea(components.TextareaProps{
			Name:  "body",
			Label: "Body",
			Value: .Form.Body,
			Page: .,
			Rows:  6,
			Required: true,
		}) !!}

		{!! components.Checkbox(components.CheckboxProps{
			Name:    "approved",
			Label:   "Approved",
			Value:   "1",
			Checked: .Form.Approved,
			Page: .,
		}) !!}

		<div class="flex items-center gap-3">
			<button class="rounded-md bg-slate-900 px-3 py-2 text-sm font-medium text-white hover:bg-slate-700 dark:bg-slate-100 dark:text-slate-900 dark:hover:bg-slate-300" type="submit">Save</button>
			<a class="text-sm text-slate-500 underline underline-offset-2 dark:text-slate-400" href="/comments">Cancel</a>
		</div>
	</form>
@endsection
