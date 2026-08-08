//go:build kyse

package comments

import (
	"github.com/arandu-io/kyse/components"

	"github.com/arandu-io/framework/view"
)

@go
// CommentsEditData is what CommentController.Edit hands this page: the form
// filled in with a stored record, or with what was typed when Update rejected it.
type CommentsEditData struct {
	// Page is the state the layout draws. Its Token is what @csrf writes into
	// the hidden field.
	view.Page
	// Form is the record as text.
	Form CommentForm
	// Errors is the message per field, as validation produced it.
	Errors map[string][]string
}

// FieldError is the first message for a field, or empty.
//
// A method rather than a lookup in the markup: a view that indexes a map has to
// check the length first, and d.Errors["title"][0] without that check panics
// on the happy path -- which is the request where nothing was wrong.
func (d CommentsEditData) FieldError(field string) string {
	if msgs := d.Errors[field]; len(msgs) > 0 {
		return msgs[0]
	}
	return ""
}

// Compile-time proof that this page fits the layout it extends.
var _ view.Layout = CommentsEditData{}

// arandu:begin custom
// Anything else this page needs in Go goes here, and survives regeneration.
// arandu:end custom
@endgo

@extends('layouts.app')

@section('content')
	<nav class="text-sm text-slate-500 dark:text-slate-400">
		<a class="underline underline-offset-2 hover:text-slate-900 dark:hover:text-slate-100" href="/comments/{{ .Form.ID }}">Back</a>
	</nav>

	<h1 class="mt-2 text-3xl font-semibold tracking-tight">{{ .Title }}</h1>

	<!-- hx-put, and no action: a browser form can only send GET and POST, and
	the update route is PUT. HTMX sends the real method, which is why this
	stack does not need a hidden _method field. -->
	<form class="mt-8 space-y-6" hx-put="/comments/{{ .Form.ID }}" hx-target="this" hx-swap="outerHTML">
		@csrf
		
		{!! components.Field(components.FieldProps{
			Name:  "post_id",
			Label: "Post id",
			Type:  "text",
			Value: .Form.PostId,
			Error: .FieldError("post_id"),
			Required: true,
		}) !!}

		{!! components.Field(components.FieldProps{
			Name:  "author",
			Label: "Author",
			Type:  "text",
			Value: .Form.Author,
			Error: .FieldError("author"),
			Required: true,
		}) !!}

		{!! components.Textarea(components.TextareaProps{
			Name:  "body",
			Label: "Body",
			Value: .Form.Body,
			Error: .FieldError("body"),
			Rows:  6,
			Required: true,
		}) !!}

		<label class="flex items-center gap-2 text-sm">
			<input class="input" id="approved" name="approved" type="checkbox" value="1" {{ .Form.ApprovedAttr() }}>
			Approved
		</label>

		<div class="flex items-center gap-3">
			<button class="rounded-md bg-slate-900 px-3 py-2 text-sm font-medium text-white hover:bg-slate-700 dark:bg-slate-100 dark:text-slate-900 dark:hover:bg-slate-300" type="submit">Save</button>
			<a class="text-sm text-slate-500 underline underline-offset-2 dark:text-slate-400" href="/comments/{{ .Form.ID }}">Cancel</a>
		</div>
	</form>
@endsection
