package models

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/security"
	"github.com/arandu-io/hesape/database/model"
)

// Category is the entity: the columns of one section, over the row core every
// entity embeds.
//
// It has no persistence methods of its own: this is not Active Record. The
// model.Model it embeds promotes Save, Delete and Load onto it, and each of them
// takes a security.Grant, so nothing reaches the table without a Policy's
// answer. The table is reached through Categories, generated beside this file in
// CategoryQuery.go, and every terminal a query ends in -- Find and Get included
// -- takes a Grant too. The model is data; the Policy is the door.
//
// # Why it is still handed around by value
//
// security.Policy[Category] takes the entity by value, and so does every
// service, request and view below it. The embedded model does not change that,
// and it is part of why that is safe:
//
//   - model.Model is one pointer, so Category stays comparable and a copy costs
//     a pointer, not a row.
//   - a value copy holds the model of the row it was copied from, and every
//     statement through it -- Save, Update, Delete, Load -- answers
//     model.ErrUnwired instead of reaching that row. A view struct built from a
//     copied Category holds a value that cannot write, which is the shape this
//     tree exists to keep.
//   - the one value that can write is the row a terminal handed back, and the
//     services keep it inside the function that spent the Grant on it. See
//     entities in CategoryService.
//
// The cost is exact and it is not hidden: a copy cannot load a relation either,
// so this application never calls With on a section query and hands the copy
// back. What it calls instead is PostsIn, which builds the section as a row of
// its own for the one load it needs.
type Category struct {
	model.Model

	ID string `db:"id"`

	// TenantID is whose section this is. It is written from the Grant and never
	// from a request, and it is not a mutable field: moving a row between
	// tenants is not an update.
	TenantID string `db:"tenant_id"`

	Name        string    `db:"name"`
	Slug        string    `db:"slug"`
	Description string    `db:"description"`
	CreatedAt   time.Time `db:"created_at"`
}

// categoryTable is the table Category is a row of: the declaration every
// statement about a section is built from. Its query, Categories, is generated
// beside it by aru model:build.
//
// The spec names the two places this table is not the default one, and each
// is a line below rather than a convention somebody has to remember. The key is
// a text identifier the application writes, so it neither increments nor is
// drawn by the model: ManualKey. The table has no updated_at column, so nothing
// may try to write one -- model.NoColumn keeps the update stamp off both the
// insert and the update, while created_at is still stamped on insert. The tenant
// column is left at its default, tenant_id, because the only thing worth saying
// out loud about the scoping is turning it off, and nothing here does.
//
// Categories takes the handle a constructor was given. There is no adapter and
// no second handle: *data.DB carries the five verbs a model connection needs
// plus the grammar and the processor, so a statement built here runs on the
// same pool, joins the same open transaction, and is recorded on the same
// Collector as one the repositories next door issue.
var categoryTable = model.NewTable(model.TableSpec{
	Name:            "categories",
	New:             func() model.Entity { return new(Category) },
	ManualKey:       true,
	UpdatedAtColumn: model.NoColumn,
	// arandu:begin custom
	// arandu:end custom
})

// init registers the has-many from one section to the articles filed under
// it, posts.category_id pointing at categories.id, under the name "posts".
//
// The keys are not named. An empty foreign key is the conventional one --
// category_id, from a table of Category rows -- and an empty local key is the
// parent's own, so writing either would be repeating what the convention
// already answers, in a place that can then disagree with it.
//
// It is registered here rather than in the table's spec because it names
// postTable, and two table variables whose initializers named each other would
// be an initialization cycle. The function runs only when a load asks for the
// relation.
func init() {
	categoryTable.Relate("posts", func(section *model.Model) model.Relation {
		return model.HasMany(section, postTable, "", "")
	})
}

