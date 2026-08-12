//go:build kyse

package auth

import (
	"github.com/arandu-io/kyse/components"
	"github.com/arandu-io/kyse/icons"

	authui "github.com/arandu-io/examples/app/Http/Controllers/Auth"
)

@go
// RegisterData is what the sign-up screen draws.
type RegisterData = authui.AuthPage
@endgo

@extends('layouts.app')

@section('content')
	<div class="mx-auto w-full max-w-md">
		<header class="mb-8 text-center">
			<span class="auth-mark">{!! icons.UserPlus(icons.Props{}) !!}</span>
			<h1 class="headline headline-lg mt-4">Create an account</h1>
			<p class="text-muted-foreground mt-2 text-sm">
				Reading needs nothing. Commenting needs a confirmed address.
			</p>
		</header>

		<section class="card">
			<form class="flex flex-col gap-4 px-6 py-6" method="post" action="{{ .RegisterURL }}">
				@csrf

				{!! components.Field(components.FieldProps{
					Name: "name", Label: "Name",
					Value: .Name, Page: .,
					Hint: "What your comments are signed with.",
					Autocomplete: "name", Required: true, Autofocus: true,
				}) !!}

				{!! components.Field(components.FieldProps{
					Name: "email", Label: "Email", Type: "email",
					Value: .Email, Page: .,
					Hint: "We send one link here and nothing else.",
					Autocomplete: "email", Required: true,
				}) !!}

				{!! components.Field(components.FieldProps{
					Name: "password", Label: "Password", Type: "password",
					Page: .,
					Hint: "At least twelve characters.",
					Autocomplete: "new-password", Required: true,
				}) !!}

				{!! components.Field(components.FieldProps{
					Name: "password_confirmation", Label: "Confirm password", Type: "password",
					Page: .,
					Autocomplete: "new-password", Required: true,
				}) !!}

				<div>
					<button type="submit" class="btn">Create the account</button>
				</div>
			</form>
		</section>

		<p class="text-muted-foreground mt-6 text-center text-sm">
			Already registered? <a class="text-foreground hover:underline" href="{{ .LoginURL }}">Sign in</a>.
		</p>
	</div>
@endsection
