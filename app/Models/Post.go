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
	ID          string
	Title       string
	Slug        string
	Body        string
	PublishedAt time.Time
	CreatedAt   time.Time
}

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