// PostsIn loads the section's articles and reads them back as entities.
//
// # Why it starts from a row of its own
//
// The services hand a Category around by value -- see the type comment -- and a
// value copy holds the model of the row it was copied from, so Load refuses it
// with model.ErrUnwired. PostsIn therefore builds the section as a row of its
// own, on the connection it was given, from the two columns that say which
// section it is: the key the relation narrows by and the tenant the section
// belongs to. Nothing is inserted; the row exists to be the parent of one load,
// and a parent with an empty key loads no articles rather than every article
// there is.
//
// The child query is scoped by the posts table's own tenant filter, from the
// Grant handed to Load, so a section id cannot reach across tenants even though
// posts.category_id is a plain column with no tenant beside it -- which is
// exactly the row the tenant suite writes on purpose.
//
// It is a function rather than a method for the same reason: what a caller
// holds is a value copy, and a method on it could load nothing.
func PostsIn(ctx context.Context, g security.Grant, db *data.DB, ca Category) ([]Post, error) {
	section, err := Categories(db).New()
	if err != nil {
		return nil, err
	}
	section.ID = ca.ID
	section.TenantID = ca.TenantID

	// The Grant is on the load, as it is on every other read in this
	// application: the relation was registered without one, because declaring
	// a sentence authorizes nothing and only the statement that runs does.
	if err := section.Load(ctx, g, "posts"); err != nil {
		return nil, err
	}

	// Related hands the loaded rows back as rows of any table, and
	// PostCollectionOf, generated beside Post, turns them into the entity. It
	// reports false for a relation that holds something other than rows, which
	// cannot happen here and is not worth a panic: a relation that came back
	// holding something else is a relation that loaded nothing this caller can
	// use.
	rows, ok := section.Related("posts")
	if !ok {
		return nil, ErrCategoryPosts
	}
	filed := PostCollectionOf(rows)
	out := make([]Post, 0, len(filed))
	for _, post := range filed {
		out = append(out, *post)
	}
	return out, nil
}

// What can go wrong with a category, declared beside the entity rather than
// inside the repository.
//
// The controller maps them to a status code and the repository returns them, so
// both need to name them -- and a controller that imported the repository to
// reach an error would be a controller with a door to the data layer, which is
// the one thing the tree exists to prevent.
var (
	// ErrCategoryNotFound is returned when no row matches.
	ErrCategoryNotFound = errors.New("category: not found")
	// ErrCategoryConflict is a unique constraint refusing a duplicate.
	ErrCategoryConflict = errors.New("category: already exists")
	// ErrCategorySort is an ordering the allowlist does not contain.
	ErrCategorySort = errors.New("category: sort field not allowed")
	// ErrCategoryNotEmpty refuses to delete a section that still holds
	// articles. The policy says the same thing in prose and cannot enforce it:
	// the count is not on the entity, so the rule that reads it lives one layer
	// down, where a query is allowed.
	ErrCategoryNotEmpty = errors.New("category: still holds posts")
	// ErrCategoryPosts is a section relation that a load did not leave as rows:
	// the relation is not the one registered on the table, or it holds
	// something other than rows of a table. It is a wiring mistake rather than a
	// runtime condition, and it is an error rather than a panic because a read
	// path answering with nothing is always better than a read path taking the
	// process down.
	ErrCategoryPosts = errors.New("category: the posts relation did not load as rows")
)

// LogValue implements slog.LogValuer, so passing the whole entity to a log call
// records the identifiers and nothing else. Add any sensitive field to the
// custom block below and it stays out of logs, dumps and the debug page.
func (ca Category) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("id", ca.ID),
		slog.String("tenant", ca.TenantID),
	)
}

// arandu:begin custom
// MarshalJSON writes every field explicitly. No field reaches the wire without
// being named here: a column added to the struct and left out below is private
// by omission.
func (ca Category) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		ID          string    `json:"id"`
		TenantID    string    `json:"tenant_id"`
		Name        string    `json:"name"`
		Slug        string    `json:"slug"`
		Description string    `json:"description"`
		CreatedAt   time.Time `json:"created_at"`
	}{
		ID: ca.ID, TenantID: ca.TenantID, Name: ca.Name, Slug: ca.Slug,
		Description: ca.Description, CreatedAt: ca.CreatedAt,
	})
}

// arandu:end custom
