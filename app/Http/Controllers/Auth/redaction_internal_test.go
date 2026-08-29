package authui

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestInternalAuthenticationStateRedactsDiagnostics(t *testing.T) {
	tests := []struct {
		name    string
		value   any
		secrets []string
		safe    []string
		markers int
	}{
		{
			name: "registration input",
			value: registrationInput{
				Name: "Ada", Email: "ada@example.test",
				Password: "registration-secret", PasswordConfirmation: "confirmation-secret",
			},
			secrets: []string{"registration-secret", "confirmation-secret"},
			safe:    []string{"Ada", "ada@example.test"}, markers: 2,
		},
		{
			name: "pending second factor",
			value: pendingSignIn{
				Tenant: "tenant-a", UserID: "user-a",
				PasswordFingerprint: "password-state-fingerprint", Remember: true,
			},
			secrets: []string{"password-state-fingerprint"},
			safe:    []string{"tenant-a", "user-a"}, markers: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			encoded, err := json.Marshal(test.value)
			if err != nil {
				t.Fatalf("marshalling diagnostic state: %v", err)
			}
			var log bytes.Buffer
			slog.New(slog.NewTextHandler(&log, nil)).Info("state", "value", test.value)
			for name, output := range map[string]string{"json": string(encoded), "log": log.String()} {
				for _, secret := range test.secrets {
					if strings.Contains(output, secret) {
						t.Errorf("%s exposed %q: %s", name, secret, output)
					}
				}
				for _, safe := range test.safe {
					if !strings.Contains(output, safe) {
						t.Errorf("%s omitted %q: %s", name, safe, output)
					}
				}
				if got := strings.Count(output, "[redacted]"); got != test.markers {
					t.Errorf("%s redaction markers = %d, want %d: %s", name, got, test.markers, output)
				}
			}
		})
	}
}
