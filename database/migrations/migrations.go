// Package migrations holds this application's own schema changes.
//
// The shape is the conventional one: one file per change, applied in order,
// recorded in a table. A migration is a type that says its own name and applies
// itself, so what runs is Go and the statements are handed to the connection one
// at a time. That is what lets a backfill read before it writes -- Connection
// carries Select beside Statement for exactly that.
//
// There is no schema builder. A migration is SQL, written once in the portable
// subset every supported database shares, and what you read is what runs.
// A schema builder exists where an ORM hides the database; here the point
// is that nothing hides it.
//
// Two things every migration answers are answered for it by BaseMigration, and
// none of the ones below overrides either. ShouldRun decides whether a migration
// runs at all, and it is what a feature still switched off, or a change that
// applies to one engine, would say no from -- none of these is that. And
// WithinTransaction asks for a transaction around the change, which is what
// BaseMigration already says: the engines that cannot roll a schema change back
// are handled by the connection, so the only reason to opt out is a statement an
// engine refuses inside a transaction, and none of these is that either.
//
// A migration never runs at boot. `aru migrate` is a pipeline step: with N
// replicas starting together, N migrations race. Every migration is also
// compatible with the previous version of the binary during a rollout -- a new
// column is nullable or has a default, and dropping one takes two releases.
package migrations

import "github.com/arandu-io/framework/kernel"

// All returns this application's migrations, in the order they are applied.
//
// It starts empty, and that is correct: the users table comes from the auth
// module, the outbox from events and the jobs table from queue. The kernel
// collects those from the modules themselves, so repeating them here would apply
// each one twice.
//
// `aru make:module` appends to the list. Adding one by hand means adding the
// file next to this one and its type below.
func All() []kernel.Migration {
	return []kernel.Migration{
		// arandu:begin custom
		createPostsTable{},
		createCommentsTable{},
		addViewsToPosts{},
		backfillPostViews{},
		createCategoriesTable{},
		addCategoryToPosts{},
		// arandu:end custom
	}
}
