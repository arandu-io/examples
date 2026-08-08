package policies

import (
	"context"
	"fmt"

	"github.com/arandu-io/framework/security"

	models "github.com/arandu-io/examples/app/Models"
)

// The actions of Post. Constants rather than strings at the call site: a
// typo in an action name would silently authorize nothing, or worse, everything.
//
// They carry the entity in the name because every policy in the application
// lives in this package now, and five constants called ActionView would not
// compile past the first module.
const (
	// PostView is reading one record.
	PostView security.Action = "post.view"
	// PostList is paging through the records.
	PostList security.Action = "post.list"
	// PostCreate is adding one.
	PostCreate security.Action = "post.create"
	// PostUpdate is changing one.
	PostUpdate security.Action = "post.update"
	// PostDelete is removing one.
	PostDelete security.Action = "post.delete"
)

// PostPolicy is the only authority over who does what with Post.
//
// IT DENIES EVERYTHING. That is deliberate: a generated policy that allowed
// anything would be a hole shipped by default, in every project that ran the
// generator. Open what this module actually needs, and nothing else.
type PostPolicy struct{}

// Compile-time proof that the policy answers about this entity and no other.
var _ security.Policy[models.Post] = PostPolicy{}

// Can decides whether the subject may perform the action.
func (PostPolicy) Can(ctx context.Context, s security.Subject, a security.Action, p models.Post) error {

	// arandu:begin custom
	//
	// Every action needs somebody signed in, and the reason is not a choice
	// this policy made: `security.Authorize` refuses an anonymous subject
	// before it ever calls Can, so a rule written for an empty ID would never
	// be reached. The controller redirects to the sign-in screen first, which
	// is the same answer arriving earlier.
	//
	// # There is no public reading yet, and this is where it would go
	//
	// A blog is read by people who are not signed in, and today this framework
	// has no path for that: Authorize refuses an anonymous subject, there is no
	// guest Subject, and the one way to reach a repository without a session is
	// security.SystemGrant -- which `aru doctor` reports outside a seeder, a
	// job or a command, on purpose.
	//
	// So this application is an authoring tool with a login, not a public site.
	// Making it one is a framework decision, not a policy this file can write,
	// and it is the same decision the arandu-io website needs.
	if s.ID == "" {
		return fmt.Errorf("this blog is read and written by signed-in authors")
	}

	switch a {
	case PostList, PostView:
		return nil

	case PostCreate, PostUpdate, PostDelete:
		// A draft belongs to whoever is writing it, and this is the rule that
		// could not have been a middleware: it depends on the row, not on the
		// path. A published post is edited by anybody with an account; an
		// unpublished one is not somebody else's business.
		//
		// Post carries no author column, so "whose draft" is not expressible
		// yet -- adding author_id is what the next version of this example
		// would do, and the rule would read `p.AuthorID != s.ID`.
		return nil
	}
	// arandu:end custom

	return fmt.Errorf("no rule allows %s on post", a)
}
