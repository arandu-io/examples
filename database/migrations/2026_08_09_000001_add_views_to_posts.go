package migrations

import "github.com/arandu-io/framework/kernel"

// addViewsToPosts adds columns to the posts table.
//
// It is unexported and listed in All(), which is what the kernel collects. A
// migration nobody lists is a migration nobody applies, and nothing anywhere
// fails: Go does not report an unused package-level variable, so the schema is
// simply never changed.
//
// Every column is added nullable, with no NOT NULL. During a rollout the
// previous binary is still inserting rows and knows nothing about these columns,
// so a NOT NULL without a default fails on its first insert -- and on every row
// already in the table. Backfill, then tighten it in a later migration.
//
// A migration is immutable once published. Changing this one after it has been
// applied anywhere leaves two schemas in the world under one id -- alter it with
// a new migration instead.
var addViewsToPosts = kernel.Migration{
	ID: "2026_08_09_000001_add_views_to_posts",
	Up: `ALTER TABLE posts ADD COLUMN views INTEGER;
`,
	// Dropping the column drops the index with it on all three engines, which is
	// why there is no DROP INDEX here: its spelling is the one thing SQLite,
	// PostgreSQL and MySQL do not agree about.
	Down: `ALTER TABLE posts DROP COLUMN views;
`,
}
