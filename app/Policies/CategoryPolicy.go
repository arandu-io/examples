package policies

import (
	"context"
	"fmt"

	"github.com/arandu-io/framework/security"

	models "github.com/arandu-io/examples/app/Models"
)

// The actions of Category. Constants rather than strings at the call site: a
// typo in an action name would silently authorize nothing, or worse, everything.
//
// They carry the entity in the name because every policy in the application
// lives in this package now, and five constants called ActionView would not
// compile past the first module.
const (
	// CategoryView is reading one record.
	CategoryView security.Action = "category.view"
	// CategoryList is paging through the records.
	CategoryList security.Action = "category.list"
	// CategoryCreate is adding one.
	CategoryCreate security.Action = "category.create"
	// CategoryUpdate is changing one.
	CategoryUpdate security.Action = "category.update"
	// CategoryDelete is removing one.
	CategoryDelete security.Action = "category.delete"
)

// CategoryPolicy is the only authority over who does what with Category.
//
// IT DENIES EVERYTHING. That is deliberate: a generated policy that allowed
// anything would be a hole shipped by default, in every project that ran the
// generator. Open what this module actually needs, and nothing else.
type CategoryPolicy struct{}

// Compile-time proof that the policy answers about this entity and no other.
var _ security.Policy[models.Category] = CategoryPolicy{}

// Can decides whether the subject may perform the action.
func (CategoryPolicy) Can(ctx context.Context, s security.Subject, a security.Action, ca models.Category) error {

	// arandu:begin custom
	//
	// A category is a public label. Everybody reads them, including a reader
	// with no account -- the navigation is built from this list, and a
	// navigation that disappears when you sign out is a navigation that was
	// never public.
	//
	// Writing is the administrator's. Categories are the shape of the site
	// rather than its content: somebody who may write a post is not therefore
	// somebody who may invent a section of the blog.
	if a == CategoryView || a == CategoryList {
		return nil
	}

	// From here it is a write, and only an administrator does those. A guest
	// falls through to the refusal below, which is the correct answer and the
	// same one an ordinary account gets.
	if s.HasRole("admin") {
		switch a {
		case CategoryCreate, CategoryUpdate:
			return nil
		case CategoryDelete:
			// A category with posts in it is not deleted, it is emptied first.
			// The database says the same thing -- posts.category_id is ON
			// DELETE SET NULL, so the rows would survive -- but a refusal here
			// says why, and the constraint only says "no".
			//
			// The count is not on the entity, so this is as far as the policy
			// can see. The service checks the rest, and it is the one place
			// that can: a policy that queried would be a policy with a
			// repository, and then the repository would need a policy.
			return nil
		}
	}
	// arandu:end custom

	return fmt.Errorf("no rule allows %s on category", a)
}
