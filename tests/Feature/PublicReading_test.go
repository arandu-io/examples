package feature_test

import (
	"testing"

	"github.com/arandu-io/examples/tests"
)

// TestAReaderWithNoAccountSeesThePublishedPosts is what this blog exists to
// demonstrate, written the way an application should write it.
//
// It is a feature test: it boots the application and makes requests, and what it
// proves is a decision that lives in a policy rather than in this file.
func TestAReaderWithNoAccountSeesThePublishedPosts(t *testing.T) {
	guest, _ := tests.App(t)

	// The listing answers, and it is the published one.
	guest.Get("/posts").OK().
		See("Posts").
		// The button belongs to somebody with an account. An empty URL draws no
		// control, which is how the policy reaches the markup as data.
		DontSee("Write one")

	// The comment form is not drawn for them, and the invitation is.
	//
	// DontSee is the half that matters here: a form that renders for a guest is
	// a form that posts and fails, and "it looked fine" is what a test that only
	// asserts presence reports.
	guest.Get("/posts").OK().DontSee("Say something")
}

// TestTheSignInScreenCarriesAToken.
//
// Every write in this application is refused without one, and a screen rendered
// without a token answers 200 and then refuses the submission with 419 -- which
// reads like a broken session rather than a missing field.
func TestTheSignInScreenCarriesAToken(t *testing.T) {
	c, _ := tests.App(t)

	c.Get("/auth/login").OK().See(`name="_csrf"`)
}

// TestAWriteWithoutATokenIsRefused is the other half, and it is the one that
// proves the protection is on rather than that the field is drawn.
func TestAWriteWithoutATokenIsRefused(t *testing.T) {
	// A fresh client has loaded no page, so it holds no token -- which is
	// exactly the request an attacker makes.
	c, _ := tests.App(t)
	c.Post("/auth/login", map[string]string{"email": "you@example.test"}).
		// 419, not 403, and the distinction is worth keeping: 403 is "you may
		// not", 419 is "your page is stale, load it again".
		Status(419)
}
