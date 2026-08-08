package requests

import (
	"time"

	"github.com/arandu-io/framework/validation"
)

// StorePost is the input contract of creation. Fields are explicit: there
// is no mass assignment, so a field the client sends and this struct does not
// declare goes nowhere.
type StorePost struct {
	Title       string
	Slug        string
	Body        string
	PublishedAt time.Time
}

// Validate reports the errors per field.
func (r StorePost) Validate() validation.Errors {
	e := validation.Errors{}
	validation.Required(e, "title", r.Title)
	validation.MaxLen(e, "title", r.Title, 255)
	validation.Required(e, "slug", r.Slug)
	validation.MaxLen(e, "slug", r.Slug, 255)
	validation.Required(e, "body", r.Body)
	validation.MaxLen(e, "body", r.Body, 5000)

	// arandu:begin custom
	// Domain rules go here: ranges, formats, cross-field checks.
	// arandu:end custom

	return e
}

// UpdatePost carries the id as well, and the same rules.
type UpdatePost struct {
	ID          string
	Title       string
	Slug        string
	Body        string
	PublishedAt time.Time
}

// Validate reports the errors per field.
func (r UpdatePost) Validate() validation.Errors {
	e := StorePost{
		Title:       r.Title,
		Slug:        r.Slug,
		Body:        r.Body,
		PublishedAt: r.PublishedAt,
	}.Validate()
	validation.Required(e, "id", r.ID)
	return e
}

// Compile-time proof that both requests honor the validation contract.
var (
	_ validation.Validatable = StorePost{}
	_ validation.Validatable = UpdatePost{}
)

var _ = time.Time{}
