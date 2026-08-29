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

// What a reset code is bound to, proved from outside.
//
// The native CodeStore binds purpose, tenant, account, address and the current
// password fingerprint. Its state lives in the configured cache, not in an
// authentication table in the application database.
//
// Journey_test.go already proves the reset ends the sessions that were open.
// That is a property of the session store. What is proved here is the property
// of the code subject itself: what the cache stores, what the code is bound to,
// and what is checked when it comes back.

const resetAccount = "grace.hopper@example.com"

// TestAResetCodeIsRefusedTheSecondTimeItIsUsed.
//
// The CodeStore consumes it atomically, and the password fingerprint changes
// as a second independent invalidation boundary.
func TestAResetCodeIsRefusedTheSecondTimeItIsUsed(t *testing.T) {
	client, db, box := tests.AppWithMailbox(t)
	signInAs(t, client, db, "Grace Hopper", "")

	code := askForAResetCode(t, client, box, resetAccount)

	const first = "a-completely-new-password"
	submitReset(client, code, resetAccount, first).OK().See("has been changed")
	client.Get("/auth/password/reset?email=" + resetAccount).OK()

	// The same code, a second time. Nothing about it has changed.
	const second = "another-new-password-again"
	body := submitReset(client, code, resetAccount, second).Status(422).Body()
	if !strings.Contains(body, "not valid") {
		t.Errorf("a spent code was not refused as spent:\n%s", body)
	}

	// And the refusal is real: the second password was never written.
	client.Get("/auth/login").OK()
	client.Post("/auth/login", map[string]string{"email": resetAccount, "password": second}).
		Status(401)
	client.Get("/auth/login").OK()
	client.Post("/auth/login", map[string]string{"email": resetAccount, "password": first}).
		RedirectsTo("/")
}

// TestAResetCodeDiesWhenThePasswordChangesByAnyOtherRoute.
//
// The code is invalidated by the password
// changing, whoever changed it and however -- here it is the service call the
// operator's seeder makes, which never sees the code and knows nothing about it.
//
// That is the part a table of tokens does not give without being told to. Such
// a table has to delete the row that was used, then remember to delete the
// siblings that were minted before it, and then sweep the ones nobody ever
// used. Binding every issued code to the password fingerprint expires the whole
// set at the same instant the hash is replaced.
func TestAResetCodeDiesWhenThePasswordChangesByAnyOtherRoute(t *testing.T) {
	booted := tests.Boot(t)
	client, db, box := booted.Client, booted.DB, booted.Mail
	signInAs(t, client, db, "Grace Hopper", "")

	code := askForAResetCode(t, client, box, resetAccount)

	// Changed somewhere else entirely, by the path `aru db:seed UserSeeder -upd`
	// takes. The code above is still in the inbox and has not been touched.
	if _, err := booted.App.Users.SetPassword(
		context.Background(), bootstrap.Tenant(), resetAccount, "changed-from-the-terminal",
	); err != nil {
		t.Fatalf("replacing the password out of band: %v", err)
	}

	body := submitReset(client, code, resetAccount, "a-password-from-the-code").Status(422).Body()
	if !strings.Contains(body, "not valid") {
		t.Errorf("a code minted against a password that has since been replaced was accepted:\n%s", body)
	}
}

// TestAResetCodeIsRefusedAtAnAddressItWasNotMintedFor.
//
// The address is part of the purpose-bound CodeStore subject and is compared
// with the one typed on the form. Without that comparison the e-mail field is
// decoration: the screen asks for an address, throws it away, and resets
// whichever account the subject names.
//
// The code here is valid for this purpose, account and current password. The
// only thing wrong is the address, and that is enough.
func TestAResetCodeIsRefusedAtAnAddressItWasNotMintedFor(t *testing.T) {
	booted := tests.Boot(t)
	client, box := booted.Client, booted.Mail

	// Both accounts are made through the service rather than through the
	// registration form, and nobody is signed in. The form is behind the guest
	// guard, so registering the second account on a client that had just signed
	// in as the first is a 303 rather than a page -- and this test is about the
	// address bound to a code, not about who is holding a session.
	const other = "ada.lovelace@example.com"
	for name, email := range map[string]string{"Grace Hopper": resetAccount, "Ada Lovelace": other} {
		if _, err := booted.App.Users.EnsureUser(
			context.Background(), bootstrap.Tenant(), name, email, "a-password-that-passes", nil, true,
		); err != nil {
			t.Fatalf("creating %s: %v", email, err)
		}
	}

	code := askForAResetCode(t, client, box, resetAccount)

	body := submitReset(client, code, other, "a-password-for-the-wrong-one").
		Status(422).Body()
	if !strings.Contains(body, "not valid") {
		t.Errorf("a code minted for one address was accepted at another:\n%s", body)
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

// TestNoAuthenticationTokenTableIsCreatedForResetCodes.
//
// The claim the flow makes about itself, asked of the schema: a code is issued,
// mailed and then successfully spent through the configured cache, and at no
// point does the application database hold an authentication-token table.
//
// It reads the schema rather than counting rows in a named table, because the
// failure it guards against is somebody adding the table back: a count against
// a table that does not exist is an error, and a test that treated that error
// as "nothing was written" would go on passing after the table arrived.
func TestNoAuthenticationTokenTableIsCreatedForResetCodes(t *testing.T) {
	client, db, box := tests.AppWithMailbox(t)
	signInAs(t, client, db, "Grace Hopper", "")

	code := askForAResetCode(t, client, box, resetAccount)

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
			t.Errorf("the schema holds %q: reset codes belong to the configured native CodeStore, not an application token table", name)
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

	// And the cache-backed code works without an application token table.
	submitReset(client, code, resetAccount, "a-completely-new-password").OK().See("has been changed")
}

// askForAResetCode walks the form and returns the code out of the message.
//
// It reads the mailbox rather than issuing a code beside the application,
// because a code this test issued would prove only the test's store. What is
// under examination is the one a person would type, produced by the
// path production takes.
func askForAResetCode(t *testing.T, client *arandutest.Client, box *mail.Array, email string) string {
	t.Helper()

	client.Get("/auth/password").OK()
	client.Post("/auth/password/email", map[string]string{"email": email}).OK()

	sent, ok := box.Last()
	if !ok {
		t.Fatal("no message was sent for the reset code")
	}
	found := emailCodePattern.FindString(sent.Text)
	if found == "" {
		t.Fatalf("no reset code in the message:\n%s", sent.Text)
	}

	client.Get("/auth/password/reset?email=" + email).OK().See(email)
	return found
}

// submitReset posts the new-password form the way the screen does.
func submitReset(client *arandutest.Client, code, email, password string) *arandutest.Response {
	return client.Post("/auth/password/update", map[string]string{
		"email_code": code, "email": email,
		"password": password, "password_confirmation": password,
	})
}
