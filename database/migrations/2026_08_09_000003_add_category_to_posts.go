package migrations

import "github.com/arandu-io/framework/kernel"

// addCategoryToPosts links a post to the section it belongs to.
//
// A migration is immutable once published, so this is a second one rather than
// an edit to createPostsTable: changing an applied id changes nothing on a
// database that already ran it, and leaves two schemas in the world under one
// name (RULE 16).
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
var addCategoryToPosts = kernel.Migration{
	ID: "2026_08_09_000003_add_category_to_posts",
	Up: `ALTER TABLE posts ADD COLUMN category_id VARCHAR(255);
	CREATE INDEX posts_category_idx ON posts (category_id, created_at, id);`,
	// The index goes with the column, because the listing filtered by section is
	// the query this whole migration exists for -- and without it that page is a
	// full scan on the one table that grows forever.
	Down: `DROP INDEX posts_category_idx;
	ALTER TABLE posts DROP COLUMN category_id;`,
}
