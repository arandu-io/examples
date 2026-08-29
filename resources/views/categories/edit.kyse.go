//go:build kyse

package categories

import (
	"github.com/arandu-io/kyse/components"

	"github.com/arandu-io/hesape/view"
)

@go
// CategoriesEditData is what CategoryController.Edit hands this page: the form
// filled in with a stored record, or with what was typed when Update rejected it.
type CategoriesEditData struct {
	// Page is the state the layout draws. Its Token is what @csrf writes into
	// the hidden field.
	view.Page
	// Form is the record as text.
	Form CategoryForm
	// Errors is the message per field, as validation produced it.
	Errors map[string][]string
}

// FieldError is the first message for a field, or empty.
//
// A method rather than a lookup in the markup: a view that indexes a map has to
// check the length first, and d.Errors["title"][0] without that check panics
// on the happy path -- which is the request where nothing was wrong.
func (d CategoriesEditData) FieldError(field string) string {
	if msgs := d.Errors[field]; len(msgs) > 0 {
		return msgs[0]
	}
	return ""
}

// Compile-time proof that this page fits the layout it extends.
var _ view.Layout = CategoriesEditData{}

// arandu:begin custom
// Anything else this page needs in Go goes here, and survives regeneration.
// arandu:end custom
@endgo

@extends('layouts.app')

@section('content')
	<nav class="text-sm text-slate-500 dark:text-slate-400">
		<a class="underline underline-offset-2 hover:text-slate-900 dark:hover:text-slate-100" href="/categories/{{ .Form.ID }}">Back</a>
	</nav>

	<h1 class="mt-2 text-3xl font-semibold tracking-tight">{{ .Title }}</h1>

	<!-- hx-put, and no action: a browser form can only send GET and POST, and
	the update route is PUT. HTMX sends the real method, which is why this
	stack does not need a hidden _method field. -->
	<form class="mt-8 space-y-6" hx-put="/categories/{{ .Form.ID }}" hx-target="this" hx-swap="outerHTML">
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
			<a class="text-sm text-slate-500 underline underline-offset-2 dark:text-slate-400" href="/categories/{{ .Form.ID }}">Cancel</a>
		</div>
	</form>
@endsection
