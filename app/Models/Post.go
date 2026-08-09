package models

import (
	"errors"
	"log/slog"
	"time"
)

// Post is the entity. It has no persistence methods: this is not Active
// Record, and a type that can save itself can save itself from anywhere.
//
// Coming from an ORM, this is the difference that costs the most to find late:
// there is no Post::find, no ->save() and no query builder on this type.
// The table is reached by PostRepository, and every method of a repository
// -- Find and List included -- takes a security.Grant that only a Policy can
// issue (RULE 17). The model is data; the Policy is the door.
type Post struct {
	ID    string
	Title string
	Slug  string
	Body  string

	// CategoryID is the section this post is filed under, or empty for none.
	//
	// An id and not a Category. A model that carried the whole record would be a
	// model that had to be loaded with it, every time, from wherever it came
	// from -- which is the lazy-loading question, and the answer here is that
	// there is no lazy loading: the page that shows a section name asks for it.
	CategoryID string

	// Views is how many times the article was opened. It is a counter and never
	// read into a decision, so it is incremented with a statement of its own
	// rather than read, added to and written back -- two readers doing the
	// latter concurrently record one view.
	Views int

	PublishedAt time.Time
	CreatedAt   time.Time
}

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
	)
}

// arandu:begin custom
// MarshalJSON, computed fields and anything else about this entity go here.
// arandu:end custom
