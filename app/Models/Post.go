package models

import (
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/arandu-io/hesape/database/model"
)

// Post is the entity. It has no persistence methods of its own: this is not
// Active Record.
//
// The table is reached by PostRepository, and every method of a repository --
// Find and List included -- takes a security.Grant that only a Policy can
// issue. The model is data; the Policy is the door.
//
// # The embedded model and the db tags, and what they do not turn this into
//
// The tags name the column each field stands for, and model.Model is the row
// core a table's rows embed. Both are here for one statement: the section
// relation, whose related side is postTable -- see PostsIn. The rows that
// relation loads can answer Save, but only the row itself, under a Grant; every
// Post this application hands around is a value the repository scanned or a
// copy, and a statement through either answers model.ErrUnwired.
//
// The tag is written out even where the snake case of the field name would
// answer the same, because "the name a reader can see" and "the name a
// convention derives" stop agreeing at the first field somebody renames.
type Post struct {
	model.Model

	ID string `db:"id"`

	// TenantID is whose post this is. It is written from the Grant and never
	// from a request, and it is not a mutable field: moving a row between
	// tenants is not an update.
	TenantID string `db:"tenant_id"`

	Title string `db:"title"`
	Slug  string `db:"slug"`
	Body  string `db:"body"`

	// CategoryID is the section this post is filed under, or empty for none.
	//
	// An id and not a Category. A model that carried the whole record would be a
	// model that had to be loaded with it, every time, from wherever it came
	// from -- which is the lazy-loading question, and the answer here is that
	// there is no lazy loading: the page that shows a section name asks for it.
	CategoryID string `db:"category_id"`

	// Views is how many times the article was opened. It is a counter and never
	// read into a decision, so it is incremented with a statement of its own
	// rather than read, added to and written back -- two readers doing the
	// latter concurrently record one view.
	Views int `db:"views"`

	PublishedAt time.Time `db:"published_at"`
	CreatedAt   time.Time `db:"created_at"`
}

// postTable is the posts table as the model layer sees it, and it exists for
// the relation and for nothing else.
//
// A relation reads a table, so the related side of the section relation needs
// one, and aru model:build generates a query beside every table: Posts and
// PostQuery, in PostQuery.go, exported like every generated query. They are not
// a second door. PostRepository is the door to this table, and nothing outside
// this package calls Posts; what reaches posts through the model is the
// relation, and a relation is reachable only from a section, under a Grant.
//
// The two settings are the two places posts is not the default table. The key
// is a text identifier the application writes, so it neither increments nor is
// drawn by the model; the table has no updated_at, so nothing may try to write
// one. The tenant column is left alone: it is tenant_id, which is what the
// model already assumes, and turning the scoping off is the only thing that has
// to be said out loud.
var postTable = model.NewTable(model.TableSpec{
	Name:            "posts",
	New:             func() model.Entity { return new(Post) },
	ManualKey:       true,
	UpdatedAtColumn: model.NoColumn,
	// arandu:begin custom
	// arandu:end custom
})

// Published reports whether the post is out.
//
// PublishedAt is a time.Time rather than a pointer, so "not published" is the
// zero value -- and the zero value is written to SQL as 0001-01-01, which is a
// real date. `WHERE published_at IS NOT NULL` therefore answers true for every
// draft, which is how a draft reached the sitemap once. A method here so the
// comparison is written correctly in one place.
func (p Post) Published() bool { return !p.PublishedAt.IsZero() }

// What can go wrong with a post, declared beside the entity rather than
// inside the repository.
//
// The controller maps them to a status code and the repository returns them, so
// both need to name them -- and a controller that imported the repository to
// reach an error would be a controller with a door to the data layer, which is
// the one thing the tree exists to prevent.
var (
	// ErrPostNotFound is returned when no row matches.
	ErrPostNotFound = errors.New("post: not found")
	// ErrPostConflict is a unique constraint refusing a duplicate.
	ErrPostConflict = errors.New("post: already exists")
	// ErrPostSort is an ordering the allowlist does not contain.
	ErrPostSort = errors.New("post: sort field not allowed")
)

// LogValue implements slog.LogValuer, so passing the whole entity to a log call
// records the identifiers and nothing else. Add any sensitive field to the
// custom block below and it stays out of logs, dumps and the debug page.
func (p Post) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("id", p.ID),
		slog.String("tenant", p.TenantID),
	)
}

// arandu:begin custom
// MarshalJSON writes every field explicitly. No field reaches the wire without
// being named here: a column added to the struct and left out below is private
// by omission, which is what a database default is not.
func (p Post) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		ID          string    `json:"id"`
		TenantID    string    `json:"tenant_id"`
		Title       string    `json:"title"`
		Slug        string    `json:"slug"`
		Body        string    `json:"body"`
		CategoryID  string    `json:"category_id"`
		Views       int       `json:"views"`
		PublishedAt time.Time `json:"published_at"`
		CreatedAt   time.Time `json:"created_at"`
	}{
		ID: p.ID, TenantID: p.TenantID, Title: p.Title, Slug: p.Slug,
		Body: p.Body, CategoryID: p.CategoryID, Views: p.Views,
		PublishedAt: p.PublishedAt, CreatedAt: p.CreatedAt,
	})
}

// arandu:end custom
