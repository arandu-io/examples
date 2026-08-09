package policies

import (
	"context"
	"fmt"

	"github.com/arandu-io/framework/security"

	models "github.com/arandu-io/examples/app/Models"
)

// The actions of Comment. Constants rather than strings at the call site: a
// typo in an action name would silently authorize nothing, or worse, everything.
//
// They carry the entity in the name because every policy in the application
// lives in this package now, and five constants called ActionView would not
// compile past the first module.
const (
	// CommentView is reading one record.
	CommentView security.Action = "comment.view"
	// CommentList is paging through the records.
	CommentList security.Action = "comment.list"
	// CommentCreate is adding one.
	CommentCreate security.Action = "comment.create"
	// CommentUpdate is changing one.
	CommentUpdate security.Action = "comment.update"
	// CommentDelete is removing one.
	CommentDelete security.Action = "comment.delete"

	// CommentPublicList is the thread as a reader sees it: what is approved,
	// plus their own while it waits.
	//
	// A named action rather than CommentList with a filter, and the same
	// distinction PostPublicList draws for the same reason. The two questions
	// are different -- "every comment there is" and "what may be shown under
	// this article" -- and one permission answering both is a permission whose
	// meaning depends on who is asking. It has its own query, and the query is
	// what keeps a comment awaiting review out of a stranger's page rather than
	// a filter somebody removes while tidying.
	CommentPublicList security.Action = "comment.list.public"
)

// CommentPolicy is the only authority over who does what with Comment.
//
// IT DENIES EVERYTHING. That is deliberate: a generated policy that allowed
// anything would be a hole shipped by default, in every project that ran the
// generator. Open what this module actually needs, and nothing else.
type CommentPolicy struct{}

// Compile-time proof that the policy answers about this entity and no other.
var _ security.Policy[models.Comment] = CommentPolicy{}

// Can decides whether the subject may perform the action.
func (CommentPolicy) Can(ctx context.Context, s security.Subject, a security.Action, co models.Comment) error {

	// arandu:begin custom
	//
	// What this blog allows.
	//
	// Reading is public and writing is not, and the line between them is drawn
	// three times below rather than once, because they are three different
	// questions: what a stranger may see, what an account may add, and what a
	// moderator may do about it.
	//
	// A reader with no session arrives here as security.Guest, which is a
	// subject somebody built on purpose. A Subject nobody filled in is not one,
	// and Authorize refuses it before this method is called -- an empty subject
	// is almost always a session that failed to load, and answering an
	// authorization question about nobody is how a hole opens by accident.
	switch a {
	case CommentView, CommentPublicList:
		// The thread is public. A blog where you have to sign in to READ what
		// people said is not a blog, and the moderation state is what the query
		// behind this action protects -- not the login.
		return nil

	case CommentList:
		// Every comment there is, moderation queue included. That is the
		// administrator's, and it is the difference between this action and the
		// one above.
		if s.HasRole("admin") {
			return nil
		}

	case CommentCreate:
		// Writing needs a confirmed address, and reading does not.
		//
		// It is the cheapest thing that makes a comment section usable: an
		// account costs nothing to create, so "signed in" is not a bar at all,
		// and one round trip through an inbox is. It is also the reason to have
		// a verification flow -- a verified column nothing consults is a column.
		//
		// The check is here rather than in the controller because the controller
		// is not the authority, and because a second write path -- an import, an
		// API, a job -- would reach the same policy and be refused by the same
		// line.
		if !s.Verified {
			return fmt.Errorf("confirm your email address before commenting")
		}
		return nil
	}

	// Moderation is the administrator's. Approving somebody else's words, and
	// deleting them, is the pair of actions that needs an owner.
	//
	// Named, not "an administrator may do anything". Written the second way this
	// method answered nil for an action nobody has defined yet, which is a hole
	// that opens itself: the next action added to this entity would be allowed
	// before anybody decided it should be.
	if s.HasRole("admin") && (a == CommentUpdate || a == CommentDelete) {
		return nil
	}

	// Deleting your own comment is allowed while it is still awaiting review. The
	// author column holds the subject id, which is what makes this comparable.
	// After it is approved somebody has replied to it, and a thread with holes
	// in it reads worse than a comment its author regrets.
	if a == CommentDelete && co.Author == s.ID && !co.Approved {
		return nil
	}

	// arandu:end custom

	// Everything else is refused, including an action nobody has defined yet.
	// This line is what makes the switch above the whole list rather than the
	// beginning of one.
	return fmt.Errorf("no rule allows %s on comment", a)
}
