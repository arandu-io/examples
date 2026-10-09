package requests

import (
	"github.com/arandu-io/hesape/validation"
)

// StoreComment is the input contract of creation. ctx.Bind fills it through
// the form tags, and only those: there is no mass assignment, so a key the
// client sends and this struct does not declare goes nowhere.
type StoreComment struct {
	PostId   string `form:"post_id"`
	Author   string `form:"author"`
	Body     string `form:"body"`
	Approved bool   `form:"approved"`
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

// Compile-time proof that the request honors the validation contract.
var _ validation.Validatable = StoreComment{}
