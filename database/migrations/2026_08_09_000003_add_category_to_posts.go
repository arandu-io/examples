package migrations

import (
	"context"

	"github.com/arandu-io/hesape/database/migrations"
)

func init() { migrations.Register(addCategoryToPosts{}) }

// addCategoryToPosts links a post to the section it belongs to.
//
// A migration is immutable once published, so this is a second one rather than
// an edit to createPostsTable: changing an applied id changes nothing on a
// database that already ran it, and leaves two schemas in the world under one
// name.
//
// The column is nullable, and that is the same rule read forwards. During a
// rollout the previous binary is still inserting posts that know nothing about
// categories, and a NOT NULL column with no default fails every one of those
// inserts -- for as long as the two versions overlap, which is the whole
// deploy.
//
// It is also the honest shape: a post without a section is a post nobody has
// filed yet, not an error.
//
// # Why there is no foreign key
//
// SQLite enforces one only when the connection asks it to, and the three engines
// disagree about what a violation reports. The rule this application needs --
// "deleting a section does not delete what was written in it" -- is the service's
// (see CategoryService.Delete), where it can say why rather than answering with a
// constraint name.
type addCategoryToPosts struct{ migrations.BaseMigration }

// Down is found by a type assertion when a rollback runs, so one with the
// wrong signature would apply fine and simply never be reversed. This is the
// line that turns that into a build failure.
var _ migrations.ReversibleMigration = addCategoryToPosts{}

// GetName is this migration's identity. The date prefix carries the order.
func (addCategoryToPosts) GetName() string { return "2026_08_09_000003_add_category_to_posts" }

// Up adds the column, then the index the listing filtered by section reads --
// the query this whole migration exists for, and without it a full scan on the
// one table that grows forever.
func (addCategoryToPosts) Up(ctx context.Context, conn migrations.Connection) error {
	if _, err := conn.Statement(ctx, `ALTER TABLE posts ADD COLUMN category_id VARCHAR(255)`, nil); err != nil {
		return err
	}

	_, err := conn.Statement(ctx, `CREATE INDEX posts_category_idx ON posts (category_id, created_at, id)`, nil)
	return err
}

// Down drops the index first: SQLite refuses to drop a column an index still
// names.
func (addCategoryToPosts) Down(ctx context.Context, conn migrations.Connection) error {
	if _, err := conn.Statement(ctx, `DROP INDEX posts_category_idx`, nil); err != nil {
		return err
	}

	_, err := conn.Statement(ctx, `ALTER TABLE posts DROP COLUMN category_id`, nil)
	return err
}
