//go:build kyse

package views

@go
// PostsIndexData is what PostController.Index hands this page.
type PostsIndexData struct {
	// Page is the state the layout draws: the title, the brand, the CSRF token
	// and the navigation. Embedded rather than repeated, and what makes this
	// struct fit the layout.
	Page
	// Posts is the page of records.
	Posts []PostRow
	// NextCursor is the keyset cursor of the following page. It is empty on the
	// last page, and the link is not rendered then.
	NextCursor string
}

// Compile-time proof that this page fits the layout it extends.
var _ Layout = PostsIndexData{}

// PostRow is one record, formatted for display by the controller.
type PostRow struct {
	// ID is what the row links to.
	ID string
	// Title is the Title column.
	Title string
	// Slug is the Slug column.
	Slug string
	// Body is the Body column.
	Body string
	// PublishedAt is the Published at column.
	PublishedAt string
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
		<a class="rounded-md border border-slate-300 px-3 py-2 text-sm font-medium hover:bg-slate-50 dark:border-slate-700 dark:hover:bg-slate-900" href="/posts/create">New post</a>
	</div>

	@if(len(d.Posts) == 0)
	<p class="mt-8 text-sm text-slate-500 dark:text-slate-400">
		No post yet. <a class="underline underline-offset-2" href="/posts/create">Add the first one</a>.
	</p>
	@endif

	@if(len(d.Posts) > 0)
	<div class="mt-8 overflow-x-auto">
		<table class="w-full border-collapse text-left text-sm">
			<thead class="border-b border-slate-200 text-slate-500 dark:border-slate-800 dark:text-slate-400">
				<tr>
					<th class="py-2 pr-4 font-medium">Title</th>
					<th class="py-2 pr-4 font-medium">Slug</th>
					<th class="py-2 pr-4 font-medium">Body</th>
					<th class="py-2 pr-4 font-medium">Published at</th>
					<th class="py-2 font-medium">Created</th>
				</tr>
			</thead>
			<tbody>
				@foreach(.Posts as post)
				<tr class="border-b border-slate-100 dark:border-slate-900">
					<td class="py-2 pr-4">
						<a class="font-medium underline underline-offset-2" href="/posts/{{ post.ID }}">{{ post.Title }}</a>
					</td>
					<td class="py-2 pr-4">{{ post.Slug }}</td>
					<td class="py-2 pr-4">{{ post.Body }}</td>
					<td class="py-2 pr-4">{{ post.PublishedAt }}</td>
					<td class="py-2 text-slate-500 dark:text-slate-400">{{ post.Created }}</td>
				</tr>
				@endforeach
			</tbody>
		</table>
	</div>
	@endif

	@if(d.NextCursor != "")
	<a class="mt-6 inline-block text-sm underline underline-offset-2" href="/posts?cursor={{ .NextCursor }}">Next page</a>
	@endif
@endsection
