package unit_test

import (
	"testing"

	"github.com/arandu-io/framework/modules/auth"
)

// The encoding of a password reset payload, checked from the application that
// signs one.
//
// The payload is four values in a string -- the tenant, the account, the address
// it was mailed to and a fingerprint of the password -- and each is written with
// its byte length in front of it. With a plain separator instead, the boundary
// between two values is decided by the values themselves, and two different
// accounts can produce the same bytes: a link minted for one then resets the
// other.
//
// It is checked here rather than left to the module because this application is
// what puts values into it. The tenant is a UUID today and the id is a UUID
// today, and neither of those is a promise -- a deployment that resolves the
// tenant from a host name, or an application that keys users on an address, puts
// characters in these fields that a separator would have to survive.

// TestTwoAccountsCannotShareAResetPayload.
//
// The pair that a plain separator collapses. Written with "|" between the
// fields, both of these are "a|b|c|..." and the signature over one verifies the
// other. Written with the length in front, they are different strings, and the
// boundary is decided by the writer rather than by the data.
func TestTwoAccountsCannotShareAResetPayload(t *testing.T) {
	one := auth.ResetPayload(auth.User{
		TenantID: "a",
		ID:       "b|c",
		Email:    "someone@example.com",
		Password: "$argon2id$v=19$m=65536,t=3,p=4$c2FsdHNhbHRzYWx0c2E$aGFzaA",
	})
	two := auth.ResetPayload(auth.User{
		TenantID: "a|b",
		ID:       "c",
		Email:    "someone@example.com",
		Password: "$argon2id$v=19$m=65536,t=3,p=4$c2FsdHNhbHRzYWx0c2E$aGFzaA",
	})

	if one == two {
		t.Fatalf("two accounts in different tenants produced the same payload: %q\n"+
			"a signature over one verifies the other, so a link minted for either resets whichever "+
			"account the reader of the payload picks", one)
	}
}

// TestTheAddressIsReadBackWholeWhateverIsInTheFieldsBeforeIt.
//
// The screen behind the link fills in the e-mail field from the payload, which
// means the address is read back out of it on every reset. A field ahead of it
// that contains the delimiter must not move where the address starts.
//
// The separator this uses is ":", which is what the length prefix itself is
// written with -- so a tenant that contains one is the case where a parser that
// scanned for delimiters instead of counting bytes goes wrong.
func TestTheAddressIsReadBackWholeWhateverIsInTheFieldsBeforeIt(t *testing.T) {
	const address = "grace.hopper@example.com"

	for _, tenant := range []string{
		"11111111-1111-4111-8111-111111111111",
		"tenant:with:colons",
		"9:9",
		"",
	} {
		got := auth.ResetAddress(auth.ResetPayload(auth.User{
			TenantID: tenant,
			ID:       "00000000-0000-4000-8000-000000000001",
			Email:    address,
			Password: "$argon2id$v=19$m=65536,t=3,p=4$c2FsdHNhbHRzYWx0c2E$aGFzaA",
		}))
		if got != address {
			t.Errorf("with a tenant of %q the address read back as %q, want %q", tenant, got, address)
		}
	}
}

// TestAPayloadThisApplicationDidNotWriteIsRefused.
//
// ResetAddress answers with the empty string rather than guessing. The
// application reads it only out of a payload that has already come through the
// signer, so this is not the control -- but a parser that returned something
// plausible for input it did not understand is one that accepts two spellings of
// one token, and the flow's single-use property is a comparison of exact bytes.
func TestAPayloadThisApplicationDidNotWriteIsRefused(t *testing.T) {
	for _, payload := range []string{
		"",
		"not a payload",
		"1:a|1:b|1:c",
		// The four fields, and then something after them.
		"1:a1:b3:c@d1:e trailing",
		// A length that reaches past the end of what follows it.
		"99:a",
		// A length that is not a number.
		"x:a",
	} {
		if got := auth.ResetAddress(payload); got != "" {
			t.Errorf("payload %q was read as an address of %q, want the empty string", payload, got)
		}
	}
}
