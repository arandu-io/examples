package unit_test

import (
	"strings"
	"testing"

	"github.com/arandu-io/examples/app/Models"
)

// Password-reset state belongs to the application account model. Controllers
// sign this opaque fingerprint instead of the stored password hash, and the
// service compares it again immediately before the conditional update.
func TestPasswordFingerprintBindsTheCompleteCurrentHash(t *testing.T) {
	const first = "$argon2id$v=19$m=65536,t=3,p=4$c2FsdDE$aGFzaDE"
	const second = "$argon2id$v=19$m=65536,t=3,p=4$c2FsdDI$aGFzaDI"

	one := (models.User{Password: first}).PasswordFingerprint()
	two := (models.User{Password: second}).PasswordFingerprint()
	if one == two {
		t.Fatal("different password hashes produced the same reset fingerprint")
	}
	if strings.Contains(one, first) || strings.Contains(two, second) {
		t.Fatal("a reset fingerprint exposed stored password material")
	}
	if got := (models.User{Password: first}).PasswordFingerprint(); got != one {
		t.Fatalf("the same stored hash produced fingerprints %q and %q", one, got)
	}
}
