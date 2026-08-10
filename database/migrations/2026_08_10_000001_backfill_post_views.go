package migrations

import "github.com/arandu-io/framework/kernel"

// backfillPostViews gives every post a view count of zero.
//
// 2026_08_09_000001 added the column nullable with no DEFAULT, and the scan in
// PostRepository read it straight into an int -- so every row written before it,
// and every row the previous binary inserted during the rollout, was unreadable:
// "converting NULL to int is unsupported", on every read of the table.
//
// That migration is not edited, and this one exists for that reason. A migration
// is immutable once it has been applied anywhere: changing it would leave two
// schemas in the world under one id, and the replica that already ran the old
// text would never run the new one.
//
// The DEFAULT is not set here on purpose. SQLite cannot alter a column's
// default, so a portable statement for it does not exist -- and the durable half
// of the fix is the one in the scan, which now reads through sql.NullInt64 the
// way the category column beside it always did. This migration closes the rows
// that are already wrong; the scan closes the window where new ones appear.
var backfillPostViews = kernel.Migration{
	ID: "2026_08_10_000001_backfill_post_views",
	Up: `UPDATE posts SET views = 0 WHERE views IS NULL;
`,
	// A no-op, and the kernel requires something rather than nothing -- a
	// migration with an empty Down cannot be rolled back at all, which would make
	// this one a wall in front of every rollback past it.
	//
	// There is nothing to undo. Putting the NULLs back would restore the defect,
	// and zero is a correct value under the schema before this migration and
	// after it: the column was nullable either way, and the rows this touched
	// were rows nobody had ever written a count to.
	Down: `SELECT 1;
`,
}
