package requests

import (
	"github.com/arandu-io/framework/validation"
)

// StoreCategory is the input contract of creation. ctx.Bind fills it through
// the form tags, and only those: there is no mass assignment, so a key the
// client sends and this struct does not declare goes nowhere.
type StoreCategory struct {
	Name        string `form:"name"`
	Slug        string `form:"slug"`
	Description string `form:"description"`
}

// Validate reports the errors per field.
func (r StoreCategory) Validate() validation.Errors {
	e := validation.Errors{}
	validation.Required(e, "name", r.Name)
	validation.MaxLen(e, "name", r.Name, 255)
	validation.Required(e, "slug", r.Slug)
	validation.MaxLen(e, "slug", r.Slug, 255)
	validation.MaxLen(e, "description", r.Description, 5000)

	// arandu:begin custom
	// Domain rules go here: ranges, formats, cross-field checks.
	// arandu:end custom

	return e
}

// UpdateCategory carries the id as well, and the same rules.
type UpdateCategory struct {
	ID          string
	Name        string
	Slug        string
	Description string
}

// Validate reports the errors per field.
func (r UpdateCategory) Validate() validation.Errors {
	e := StoreCategory{
		Name:        r.Name,
		Slug:        r.Slug,
		Description: r.Description,
	}.Validate()
	validation.Required(e, "id", r.ID)
	return e
}

// Compile-time proof that both requests honor the validation contract.
var (
	_ validation.Validatable = StoreCategory{}
	_ validation.Validatable = UpdateCategory{}
)
