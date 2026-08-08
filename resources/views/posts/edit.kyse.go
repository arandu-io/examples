//go:build kyse

package views

@go
// PostsEditData is what PostController.Edit hands this page: the form
// filled in with a stored record, or with what was typed when Update rejected it.
type PostsEditData struct {
	// Page is the state the layout draws. Its Token is what @csrf writes into
	// the hidden field.
	Page
	// Form is the record as text.
	Form PostForm
	// Errors is the message per field, as validation produced it.
	Errors map[string][]string
}

// Compile-time proof that this page fits the layout it extends.
var _ Layout = PostsEditData{}

// arandu:begin custom
// Anything else this page needs in Go goes here, and survives regeneration.
// arandu:end custom
@endgo

@extends('layouts.app')

@section('content')
	<nav class="text-sm text-slate-500 dark:text-slate-400">
		<a class="underline underline-offset-2 hover:text-slate-900 dark:hover:text-slate-100" href="/posts/{{ .Form.ID }}">Back</a>
	</nav>

	<h1 class="mt-2 text-3xl font-semibold tracking-tight">{{ .Title }}</h1>

	<!-- hx-put, and no action: a browser form can only send GET and POST, and
	     the update route is PUT. HTMX sends the real method, which is why this
	     stack does not need a hidden _method field. -->
	<form class="mt-8 space-y-6" hx-put="/posts/{{ .Form.ID }}" hx-target="this" hx-swap="outerHTML">
		@csrf

		<div class="space-y-1">
			<label class="block text-sm font-medium" for="title">Title</label>
			<input class="block w-full rounded-md border border-slate-300 px-3 py-2 text-sm dark:border-slate-700 dark:bg-slate-900" id="title" name="title" type="text" value="{{ .Form.Title }}" required>
			@if(len(d.Errors["title"]) > 0)
			<p class="text-sm text-red-600 dark:text-red-400">{{ d.Errors["title"][0] }}</p>
			@endif
		</div>

		<div class="space-y-1">
			<label class="block text-sm font-medium" for="slug">Slug</label>
			<input class="block w-full rounded-md border border-slate-300 px-3 py-2 text-sm dark:border-slate-700 dark:bg-slate-900" id="slug" name="slug" type="text" value="{{ .Form.Slug }}" required>
			@if(len(d.Errors["slug"]) > 0)
			<p class="text-sm text-red-600 dark:text-red-400">{{ d.Errors["slug"][0] }}</p>
			@endif
		</div>

		<div class="space-y-1">
			<label class="block text-sm font-medium" for="body">Body</label>
			<textarea class="block w-full rounded-md border border-slate-300 px-3 py-2 text-sm dark:border-slate-700 dark:bg-slate-900" id="body" name="body" rows="4" required>{{ .Form.Body }}</textarea>
			@if(len(d.Errors["body"]) > 0)
			<p class="text-sm text-red-600 dark:text-red-400">{{ d.Errors["body"][0] }}</p>
			@endif
		</div>

		<div class="space-y-1">
			<label class="block text-sm font-medium" for="published_at">Published at</label>
			<input class="block w-full rounded-md border border-slate-300 px-3 py-2 text-sm dark:border-slate-700 dark:bg-slate-900" id="published_at" name="published_at" type="datetime-local" value="{{ .Form.PublishedAt }}">
			@if(len(d.Errors["published_at"]) > 0)
			<p class="text-sm text-red-600 dark:text-red-400">{{ d.Errors["published_at"][0] }}</p>
			@endif
		</div>

		<div class="flex items-center gap-3">
			<button class="rounded-md bg-slate-900 px-3 py-2 text-sm font-medium text-white hover:bg-slate-700 dark:bg-slate-100 dark:text-slate-900 dark:hover:bg-slate-300" type="submit">Save</button>
			<a class="text-sm text-slate-500 underline underline-offset-2 dark:text-slate-400" href="/posts/{{ .Form.ID }}">Cancel</a>
		</div>
	</form>
@endsection
