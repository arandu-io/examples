package models

import (
	"errors"
	"log/slog"
	"time"
)

// Category is the entity. It has no persistence methods: this is not Active
// Record, and a type that can save itself can save itself from anywhere.
//
// There is no Find, no Save and no query builder on this type. The table is
// reached by CategoryRepository, and every method of a repository -- Find and
// List included -- takes a security.Grant that only a Policy can issue. The
// model is data; the Policy is the door.
type Category struct {
	ID          string
	Name        string
	Slug        string
	Description string
	CreatedAt   time.Time
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
)

// LogValue implements slog.LogValuer, so passing the whole entity to a log call
// records the identifiers and nothing else. Add any sensitive field to the
// custom block below and it stays out of logs, dumps and the debug page.
func (ca Category) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("id", ca.ID),
	)
}

// arandu:begin custom
// MarshalJSON, computed fields and anything else about this entity go here.
// arandu:end custom
