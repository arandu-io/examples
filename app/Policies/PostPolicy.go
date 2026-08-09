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

	// PostPublicList is the published listing: what a reader with no account
	// sees, and what the sitemap is built from.
	//
	// A named action rather than PostList with a filter. The two questions are
	// different -- "page through everything" and "what is public" -- and one
	// permission answering both is a permission whose meaning depends on who is
	// asking, which is the kind nobody can audit. It has its own query, and the
	// query is what makes a draft unreachable rather than merely unlisted.
	PostPublicList security.Action = "post.list.public"
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
	// # A reader with no session
	//
	// A blog is read by people who are not signed in, and this is where that is
	// decided -- in the policy, like everything else, rather than by a
	// middleware that lets a request past before anybody asked what it wanted.
	//
	// A guest is a subject security.Guest built on purpose. A Subject nobody
	// filled in is not one, and Authorize still refuses it before reaching this
	// method: an empty subject is almost always a session that failed to load,
	// and answering an authorization question about nobody is how a hole opens
	// by accident.
	//
	// What a guest may do is read one published post. Not the listing: the index
	// pages through everything, drafts included, and a listing filtered "for
	// guests" would be a second query path with a second chance to be wrong.
	// Reading the article is what a link and a search result lead to.
	if s.IsGuest() {
		switch {
		// The published listing, which has a query of its own. PostList is not
		// opened: it pages through everything, drafts included, and a listing
		// "filtered for guests" would be a second query path with a second
		// chance to be wrong.
		case a == PostPublicList:
			return nil
		// PostView is asked twice: once with the zero value, for permission to
		// look at all, and again with the row that was read. The first has no
		// id, and answering it is what produces the Grant the repository needs.
		case a == PostView && p.ID == "":
			return nil
		case a == PostView && !p.PublishedAt.IsZero():
			return nil
		}
		return fmt.Errorf("%s is not public: a reader without an account sees published posts", a)
	}

	// From here it is somebody with an account.
	switch a {
	case PostList, PostView, PostPublicList:
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
