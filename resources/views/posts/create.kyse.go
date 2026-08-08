//go:build kyse

package posts

import (
	"github.com/arandu-io/kyse/components"

	"github.com/arandu-io/framework/view"
)

@go
// PostsCreateData is what PostController.Create hands this page, and what
// Store hands it back when the submission was rejected: same view, same data,
// with the messages filled in.
type PostsCreateData struct {
	// Page is the state the layout draws. Its Token is what @csrf writes into
	// the hidden field, through Page.CSRFToken -- it comes from the page data
	// rather than from a global, because a template that reaches for request
	// state outside the data it was given is how a form ends up carrying
	// another session's token under load.
	view.Page
	// Form is what was typed, so a rejected submission comes back filled in.
	Form PostForm
	// Errors is the message per field, as validation produced it.
	Errors map[string][]string
}

// FieldError is the first message for a field, or empty.
//
// A method rather than a lookup in the markup: a view that indexes a map has to
// check the length first, and d.Errors["title"][0] without that check panics
// on the happy path -- which is the request where nothing was wrong.
func (d PostsCreateData) FieldError(field string) string {
	if msgs := d.Errors[field]; len(msgs) > 0 {
		return msgs[0]
	}
	return ""
}

// Compile-time proof that this page fits the layout it extends.
var _ view.Layout = PostsCreateData{}

// PostForm is the form as text, which is what a form carries.
//
// The value that comes back after a rejection is exactly what was typed,
// including the number that failed to parse -- retyping a whole form because one
// field was wrong is how a screen becomes unpleasant.
type PostForm struct {
	// ID is empty on creation and set on edit, where it addresses the record.
	ID string
	// Title is the Title input.
	Title string
	// Slug is the Slug input.
	Slug string
	// Body is the Body input.
	Body string
	// PublishedAt is the Published at input.
	PublishedAt string
}

// arandu:begin custom
// Anything else these forms need in Go goes here, and survives regeneration.
// arandu:end custom
@endgo

@extends('layouts.app')

@section('content')
	<nav class="text-sm text-slate-500 dark:text-slate-400">
		<a class="underline underline-offset-2 hover:text-slate-900 dark:hover:text-slate-100" href="/posts">Posts</a>
	</nav>

	<h1 class="mt-2 text-3xl font-semibold tracking-tight">{{ .Title }}</h1>

	<form class="mt-8 space-y-6" method="post" action="/posts" hx-post="/posts" hx-target="this" hx-swap="outerHTML">
		@csrf
		
		{!! components.Field(components.FieldProps{
			Name:  "title",
			Label: "Title",
			Type:  "text",
			Value: .Form.Title,
			Error: .FieldError("title"),
			Required: true,
		}) !!}

		{!! components.Field(components.FieldProps{
			Name:  "slug",
			Label: "Slug",
			Type:  "text",
			Value: .Form.Slug,
			Error: .FieldError("slug"),
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

		{!! components.Field(components.FieldProps{
			Name:  "published_at",
			Label: "Published at",
			Type:  "datetime-local",
			Value: .Form.PublishedAt,
			Error: .FieldError("published_at"),
		}) !!}

		<div class="flex items-center gap-3">
			<button class="rounded-md bg-slate-900 px-3 py-2 text-sm font-medium text-white hover:bg-slate-700 dark:bg-slate-100 dark:text-slate-900 dark:hover:bg-slate-300" type="submit">Save</button>
			<a class="text-sm text-slate-500 underline underline-offset-2 dark:text-slate-400" href="/posts">Cancel</a>
		</div>
	</form>
@endsection
