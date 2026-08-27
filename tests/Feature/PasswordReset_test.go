package feature_test

import (
	"context"
	"strings"
	"testing"

	"github.com/arandu-io/framework/arandutest"
	"github.com/arandu-io/framework/mail"

	"github.com/arandu-io/examples/bootstrap"
	"github.com/arandu-io/examples/tests"
)

// What a reset link is made of, proved from outside.
//
// The link carries the tenant, the account, the address it was mailed to and a
// fingerprint of the password that account had when it was minted, signed with
// the application key. Nothing is written when the mail goes out: there is no
// token table, no cleanup job and no row to delete.
//
// Journey_test.go already proves the reset ends the sessions that were open.
// That is a property of the session store. What is proved here is the property
// of the payload itself -- the part that has no row behind it -- because a flow
// that stores nothing is a flow whose correctness is entirely in what the link
// says and what is checked when it comes back.

const resetAccount = "grace.hopper@example.com"

// TestAResetLinkIsRefusedTheSecondTimeItIsUsed.
//
// This is what makes the link single use, and it is the fingerprint doing it
// rather than a row being deleted. The signature is still good and the hour has
// not passed -- the token is byte for byte the one that worked a moment ago --
// so the only thing left to refuse it is the fingerprint, which no longer
// matches the hash the first reset wrote.
func TestAResetLinkIsRefusedTheSecondTimeItIsUsed(t *testing.T) {
	client, db, box := tests.AppWithMailbox(t)
	signInAs(t, client, db, "Grace Hopper", "")

	token := askForAResetLink(t, client, box, resetAccount)

	const first = "a-completely-new-password"
	submitReset(client, token, resetAccount, first).OK().See("has been changed")

	// The signature and the clock still accept it. This GET verifies the token
	// and nothing else -- it does not look at the fingerprint -- so a 200 here
	// is the proof that the refusal below is not the signature failing and not
	// the hour having passed. The token is still one this application issued,
	// for this purpose, within its TTL.
	client.Get("/auth/password/reset?token=" + token).OK()

	// The same token, a second time. Nothing about it has changed.
	const second = "another-new-password-again"
	body := submitReset(client, token, resetAccount, second).Status(422).Body()
	if !strings.Contains(body, "not valid any more") {
		t.Errorf("a spent link was not refused as spent:\n%s", body)
	}
	// It is refused as spent and not as expired, which is the distinction the
	// screen draws and the one that says the fingerprint did this rather than
	// the clock.
	if strings.Contains(body, "has expired") {
		t.Error("the spent link was reported as expired: the hour has not passed, the fingerprint is what refused it")
	}

	// And the refusal is real: the second password was never written.
	client.Get("/auth/login").OK()
	client.Post("/auth/login", map[string]string{"email": resetAccount, "password": second}).
		Status(401)
	client.Get("/auth/login").OK()
	client.Post("/auth/login", map[string]string{"email": resetAccount, "password": first}).
		RedirectsTo("/")
}

// TestAResetLinkDiesWhenThePasswordChangesByAnyOtherRoute.
//
// The link is not expired by being used. It is expired by the password
// changing, whoever changed it and however -- here it is the service call the
// operator's seeder makes, which never sees the link and knows nothing about it.
//
// That is the part a table of tokens does not give without being told to. Such
// a table has to delete the row that was used, then remember to delete the
// siblings that were minted before it, and then sweep the ones nobody ever
// clicked. This expires the whole set at the same instant, because there is no
// set: there is one hash, and every link ever minted against the old one stops
// verifying when it is replaced.
func TestAResetLinkDiesWhenThePasswordChangesByAnyOtherRoute(t *testing.T) {
	booted := tests.Boot(t)
	client, db, box := booted.Client, booted.DB, booted.Mail
	signInAs(t, client, db, "Grace Hopper", "")

	token := askForAResetLink(t, client, box, resetAccount)

	// Changed somewhere else entirely, by the path `aru db:seed UserSeeder -upd`
	// takes. The link above is still in the inbox and has not been touched.
	if _, err := booted.App.Auth.SetPassword(
		context.Background(), bootstrap.Tenant(), resetAccount, "changed-from-the-terminal",
	); err != nil {
		t.Fatalf("replacing the password out of band: %v", err)
	}

	body := submitReset(client, token, resetAccount, "a-password-from-the-link").Status(422).Body()
	if !strings.Contains(body, "not valid any more") {
		t.Errorf("a link minted against a password that has since been replaced was accepted:\n%s", body)
	}
}

