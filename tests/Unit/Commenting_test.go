package unit_test

import (
	"context"
	"strings"
	"testing"

	"github.com/arandu-io/framework/security"

	models "github.com/arandu-io/examples/app/Models"
	policies "github.com/arandu-io/examples/app/Policies"
)

// The three subjects this blog answers to, and the whole point of the file is
// that they are three and not two.
func subjects() (guest, reader, unverified, admin security.Subject) {
	guest = security.Guest("t1")
	reader = security.Subject{ID: "u1", Tenant: "t1", Verified: true}
	unverified = security.Subject{ID: "u2", Tenant: "t1"}
	admin = security.Subject{ID: "u3", Tenant: "t1", Roles: []string{"admin"}, Verified: true}
	return
}

// TestReadingTheThreadIsPublicAndTheQueueIsNot.
//
// A blog where you have to sign in to read what people said is not a blog. The
// thing that is NOT public is a comment waiting for review -- and that is what
// the two list actions are for: CommentPublicList has a query behind it that
// returns what is approved plus the reader's own, and CommentList returns
// everything and belongs to a moderator.
//
// One action answering both questions would be an action whose meaning depends
// on who is asking, which is the kind nobody can audit.
func TestReadingTheThreadIsPublicAndTheQueueIsNot(t *testing.T) {
	guest, reader, _, admin := subjects()

	for _, c := range []struct {
		who     string
		subject security.Subject
		action  security.Action
		allowed bool
	}{
		{"a guest reads the thread", guest, policies.CommentPublicList, true},
		{"a reader reads the thread", reader, policies.CommentPublicList, true},

		{"a guest cannot page the queue", guest, policies.CommentList, false},
		{"a reader cannot page the queue", reader, policies.CommentList, false},
		{"a moderator can", admin, policies.CommentList, true},
	} {
		t.Run(c.who, func(t *testing.T) {
			err := (policies.CommentPolicy{}).Can(context.Background(), c.subject, c.action, models.Comment{})
			if c.allowed && err != nil {
				t.Fatalf("refused: %v", err)
			}
			if !c.allowed && err == nil {
				t.Fatal("allowed")
			}
		})
	}
}

// TestWritingNeedsAConfirmedAddress.
//
// It is the cheapest thing that makes a comment section usable: an account costs
// nothing to create, so "signed in" is not a bar at all, and one round trip
// through an inbox is.
//
// It is also the only reason the verification flow is worth having. A verified
// column that nothing consults is a column.
func TestWritingNeedsAConfirmedAddress(t *testing.T) {
	guest, reader, unverified, _ := subjects()

	if err := (policies.CommentPolicy{}).Can(context.Background(), reader, policies.CommentCreate, models.Comment{}); err != nil {
		t.Fatalf("a verified reader could not comment: %v", err)
	}

	err := (policies.CommentPolicy{}).Can(context.Background(), unverified, policies.CommentCreate, models.Comment{})
	if err == nil {
		t.Fatal("an unconfirmed address was allowed to comment")
	}
	// The message is shown to a person, so it says what to do rather than what
	// went wrong.
	if !strings.Contains(err.Error(), "email") {
		t.Errorf("the refusal does not say what is missing: %v", err)
	}

	if err := (policies.CommentPolicy{}).Can(context.Background(), guest, policies.CommentCreate, models.Comment{}); err == nil {
		t.Fatal("a guest was allowed to comment")
	}
}

// TestModerationIsNamedAndNotInherited.
//
// Written as "an administrator may do anything", this policy answered nil for
// actions nobody had defined yet -- a hole that opens itself the next time
// somebody adds one. It is the mistake this test exists to keep out.
func TestModerationIsNamedAndNotInherited(t *testing.T) {
	_, _, _, admin := subjects()

	for _, a := range []security.Action{
		policies.CommentUpdate, policies.CommentDelete,
	} {
		if err := (policies.CommentPolicy{}).Can(context.Background(), admin, a, models.Comment{}); err != nil {
			t.Errorf("a moderator was refused %s: %v", a, err)
		}
	}

	if err := (policies.CommentPolicy{}).Can(context.Background(), admin,
		"comment.something_new", models.Comment{}); err == nil {
		t.Fatal("an action nobody has defined was allowed for an administrator")
	}
}

// TestAnAuthorMayWithdrawWhatIsStillWaiting.
//
// While it waits, deleting it costs nobody anything. After it is approved
// somebody has replied to it, and a thread with holes in it reads worse than a
// comment its author regrets.
func TestAnAuthorMayWithdrawWhatIsStillWaiting(t *testing.T) {
	_, reader, _, _ := subjects()

	waiting := models.Comment{ID: "c1", Author: reader.ID, Approved: false}
	if err := (policies.CommentPolicy{}).Can(context.Background(), reader, policies.CommentDelete, waiting); err != nil {
		t.Fatalf("an author could not withdraw their own pending comment: %v", err)
	}

	approved := models.Comment{ID: "c2", Author: reader.ID, Approved: true}
	if err := (policies.CommentPolicy{}).Can(context.Background(), reader, policies.CommentDelete, approved); err == nil {
		t.Error("an approved comment was deleted by its author, leaving a hole in the thread")
	}

	somebodyElses := models.Comment{ID: "c3", Author: "u9", Approved: false}
	if err := (policies.CommentPolicy{}).Can(context.Background(), reader, policies.CommentDelete, somebodyElses); err == nil {
		t.Error("a reader deleted somebody else's comment")
	}
}

// TestCategoriesAreReadByEverybodyAndWrittenByNobodyOrdinary.
//
// The navigation is built from this list, and a navigation that disappears when
// you sign out is a navigation that was never public. Writing is another
// question: somebody who may write a post is not therefore somebody who may
// invent a section of the blog.
func TestCategoriesAreReadByEverybodyAndWrittenByNobodyOrdinary(t *testing.T) {
	guest, reader, _, admin := subjects()

	for _, s := range []security.Subject{guest, reader, admin} {
		for _, a := range []security.Action{policies.CategoryView, policies.CategoryList} {
			if err := (policies.CategoryPolicy{}).Can(context.Background(), s, a, models.Category{}); err != nil {
				t.Errorf("%s was refused for %q: %v", a, s.ID, err)
			}
		}
	}

	for _, a := range []security.Action{
		policies.CategoryCreate, policies.CategoryUpdate, policies.CategoryDelete,
	} {
		if err := (policies.CategoryPolicy{}).Can(context.Background(), reader, a, models.Category{}); err == nil {
			t.Errorf("an ordinary account was allowed %s", a)
		}
		if err := (policies.CategoryPolicy{}).Can(context.Background(), guest, a, models.Category{}); err == nil {
			t.Errorf("a guest was allowed %s", a)
		}
		if err := (policies.CategoryPolicy{}).Can(context.Background(), admin, a, models.Category{}); err != nil {
			t.Errorf("an administrator was refused %s: %v", a, err)
		}
	}

	if err := (policies.CategoryPolicy{}).Can(context.Background(), admin,
		"category.something_new", models.Category{}); err == nil {
		t.Fatal("an action nobody has defined was allowed")
	}
}
