package customer

import (
	"strings"

	"github.com/arandu-io/framework/validation"
)

// CreateRequest is the input contract. Fields are explicit: there is no mass
// assignment, so the bug class Laravel's $fillable exists to contain does not
// exist here -- a field the client sends and the struct does not declare simply
// goes nowhere.
type CreateRequest struct {
	Name     string
	Email    string
	Document string
}

// Validate reports the errors per field.
func (r CreateRequest) Validate() validation.Errors {
	e := validation.Errors{}

	validation.Required(e, "name", r.Name)
	validation.MinLen(e, "name", r.Name, 2)
	validation.MaxLen(e, "name", r.Name, 120)

	validation.Required(e, "email", r.Email)
	validation.Email(e, "email", r.Email)
	validation.MaxLen(e, "email", r.Email, 254)

	validation.Required(e, "document", r.Document)
	validateDocument(e, "document", r.Document)

	return e
}

// UpdateRequest carries the id as well, and the same rules.
type UpdateRequest struct {
	ID       string
	Name     string
	Email    string
	Document string
}

// Validate reports the errors per field.
func (r UpdateRequest) Validate() validation.Errors {
	e := CreateRequest{Name: r.Name, Email: r.Email, Document: r.Document}.Validate()
	validation.Required(e, "id", r.ID)
	return e
}

// validateDocument checks the shape of the registration number.
//
// The rule lives here, in the module, and not in the framework: a document is a
// domain concept, and the framework that ships a national tax id validator ends
// up shipping twenty of them. This one checks digits and length only -- a real
// application replaces it with the check digit algorithm it needs.
func validateDocument(e validation.Errors, field, value string) {
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, value)

	if len(digits) < 11 || len(digits) > 14 {
		e.Add(field, "must have between 11 and 14 digits")
	}
}

// Compile-time proof that both requests honor the validation contract.
var (
	_ validation.Validatable = CreateRequest{}
	_ validation.Validatable = UpdateRequest{}
)
