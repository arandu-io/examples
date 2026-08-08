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
	// What this blog allows, and it is four lines because the interesting part
	// of a comment system is who may moderate rather than who may write.
	//
	// Anybody signed in may read the thread and add to it. security.Authorize
	// refuses an anonymous subject before it ever calls this method, so an
	// unauthenticated reader never reaches here -- the controller answers them
	// with the article and no form.
	switch a {
	case CommentView, CommentList, CommentCreate:
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
