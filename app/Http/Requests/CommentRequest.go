package requests

import (
	"github.com/arandu-io/framework/validation"
)

// StoreComment is the input contract of creation. Fields are explicit: there
// is no mass assignment, so a field the client sends and this struct does not
// declare goes nowhere.
type StoreComment struct {
	PostId   string
	Author   string
	Body     string
	Approved bool
}

// Validate reports the errors per field.
func (r StoreComment) Validate() validation.Errors {
	e := validation.Errors{}
	validation.Required(e, "post_id", r.PostId)
	validation.MaxLen(e, "post_id", r.PostId, 255)
	validation.Required(e, "author", r.Author)
	validation.MaxLen(e, "author", r.Author, 255)
	validation.Required(e, "body", r.Body)
	validation.MaxLen(e, "body", r.Body, 5000)

	// arandu:begin custom
	// Domain rules go here: ranges, formats, cross-field checks.
	// arandu:end custom

	return e
}

// UpdateComment carries the id as well, and the same rules.
type UpdateComment struct {
	ID       string
	PostId   string
	Author   string
	Body     string
	Approved bool
}

// Validate reports the errors per field.
func (r UpdateComment) Validate() validation.Errors {
	e := StoreComment{
		PostId:   r.PostId,
		Author:   r.Author,
		Body:     r.Body,
		Approved: r.Approved,
	}.Validate()
	validation.Required(e, "id", r.ID)
	return e
}

// Compile-time proof that both requests honor the validation contract.
var (
	_ validation.Validatable = StoreComment{}
	_ validation.Validatable = UpdateComment{}
)
