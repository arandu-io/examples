//go:build kyse

package auth

import (
	"github.com/arandu-io/kyse/components"

	authui "github.com/arandu-io/examples/app/Http/Controllers/Auth"
)

@go
// VerifyData is what the verification notice draws.
type VerifyData = authui.AuthPage
@endgo

@extends('layouts.app')

@section('content')
	<div class="mx-auto w-full max-w-md">
		<header class="mb-8 text-center">
			<h1 class="headline text-3xl">Check your inbox</h1>
			<p class="text-muted-foreground mt-2 text-sm">
				One link, and the account is yours.
			</p>
		</header>

		{{-- A link that was not valid, or has expired. It is an error about
		     something that already happened, so it is a banner rather than a
		     message under a field: there is no field it belongs to. --}}
		@if(.EmailError != "")
			<div class="mb-6">
				{!! components.Alert(components.AlertProps{Title: .EmailError, Variant: "destructive"}) !!}
			</div>
		@endif

		@if(.Resent)
			<div class="mb-6">
				{!! components.Alert(components.AlertProps{
					Title: "A fresh link is on its way",
					Message: "Check the address you registered with.",
				}) !!}
			</div>
		@endif

		<section class="card">
			<div class="flex flex-col gap-4 px-6 py-6 text-sm">
				<p class="text-muted-foreground">
					Until you follow it you can read everything here. Writing a
					comment is what needs the address confirmed.
				</p>
				<p class="text-muted-foreground">
					In development nothing is actually sent: the whole message,
					link included, is in the output of <code class="font-mono text-xs">aru dev</code>.
				</p>

				<form method="post" action="{{ .VerificationResendURL }}">
					@csrf
					<button type="submit" class="btn" data-variant="outline">Send it again</button>
				</form>
			</div>
		</section>
	</div>
@endsection
