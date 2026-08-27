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
	"github.com/arandu-io/hesape/database/model/relations"
)

// Category is the entity, and it is a plain struct on purpose.
//
// It has no persistence methods: this is not Active Record, and a type that can
// save itself can save itself from anywhere. There is no Find, no Save and no
// query builder on this type. The table is reached through Categories below,
// and every terminal a query ends in -- Find and Get included -- takes a
// security.Grant. The model is data; the Policy is the door.
//
// # Why it does not embed model.Model[Category], and what that costs
//
// The model layer works over either shape. A T that embeds model.Model[T]
// carries its model inside itself, so a row a terminal hands back can answer for
// itself: Save, Load and model.Related are reachable from the value. A T that
// does not embed one -- this one -- is columns and nothing else.
//
// This one does not, for three reasons that are about this application rather
// than about taste:
//
//   - security.Policy[Category] takes the entity by value, and so does every
//     service, request and view below it. model.Model[Category] holds maps, so
//     embedding it makes Category uncomparable, and every one of those copies
//     would carry a back pointer aimed at the value it was copied from. One row
//     would have two identities and the second one saves nothing.
//   - the embedded model promotes Save, Delete and Fill onto the entity, and a
//     view struct is built from this type. A template holding a value that can
//     write is the shape this tree exists to prevent.
//   - the entity is what a Policy is asked about. It should carry the columns a
//     rule reads and no machinery a rule could reach.
//
// The cost is exact and it is not hidden: model.Related and Model.Load read a
// relation off the row, and there is no row to read it off here, so this
// application never calls Builder.With -- an eager load would attach its
// matches to a model no terminal hands back. What it calls instead is PostsOf,
// which is the relation itself: it is loaded under a Grant and read back
// through model.Unref, which unwraps the rows the relation returned into the
// entities they carry. See PostsOf.
type Category struct {
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

// Categories is the door to the categories table: the model every statement
// about a section is built from.
//
// It is model.NewModel rather than model.Query because this table is not the
// default model in three places, and each of them is a line below rather than a
// convention somebody has to remember. The key is a text identifier the
// application generates, so it does not increment. The table has no updated_at
// column, so nothing may try to write one -- an empty UpdatedAtColumn is what
// keeps both the insert stamp and the update stamp off it, while created_at is
// still stamped on insert. The tenant column is left at its default, tenant_id,
// because the only thing worth saying out loud about the scoping is turning it
// off, and nothing here does.
//
// It takes the handle a constructor was given. There is no adapter and no
// second handle: *data.DB carries the five verbs a model connection needs plus
// the grammar and the processor, so a statement built here runs on the same
// pool, joins the same open transaction, and is recorded on the same Collector
// as one the repositories next door issue.
func Categories(db *data.DB) *model.Model[Category] {
	m := model.NewModel[Category]("categories", db, db.GetQueryGrammar(), db.GetPostProcessor())
	m.KeyType = "string"
	m.Incrementing = false
	m.UpdatedAtColumn = ""
	return m
}

// PostsOf is the has-many from one section to the articles filed under it:
// posts.category_id pointing at categories.id.
//
// The keys are not named. An empty foreign key is the conventional one --
// category_id, from a model over Category -- and an empty local key is the
// parent's own, so writing either would be repeating what the convention
// already answers, in a place that can then disagree with it.
//
// # Reading it back
//
// The relation is loaded by calling Get on what this returns, with the Grant:
//
//	rel, err := models.PostsOf(db, section)
//	rows, err := rel.Get(ctx, g)
//
// and rows are the narrow interface the relation tree passes around, not the
// entity. PostsIn below is the read back: model.Unref turns each of them into
// the typed model it really is, and the entity is the model's own. That is the
// path a plain entity has -- see the type comment for why this one is plain.
//
// The parent is built from the section that was already read, and it carries
// its tenant with it. The child query is scoped by the posts model's own tenant
// filter, from the Grant handed to Get, so a section id cannot reach across
// tenants even though posts.category_id is a plain column with no tenant beside
// it -- which is exactly the row the tenant suite writes on purpose.
func PostsOf(db *data.DB, ca Category) (*relations.HasMany, error) {
	// NewFromBuilder is the section as a row that already exists, which is what
	// a relation needs to narrow itself: a parent with no key answers with an
	// empty slice rather than with every post there is.
	parent, err := Categories(db).NewFromBuilder(map[string]any{
		"id":        ca.ID,
		"tenant_id": ca.TenantID,
	})
	if err != nil {
		return nil, err
	}
	return model.HasManyOf(parent, postsModel(db), "", ""), nil
}

// PostsIn loads the section's articles and reads them back as entities.
//
// It is the second half of PostsOf, and it is a function rather than a method
// because the row it starts from is a plain struct with nothing to hang one on.
// A caller that wants the relation narrowed further takes PostsOf and adds to
// it; this is the whole of it, which is what the one caller needs.
func PostsIn(ctx context.Context, g security.Grant, db *data.DB, ca Category) ([]Post, error) {
	rel, err := PostsOf(db, ca)
	if err != nil {
		return nil, err
	}
	// The Grant is on the terminal, as it is on every other read in this
	// application: the relation was built without one, because building a
	// sentence authorizes nothing and only the statement that runs does.
	rows, err := rel.Get(ctx, g)
	if err != nil {
		return nil, err
	}

	out := make([]Post, 0, len(rows))
	for _, row := range rows {
		// The relation tree holds models through a narrow interface, so the way
		// back to the typed one is model.Unref. It answers false for a row of
		// another entity, which cannot happen here and is not worth a panic:
		// a relation that came back holding something else is a relation that
		// loaded nothing this caller can use.
		found, ok := model.Unref[Post](row)
		if !ok {
			return nil, ErrCategoryPosts
		}
		out = append(out, *found.Entity)
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
	// ErrCategoryPosts is a section relation that came back holding rows of
	// another entity. It is a wiring mistake rather than a runtime condition,
	// and it is an error rather than a panic because a read path answering with
	// nothing is always better than a read path taking the process down.
	ErrCategoryPosts = errors.New("category: the posts relation loaded rows of another entity")
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
