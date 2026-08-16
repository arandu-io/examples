package models

import (
	"errors"
	"log/slog"
	"time"
)

// Comment is the entity. It has no persistence methods: this is not Active
// Record, and a type that can save itself can save itself from anywhere.
//
// There is no Find, no Save and no query builder on this type. The table is
// reached by CommentRepository, and every method of a repository -- Find and
// List included -- takes a security.Grant that only a Policy can issue. The
// model is data; the Policy is the door.
type Comment struct {
	ID        string
	PostId    string
	Author    string
	Body      string
	Approved  bool
	CreatedAt time.Time
}

// What can go wrong with a comment, declared beside the entity rather than
// inside the repository.
//
// The controller maps them to a status code and the repository returns them, so
// both need to name them -- and a controller that imported the repository to
// reach an error would be a controller with a door to the data layer, which is
// the one thing the tree exists to prevent.
var (
	// ErrCommentNotFound is returned when no row matches.
	ErrCommentNotFound = errors.New("comment: not found")
	// ErrCommentConflict is a unique constraint refusing a duplicate.
	ErrCommentConflict = errors.New("comment: already exists")
	// ErrCommentSort is an ordering the allowlist does not contain.
	ErrCommentSort = errors.New("comment: sort field not allowed")
)

// LogValue implements slog.LogValuer, so passing the whole entity to a log call
// records the identifiers and nothing else. Add any sensitive field to the
// custom block below and it stays out of logs, dumps and the debug page.
func (co Comment) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("id", co.ID),
	)
}

// arandu:begin custom
// MarshalJSON, computed fields and anything else about this entity go here.
// arandu:end custom
