// Package migrations holds this application's own schema changes.
//
// One file per change and one type per file. The type embeds
// migrations.BaseMigration, says its own name, writes Up, and registers itself
// from init().
//
// The name carries the order: the registry sorts by that string and nothing
// else. It is fixed the moment the migration has run against any database,
// because a migration that changes its name is a migration that runs twice.
//
// Registering is what makes a migration reachable, and the blank import of this
// package in bootstrap/app.go is what makes the registering happen. A package
// nothing imports is not in the binary at all.
//
// There is no schema builder. A migration is SQL, written once in the portable
// subset every supported database shares, and what you read is what runs.
// A schema builder exists where an ORM hides the database; here the point
// is that nothing hides it.
//
// The connection is handed to Up rather than being a string the migration
// returns, so a migration that has to read before it writes -- a backfill, a
// check that a column is empty before it is dropped -- calls Select and then
// Statement, with no second kind of migration for the case.
//
// Two things every migration answers are answered for it by BaseMigration, and
// none of the ones here overrides either. ShouldRun decides whether a migration
// runs at all, and it is what a feature still switched off, or a change that
// applies to one engine, would say no from -- none of these is that. And
// WithinTransaction asks for a transaction around the change, which is what
// BaseMigration already says: the engines that cannot roll a schema change back
// are handled by the connection, so the only reason to opt out is a statement an
// engine refuses inside a transaction, and none of these is that either.
//
// The users table is not here, and neither is the outbox nor the jobs table:
// they come from the auth, events and queue modules, each of which registers
// its own. Repeating them here would apply each one twice.
//
// A migration never runs at boot. `aru migrate` is a pipeline step: with N
// replicas starting together, N migrations race. Every migration is also
// compatible with the previous version of the binary during a rollout -- a new
// column is nullable or has a default, and dropping one takes two releases.
package migrations
