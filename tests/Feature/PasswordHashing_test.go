package feature_test

import (
	"context"
	"strings"
	"testing"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/hesape/hashing"

	"github.com/arandu-io/examples/bootstrap"
	"github.com/arandu-io/examples/tests"
)

// What this application actually stores in the password column.
//
// There is one hashing path and it writes argon2id at parameters compiled into
// the binary -- not read from the environment, not configurable per deployment,
// not a driver name in a config file. An insecure hash configuration is the most
// common way to break authentication without anybody noticing, and the way it is
// usually reached is a knob somebody turned down to make a test suite faster.
//
// It is asserted over a row this application wrote, through the form a person
// fills in, rather than over a call to Make made beside it: what matters is not
// that the function can produce argon2id but that this is what ends up in the
// database of a project built on the skeleton.

// The compiled-in parameters, restated here so a change to them fails a test in
// a project rather than only in the component. They are duplicated on purpose:
// reading them from the package under examination would make this test agree
// with whatever the package says, which is not an assertion.
const (
	wantMemory  = 64 * 1024 // 64 MiB
	wantTime    = 3
	wantThreads = 4
	wantKeyLen  = 32
)

// TestTheStoredPasswordIsArgon2idAtTheCompiledInParameters.
func TestTheStoredPasswordIsArgon2idAtTheCompiledInParameters(t *testing.T) {
	client, db := tests.App(t)
	signInAs(t, client, db, "Grace Hopper", "")

	stored := storedPassword(t, db, "grace.hopper@example.com")

	// The plaintext did not leak into the column. This is the first question
	// worth asking of a password column and the cheapest one to get wrong: a
	// path that stored the password as typed still lets everybody sign in.
	if !hashing.IsHashed(stored) {
		t.Fatalf("the password column does not hold a hash this application can read: %q", stored)
	}
	if strings.Contains(stored, "a-password-that-passes") {
		t.Fatal("the plaintext password is in the password column")
	}

	params, ok := hashing.Info(stored)
	if !ok {
		t.Fatalf("the stored hash could not be read back: %q", stored)
	}
	if params.Algorithm != hashing.Argon2id {
		t.Errorf("the stored hash is %s, want argon2id: Check still reads argon2i and bcrypt so an "+
			"imported table signs in, but nothing in this application may write them",
			params.Algorithm)
	}
	if params.Memory != wantMemory || params.Time != wantTime ||
		params.Threads != wantThreads || params.KeyLen != wantKeyLen {
		t.Errorf("the stored hash is m=%d,t=%d,p=%d,keylen=%d, want m=%d,t=%d,p=%d,keylen=%d",
			params.Memory, params.Time, params.Threads, params.KeyLen,
			wantMemory, wantTime, wantThreads, wantKeyLen)
	}

	// Written at the current parameters, so nothing is due for a rehash on the
	// next sign-in. A fresh row that needs rehashing means the writer and the
	// reader disagree about what current means.
	if hashing.NeedsRehash(stored) {
		t.Error("a password this application has just written already needs rehashing")
	}

	// And it is the password that was typed.
	if err := hashing.Check("a-password-that-passes", stored); err != nil {
		t.Errorf("the stored hash does not verify against the password that was registered: %v", err)
	}
	if err := hashing.Check("not-the-password", stored); err == nil {
		t.Error("the stored hash verified against the wrong password")
	}
}

// TestEveryPathThatWritesAPasswordWritesTheSameKind.
//
// Three routes write this column -- registration, the reset code, and the
// operator's seeder -- and they are one path underneath, which is the property
// worth pinning. A second writer that reached for a hasher of its own is how an
// application ends up with rows nobody can explain: the account made from the
// terminal that cannot sign in, the reset that downgraded a hash.
//
// The salt is what makes this more than a repeated assertion: the same password
// hashed twice is two different strings, so the check below cannot pass by
// nothing having been written.
func TestEveryPathThatWritesAPasswordWritesTheSameKind(t *testing.T) {
	booted := tests.Boot(t)
	client, db, box := booted.Client, booted.DB, booted.Mail
	signInAs(t, client, db, "Grace Hopper", "")

	const address = "grace.hopper@example.com"
	registered := storedPassword(t, db, address)

	// Through the reset code.
	code := askForAResetCode(t, client, box, address)
	const viaCode = "a-completely-new-password"
	submitReset(client, code, address, viaCode).OK().See("has been changed")
	afterReset := storedPassword(t, db, address)

	// Through the service the seeder calls.
	const viaTerminal = "changed-from-the-terminal"
	if _, err := booted.App.Users.SetPassword(
		context.Background(), bootstrap.Tenant(), address, viaTerminal,
	); err != nil {
		t.Fatalf("replacing the password out of band: %v", err)
	}
	afterSeeder := storedPassword(t, db, address)

	for _, written := range []struct {
		by, hash, plain string
	}{
		{"registration", registered, "a-password-that-passes"},
		{"the reset code", afterReset, viaCode},
		{"the seeder's service call", afterSeeder, viaTerminal},
	} {
		params, ok := hashing.Info(written.hash)
		if !ok {
			t.Errorf("%s wrote something that is not a readable hash: %q", written.by, written.hash)
			continue
		}
		if params.Algorithm != hashing.Argon2id ||
			params.Memory != wantMemory || params.Time != wantTime ||
			params.Threads != wantThreads || params.KeyLen != wantKeyLen {
			t.Errorf("%s wrote %s m=%d,t=%d,p=%d,keylen=%d, want argon2id at m=%d,t=%d,p=%d,keylen=%d",
				written.by, params.Algorithm, params.Memory, params.Time, params.Threads, params.KeyLen,
				wantMemory, wantTime, wantThreads, wantKeyLen)
		}
		if err := hashing.Check(written.plain, written.hash); err != nil {
			t.Errorf("what %s wrote does not verify against the password it was given: %v", written.by, err)
		}
	}

	// Each write replaced the last. Equal hashes here would mean one of the
	// three did nothing and the assertions above read the previous row.
	if registered == afterReset || afterReset == afterSeeder {
		t.Error("two of the three writes left the same bytes in the column, so one of them wrote nothing")
	}
}

// storedPassword reads the password column of one account.
//
// Straight SQL against the throwaway database, because the hash is the one thing
// the model deliberately will not hand over: User carries MarshalJSON and
// LogValue precisely so it cannot reach a response, a log or a dump. Reading the
// column is how a test sees what a repository wrote without that guarantee being
// weakened for everybody.
func storedPassword(t *testing.T, db *data.DB, email string) string {
	t.Helper()

	var hash string
	if err := db.QueryRowContext(context.Background(),
		`SELECT password FROM users WHERE email = ? AND tenant_id = ?`,
		email, bootstrap.Tenant(),
	).Scan(&hash); err != nil {
		t.Fatalf("reading the stored password for %s: %v", email, err)
	}
	if hash == "" {
		t.Fatalf("the password column is empty for %s", email)
	}
	return hash
}
