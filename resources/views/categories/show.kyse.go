//go:build kyse

package categories

import "github.com/arandu-io/hesape/view"

@go
// CategoriesShowData is what CategoryController.Show hands this page.
type CategoriesShowData struct {
	// Page is the state the layout draws. Its Token is also what the delete
	// button sends as a header: an hx-delete carries no form body, so the
	// hidden field a form uses would never arrive and the request would be
	// refused with 419.
	view.Page
	// Category is the record.
	Category CategoryRow
}

// Compile-time proof that this page fits the layout it extends.
var _ view.Layout = CategoriesShowData{}

// arandu:begin custom
// Anything else this page needs in Go goes here, and survives regeneration.
// arandu:end custom
@endgo

@extends('layouts.app')

@section('content')
	<nav class="text-sm text-slate-500 dark:text-slate-400">
		<a class="underline underline-offset-2 hover:text-slate-900 dark:hover:text-slate-100" href="/categories">Categories</a>
	</nav>

	<div class="mt-2 flex items-center justify-between gap-4">
		<h1 class="text-3xl font-semibold tracking-tight">{{ .Title }}</h1>
		<div class="flex items-center gap-3">
			<a class="rounded-md border border-slate-300 px-3 py-2 text-sm font-medium hover:bg-slate-50 dark:border-slate-700 dark:hover:bg-slate-900" href="/categories/{{ .Category.ID }}/edit">Edit</a>
			<button class="rounded-md border border-red-300 px-3 py-2 text-sm font-medium text-red-700 hover:bg-red-50 dark:border-red-900 dark:text-red-400 dark:hover:bg-red-950" type="button" hx-delete="/categories/{{ .Category.ID }}" hx-headers='{"X-CSRF-Token": "{{ .Token }}"}' hx-confirm="Delete this category?">Delete</button>
		</div>
	</div>

	<dl class="mt-8 divide-y divide-slate-100 border-t border-slate-200 text-sm dark:divide-slate-900 dark:border-slate-800">
				<div class="grid grid-cols-3 gap-4 py-3">
			<dt class="text-slate-500 dark:text-slate-400">Name</dt>
			<dd class="col-span-2">{{ d.Category.Name }}</dd>
		</div>
		<div class="grid grid-cols-3 gap-4 py-3">
			<dt class="text-slate-500 dark:text-slate-400">Slug</dt>
			<dd class="col-span-2">{{ d.Category.Slug }}</dd>
		</div>
		<div class="grid grid-cols-3 gap-4 py-3">
			<dt class="text-slate-500 dark:text-slate-400">Description</dt>
			<dd class="col-span-2">{{ d.Category.Description }}</dd>
		</div>
		<div class="grid grid-cols-3 gap-4 py-3">
			<dt class="text-slate-500 dark:text-slate-400">Created</dt>
			<dd class="col-span-2">{{ .Category.Created }}</dd>
		</div>
	</dl>
@endsection
