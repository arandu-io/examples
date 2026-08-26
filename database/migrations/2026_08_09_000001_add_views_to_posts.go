package migrations

import (
	"context"

	"github.com/arandu-io/hesape/database/migrations"
	"github.com/arandu-io/hesape/database/schema"
)

func init() { migrations.Register(addViewsToPosts{}) }

// addViewsToPosts adds columns to the posts table.
//
// It is unexported and listed in All(), which is what the kernel collects. A
// migration nobody lists is a migration nobody applies, and nothing anywhere
// fails: Go does not report an unused package-level type, so the schema is
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
type addViewsToPosts struct{ migrations.BaseMigration }

// Down is found by a type assertion when a rollback runs, so one with the
// wrong signature would apply fine and simply never be reversed. This is the
// line that turns that into a build failure.
var _ migrations.ReversibleMigration = addViewsToPosts{}

// GetName is this migration's identity. The date prefix carries the order.
func (addViewsToPosts) GetName() string { return "2026_08_09_000001_add_views_to_posts" }

// Up adds the column.
func (addViewsToPosts) Up(ctx context.Context, conn migrations.Connection) error {
	return conn.Schema().Table(ctx, "posts", func(table *schema.Blueprint) {
		// Nullable, and that is the rollout rule rather than a preference: a NOT
		// NULL column added to a table that has rows fails on every row already
		// there, and during a rollout the previous binary does not fill it in.
		table.BigInteger("views").Nullable()
	})
}

// Down drops the column, and the index with it on all three engines -- which is
// why there is no index named here: the spelling of a drop is the one thing
// SQLite, PostgreSQL and MySQL do not agree about, and it is the grammar's to
// know.
func (addViewsToPosts) Down(ctx context.Context, conn migrations.Connection) error {
	return conn.Schema().Table(ctx, "posts", func(table *schema.Blueprint) {
		table.DropColumn("views")
	})
}
