//go:build kyse

package auth

import (
	"github.com/arandu-io/kyse/components"
	"github.com/arandu-io/kyse/icons"

	authui "github.com/arandu-io/examples/app/Http/Controllers/Auth"
)

@go
// LoginData is what the sign-in screen draws.
type LoginData = authui.AuthPage
@endgo

@extends('layouts.app')

@section('content')
	<div class="mx-auto w-full max-w-md">
		<header class="mb-8 text-center">
			<span class="auth-mark">{!! icons.SignIn(icons.Props{}) !!}</span>
			<h1 class="headline mt-4 text-3xl">Sign in</h1>
			<p class="text-muted-foreground mt-2 text-sm">To write, to comment, or to moderate.</p>
		</header>

		{{-- The one-shot message a redirect left behind: a confirmed address, a
		     password just changed. It is above the form because it is about what
		     already happened, not about what to type. --}}
		@if(.Status != "")
			<div class="mb-6">
				{!! components.Alert(components.AlertProps{Title: .Status}) !!}
			</div>
		@endif

		<section class="card">
			{{-- hx-target and hx-swap on the form itself: a rejected login answers
			     422 with this form and its messages, and the swap puts it back
			     where it was without a page load. --}}
			<form class="flex flex-col gap-4 px-6 py-6" method="post" action="{{ .LoginURL }}"
				hx-post="{{ .LoginURL }}" hx-target="this" hx-swap="outerHTML">
				@csrf

				{!! components.Field(components.FieldProps{
					Name: "email", Label: "Email", Type: "email",
					Value: .Email, Error: .EmailError,
					Autocomplete: "username", Required: true, Autofocus: true,
				}) !!}

				{!! components.Field(components.FieldProps{
					Name: "password", Label: "Password", Type: "password",
					Error: .PasswordError,
					Autocomplete: "current-password", Required: true,
				}) !!}

				<label class="flex items-center gap-2 text-sm">
					<input class="input" type="checkbox" name="remember" value="1" {{ .RememberAttribute() }}>
					Remember me
				</label>

				<div class="flex items-center justify-between gap-3">
					<button type="submit" class="btn">Sign in</button>
					@if(.HasPasswordReset)
						<a class="text-muted-foreground text-sm hover:underline" href="{{ .PasswordRequestURL }}">Forgot your password?</a>
					@endif
				</div>
			</form>
		</section>

		@if(.RegisterURL != "")
			<p class="text-muted-foreground mt-6 text-center text-sm">
				No account? <a class="text-foreground hover:underline" href="{{ .RegisterURL }}">Create one</a>.
			</p>
		@endif
	</div>
@endsection
