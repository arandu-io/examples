// This program must NOT compile.
//
// The second negative fixture: even knowing the shape of security.Grant, code
// outside that package cannot build a valid one, because every field is
// unexported. The zero value is the only Grant available anywhere else, and it
// fails Check at runtime.
package main

import (
	"context"

	"github.com/arandu-io/examples/modules/customer"
	"github.com/arandu-io/framework/security"
)

func main() {
	var repo *customer.Repo

	// Forging a valid Grant: the fields are unexported, so this does not compile.
	forged := security.Grant{valid: true, action: customer.ActionView}

	_, _ = repo.Find(context.Background(), forged, "some-id")
}
