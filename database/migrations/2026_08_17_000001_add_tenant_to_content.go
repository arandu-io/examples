package migrations

import (
	"context"

	"github.com/arandu-io/hesape/database/migrations"
)

func init() { migrations.Register(addTenantToContent{}) }

// addTenantToContent scopes the three content tables by tenant.
//
// A migration is immutable once published, so this is a fourth one rather than
// an edit to the three CREATE TABLEs: changing an applied id changes nothing on
// a database that already ran it, and leaves two schemas in the world under one
// name.
//
// # Why a single-tenant blog carries the column
//
// This example serves one tenant, and that is a constant in the configuration
// rather than a shape of the schema. There is no second code path for it: the
// same statements run, the same Grant scopes them, and growing into a second
// tenant changes a resolver and not one query. A table without the column is a
// table whose queries cannot be scoped at all, and adding the scope later means
// revisiting every statement that reads it -- which is the migration nobody
// finishes.
//
// # Why the column is nullable
//
// During a rollout the previous binary is still inserting rows and knows nothing
// about this column, so NOT NULL with no default fails every one of those
// inserts -- for as long as the two versions overlap, which is the whole deploy.
//
// There is no backfill beside it, and that is the honest answer rather than a
// missing step. The tenant is not a value SQL can know: it lives on the Grant,
// a migration runs once for every tenant at once, and a statement that invented
// a value here would file somebody's rows under somebody else. A row with no
// tenant is a row from before the column, and it is readable by nobody -- which
// is the direction to fail in.
//
// # Why the index leads with the tenant
//
// Every statement that reads these tables now opens with `tenant_id = ?`, so
// the tenant is the first column of the key. An index that led with created_at
// would be scanned and filtered instead of sought, on every listing, and the
// cost grows with the other tenants rather than with your own rows.
type addTenantToContent struct{ migrations.BaseMigration }

// Down is found by a type assertion when a rollback runs, so one with the
// wrong signature would apply fine and simply never be reversed. This is the
// line that turns that into a build failure.
var _ migrations.ReversibleMigration = addTenantToContent{}

// GetName is this migration's identity. The date prefix carries the order.
func (addTenantToContent) GetName() string { return "2026_08_17_000001_add_tenant_to_content" }

// Up adds the column to each table, then the index each listing reads.
//
// The three tables are spelled out one statement per line rather than looped
// over a slice of table names: a statement assembled by concatenation is the one
// place a migration could carry an injection, and a reader should not have to
// check that the parts are constants.
func (addTenantToContent) Up(ctx context.Context, conn migrations.Connection) error {
	for _, statement := range []string{
		`ALTER TABLE posts ADD COLUMN tenant_id VARCHAR(255)`,
		`ALTER TABLE comments ADD COLUMN tenant_id VARCHAR(255)`,
		`ALTER TABLE categories ADD COLUMN tenant_id VARCHAR(255)`,
		`CREATE INDEX posts_tenant_created_idx ON posts (tenant_id, created_at, id)`,
		`CREATE INDEX comments_tenant_created_idx ON comments (tenant_id, created_at, id)`,
		`CREATE INDEX categories_tenant_created_idx ON categories (tenant_id, created_at, id)`,
	} {
		if _, err := conn.Statement(ctx, statement, nil); err != nil {
			return err
		}
	}
	return nil
}

// Down drops the indexes first. SQLite refuses to drop a column an index still
// names, so dropping the columns alone would fail on the one engine this
// example runs on out of the box.
func (addTenantToContent) Down(ctx context.Context, conn migrations.Connection) error {
	for _, statement := range []string{
		`DROP INDEX posts_tenant_created_idx`,
		`DROP INDEX comments_tenant_created_idx`,
		`DROP INDEX categories_tenant_created_idx`,
		`ALTER TABLE posts DROP COLUMN tenant_id`,
		`ALTER TABLE comments DROP COLUMN tenant_id`,
		`ALTER TABLE categories DROP COLUMN tenant_id`,
	} {
		if _, err := conn.Statement(ctx, statement, nil); err != nil {
			return err
		}
	}
	return nil
}
