package migrations

import (
	"context"

	"github.com/arandu-io/hesape/database/migrations"
)

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
//
// # Why it writes without reading
//
// A backfill is the migration that may read before it writes, and Connection
// carries Select for exactly that. This one does not use it, because the value
// it writes is the constant zero: a Select here would fetch the ids whose views
// are NULL only to hand them back one at a time to the statement that already
// finds them itself, turning one set the engine closes atomically into as many
// round trips as there are rows, over a table another connection is still
// writing to.
//
// Select earns its place when the new value cannot be spelled in the statement
// that writes it -- a column derived from another table's rows, a re-encoding
// Go does and SQL does not. Zero is not that.
//
// # Why there is no Down
//
// A migration is reversed only if it has one: the rollback tests for Down and
// passes over the migration that does not declare it, which is not an error and
// not a wall in front of the migrations underneath.
//
// There is nothing to undo here. Putting the NULLs back would restore the
// defect, and zero is a correct value under the schema before this migration and
// after it: the column was nullable either way, and the rows this touched were
// rows nobody had ever written a count to.
type backfillPostViews struct{ migrations.BaseMigration }

// GetName is this migration's identity. The date prefix carries the order.
func (backfillPostViews) GetName() string { return "2026_08_10_000001_backfill_post_views" }

// Up sets the count on the rows that never had one.
func (backfillPostViews) Up(ctx context.Context, conn migrations.Connection) error {
	_, err := conn.Statement(ctx, `UPDATE posts SET views = 0 WHERE views IS NULL`, nil)
	return err
}
