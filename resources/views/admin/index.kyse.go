//go:build kyse

package admin

@go
// IndexData is the moderation dashboard: what is waiting, and what there is.
//
// The counts arrive as numbers rather than as the records they were counted
// from. A screen that receives two hundred rows to print the number 200 is a
// screen that gets slower every week.
type IndexData struct {
	Chrome

	// Published and Drafts are how many posts are in each state.
	Published int
	Drafts    int
	// Comments is the total, and Waiting is how many of them are not public
	// yet. Waiting is the only one that is a job to do.
	Comments int
	Waiting  int
}
@endgo

@extends('admin.layout')

@section('content')
	<header>
		<h1 class="text-2xl font-semibold tracking-tight">Dashboard</h1>
		<p class="text-muted-foreground mt-1 text-sm">What is here, and what is waiting for you.</p>
	</header>

	<dl class="mt-8 grid gap-4 sm:grid-cols-2">
		<div class="card p-5">
			<dt class="text-muted-foreground text-sm">Published</dt>
			<dd class="mt-1 text-3xl font-semibold tabular-nums">{{ .Published }}</dd>
		</div>
		<div class="card p-5">
			<dt class="text-muted-foreground text-sm">Drafts</dt>
			<dd class="mt-1 text-3xl font-semibold tabular-nums">{{ .Drafts }}</dd>
		</div>
		<div class="card p-5">
			<dt class="text-muted-foreground text-sm">Comments</dt>
			<dd class="mt-1 text-3xl font-semibold tabular-nums">{{ .Comments }}</dd>
		</div>

		{{-- The one card that is a job rather than a number. It is styled by the
		     count: nothing waiting is not a warning. --}}
		<div class="card p-5">
			<dt class="text-muted-foreground text-sm">Awaiting review</dt>
			<dd class="mt-1 text-3xl font-semibold tabular-nums">{{ .Waiting }}</dd>
			@if(.Waiting > 0)
				<a class="btn mt-4" data-size="sm" href="{{ .CommentsURL() }}">Review them</a>
			@endif
		</div>
	</dl>
@endsection