// TestAResetLinkIsRefusedAtAnAddressItWasNotMintedFor.
//
// The address is in the payload, signed, and it is compared with the one typed
// on the form. Without that comparison the e-mail field is decoration: the
// screen asks for an address, throws it away, and resets whichever account the
// token names.
//
// The token here is entirely valid -- this application signed it, for this
// purpose, within the hour, against the current password. The only thing wrong
// is the address, and that is enough.
func TestAResetLinkIsRefusedAtAnAddressItWasNotMintedFor(t *testing.T) {
	booted := tests.Boot(t)
	client, box := booted.Client, booted.Mail

	// Both accounts are made through the service rather than through the
	// registration form, and nobody is signed in. The form is behind the guest
	// guard, so registering the second account on a client that had just signed
	// in as the first is a 303 rather than a page -- and this test is about the
	// address on a link, not about who is holding a session.
	const other = "ada.lovelace@example.com"
	for name, email := range map[string]string{"Grace Hopper": resetAccount, "Ada Lovelace": other} {
		if _, err := booted.App.Auth.EnsureUser(
			context.Background(), bootstrap.Tenant(), name, email, "a-password-that-passes", nil, true,
		); err != nil {
			t.Fatalf("creating %s: %v", email, err)
		}
	}

	token := askForAResetLink(t, client, box, resetAccount)

	body := submitReset(client, token, other, "a-password-for-the-wrong-one").
		Status(422).Body()
	if !strings.Contains(body, "not valid any more") {
		t.Errorf("a link minted for one address was accepted at another:\n%s", body)
	}

	// Neither account was touched. The one whose address was typed was never a
	// candidate, and it still has the password it was created with.
	client.Get("/auth/login").OK()
	client.Post("/auth/login", map[string]string{
		"email": other, "password": "a-password-for-the-wrong-one",
	}).Status(401)
	client.Get("/auth/login").OK()
	client.Post("/auth/login", map[string]string{
		"email": other, "password": "a-password-that-passes",
	}).RedirectsTo("/")
}

// TestNothingIsWrittenDownWhenAResetLinkIsMinted.
//
// The claim the flow makes about itself, asked of the schema: a link is minted,
// mailed and then successfully spent, and at no point does the database hold a
// table for it. What makes the link work is the signature, and what makes it
// stop working is the fingerprint -- neither is a row.
//
// It reads the schema rather than counting rows in a named table, because the
// failure it guards against is somebody adding the table back: a count against
// a table that does not exist is an error, and a test that treated that error
// as "nothing was written" would go on passing after the table arrived.
func TestNothingIsWrittenDownWhenAResetLinkIsMinted(t *testing.T) {
	client, db, box := tests.AppWithMailbox(t)
	signInAs(t, client, db, "Grace Hopper", "")

	token := askForAResetLink(t, client, box, resetAccount)

	rows, err := db.QueryContext(context.Background(),
		`SELECT name FROM sqlite_master WHERE type = 'table'`)
	if err != nil {
		t.Fatalf("reading the schema: %v", err)
	}
	defer func() { _ = rows.Close() }()

	seen := 0
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("reading a table name: %v", err)
		}
		seen++
		if strings.Contains(strings.ToLower(name), "token") {
			t.Errorf("the schema holds %q: the reset flow is stateless, and a table of tokens "+
				"is a restart that drops every link in flight and a sweep nobody wrote", name)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("walking the schema: %v", err)
	}
	// A schema that could not be read would satisfy the loop above without
	// looking at anything.
	if seen == 0 {
		t.Fatal("no tables were read, so nothing above was checked")
	}

	// And the link that nothing was written for still works.
	submitReset(client, token, resetAccount, "a-completely-new-password").OK().See("has been changed")
}

// askForAResetLink walks the "send me a link" form and returns the token out of
// the message that was sent.
//
// It reads the mailbox rather than minting a token beside the application,
// because a token this test signed is a token that proves this test can sign:
// what is under examination is the one a person would click, produced by the
// path production takes.
func askForAResetLink(t *testing.T, client *arandutest.Client, box *mail.Array, email string) string {
	t.Helper()

	client.Get("/auth/password").OK()
	client.Post("/auth/password/email", map[string]string{"email": email}).OK()

	sent, ok := box.Last()
	if !ok {
		t.Fatal("no message was sent for the reset link")
	}
	found := resetToken.FindStringSubmatch(sent.Text + sent.HTML)
	if found == nil {
		t.Fatalf("no reset token in the message:\n%s", sent.Text)
	}

	// The screen behind the link fills the address in from the payload, and it
	// is asked for here so a link that cannot be opened fails at this line
	// rather than three asserts later.
	client.Get("/auth/password/reset?token=" + found[1]).OK()
	return found[1]
}

// submitReset posts the new-password form the way the screen does.
func submitReset(client *arandutest.Client, token, email, password string) *arandutest.Response {
	return client.Post("/auth/password/update", map[string]string{
		"token": token, "email": email,
		"password": password, "password_confirmation": password,
	})
}
