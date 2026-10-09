//go:build kyse

package categories

import (
	"github.com/arandu-io/kyse/components"

	"github.com/arandu-io/hesape/view"
)

@go
// CategoriesCreateData is what CategoryController.Create hands this page.
// A rejected submission comes back to it too: the router sends the browser
// back here, and the page carries the messages and what was typed from the
// flash. The controller passes nothing for that.
type CategoriesCreateData struct {
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
	Form CategoryForm
}

// Compile-time proof that this page fits the layout it extends.
var _ view.Layout = CategoriesCreateData{}

// CategoryForm is the form as text, which is what a form carries.
//
// What comes back after a rejection is exactly what was typed, from the flash,
// including a value that failed to parse -- retyping a whole form because one
// field was wrong is how a screen becomes unpleasant.
type CategoryForm struct {
	// ID is empty on creation and set on edit, where it addresses the record.
	ID string
	// Name is the Name input.
	Name string
	// Slug is the Slug input.
	Slug string
	// Description is the Description input.
	Description string
}

// arandu:begin custom
// Anything else these forms need in Go goes here, and survives regeneration.
// arandu:end custom
@endgo

@extends('layouts.app')

@section('content')
	<nav class="text-sm text-slate-500 dark:text-slate-400">
		<a class="underline underline-offset-2 hover:text-slate-900 dark:hover:text-slate-100" href="/categories">Categories</a>
	</nav>

	<h1 class="mt-2 text-3xl font-semibold tracking-tight">{{ .Title }}</h1>

	<form class="mt-8 space-y-6" method="post" action="/categories" hx-post="/categories">
		@csrf
		
		{!! components.Field(components.FieldProps{
			Name:  "name",
			Label: "Name",
			Type:  "text",
			Value: .Form.Name,
			Page: .,
			Required: true,
		}) !!}

		{!! components.Field(components.FieldProps{
			Name:  "slug",
			Label: "Slug",
			Type:  "text",
			Value: .Form.Slug,
			Page: .,
			Required: true,
		}) !!}

		{!! components.Textarea(components.TextareaProps{
			Name:  "description",
			Label: "Description",
			Value: .Form.Description,
			Page: .,
			Rows:  6,
		}) !!}

		<div class="flex items-center gap-3">
			<button class="rounded-md bg-slate-900 px-3 py-2 text-sm font-medium text-white hover:bg-slate-700 dark:bg-slate-100 dark:text-slate-900 dark:hover:bg-slate-300" type="submit">Save</button>
			<a class="text-sm text-slate-500 underline underline-offset-2 dark:text-slate-400" href="/categories">Cancel</a>
		</div>
	</form>
@endsection
