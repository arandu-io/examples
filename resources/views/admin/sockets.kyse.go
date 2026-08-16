//go:build kyse

package admin

import "github.com/arandu-io/kyse/components"

@go
// SocketsData is what the socket server is holding right now, as the operator's
// screen draws it.
//
// The numbers arrive already formatted, as strings, because that is what
// components.StatCard takes and why it takes it: a thousands separator is a
// local decision, and a component that made it would make it wrong in half the
// places it is used. What this view decides is the wording around them.
//
// There is no polling and no refresh here. The page is a reading taken when it
// was requested, which is what ReadAt says out loud -- a card that quietly
// updated itself would be a card nobody can trust the timestamp of.
type SocketsData struct {
	Chrome

	// Tenants is one row per tenant this process holds sockets for: how many
	// are open, and how many channels are alive.
	Tenants []components.StatRow

	// Messages is exactly one row, and it is a process total rather than a
	// tenant's. The controller says why; the Meta line below says the same
	// thing to whoever is reading the screen.
	Messages []components.StatRow

	// ReadAt is when the numbers were taken, spelled the way every other date in
	// this application is.
	ReadAt string
}
@endgo

@extends('admin.layout')

@section('content')
	<header>
		<h1 class="text-2xl font-semibold tracking-tight">Sockets</h1>
		<p class="text-muted-foreground mt-1 text-sm">What the realtime server is holding in this process, across every tenant in it.</p>
	</header>

	<div class="mt-8 flex flex-col gap-4">
		{!! components.StatCard(components.StatCardProps{
			Title:   "Connections",
			Meta:    "Open sockets and live channels, per tenant, read at " + .ReadAt + ".",
			Columns: []string{"Open", "Channels"},
			Rows:    .Tenants,
			Empty: components.EmptyProps{
				Title:   "Nothing is connected",
				Message: "No socket has been opened on this process since it started.",
			},
		}) !!}

		{!! components.StatCard(components.StatCardProps{
			Title:   "Messages",
			Meta:    "Frames since this process started. One line, because the two events that carry a frame carry a socket and no tenant.",
			Columns: []string{"Received", "Sent"},
			Rows:    .Messages,
			Empty: components.EmptyProps{
				Title:   "No frames yet",
				Message: "Nothing has crossed this process since it started.",
			},
		}) !!}
	</div>
@endsection
