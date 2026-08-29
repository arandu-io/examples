//go:build kyse

package categories

import "github.com/arandu-io/hesape/view"

@go
// CategoriesIndexData is what CategoryController.Index hands this page.
type CategoriesIndexData struct {
	// view.Page is the chrome the layout draws: the title, the description, the
	// CSRF token and the navigation. Embedded rather than repeated, and what
	// makes this struct fit the layout.
	view.Page
	// Categories is the page of records.
	Categories []CategoryRow
	// NextCursor is the keyset cursor of the following page. It is empty on the
	// last page, and the link is not rendered then.
	NextCursor string
}

// Compile-time proof that this page fits the layout it extends.
var _ view.Layout = CategoriesIndexData{}

// CategoryRow is one record, formatted for display by the controller.
type CategoryRow struct {
	// ID is what the row links to.
	ID string
	// Name is the Name column.
	Name string
	// Slug is the Slug column.
	Slug string
	// Description is the Description column.
	Description string
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
		<a class="rounded-md border border-slate-300 px-3 py-2 text-sm font-medium hover:bg-slate-50 dark:border-slate-700 dark:hover:bg-slate-900" href="/categories/create">New category</a>
	</div>

	@if(len(d.Categories) == 0)
		<p class="mt-8 text-sm text-slate-500 dark:text-slate-400">
			No category yet. <a class="underline underline-offset-2" href="/categories/create">Add the first one</a>.
		</p>
	@endif

	@if(len(d.Categories) > 0)
		<div class="mt-8 overflow-x-auto">
			<table class="w-full border-collapse text-left text-sm">
				<thead class="border-b border-slate-200 text-slate-500 dark:border-slate-800 dark:text-slate-400">
					<tr>
						<th class="py-2 pr-4 font-medium">Name</th>
						<th class="py-2 pr-4 font-medium">Slug</th>
						<th class="py-2 pr-4 font-medium">Description</th>
						<th class="py-2 font-medium">Created</th>
					</tr>
				</thead>
				<tbody>
					@foreach(.Categories as category)
						<tr class="border-b border-slate-100 dark:border-slate-900">
							<td class="py-2 pr-4">
								<a class="font-medium underline underline-offset-2" href="/categories/{{ category.ID }}">{{ category.Name }}</a>
							</td>
												<td class="py-2 pr-4">{{ category.Slug }}</td>
					<td class="py-2 pr-4">{{ category.Description }}</td>
							<td class="py-2 text-slate-500 dark:text-slate-400">{{ category.Created }}</td>
						</tr>
					@endforeach
				</tbody>
			</table>
		</div>
	@endif

	@if(d.NextCursor != "")
		<a class="mt-6 inline-block text-sm underline underline-offset-2" href="/categories?cursor={{ .NextCursor }}">Next page</a>
	@endif
@endsection
