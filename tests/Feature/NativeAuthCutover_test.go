package feature_test

import (
	"context"
	"strings"
	"testing"

	models "github.com/arandu-io/examples/app/Models"
	"github.com/arandu-io/examples/bootstrap"
	"github.com/arandu-io/examples/tests"
)

// This is the release regression for the cutover: the route must render
// through the application-owned auth controller and the native Hesape layout
// contract. The previous mixed framework/native wiring failed here with 500.
func TestNativeRegistrationRouteRendersSuccessfully(t *testing.T) {
	client, _ := tests.App(t)
	client.Get("/auth/register").OK().See("Create an account")
}

// The setup endpoint crosses every native seam: application-owned account and
// factor services, Hesape OTP provisioning, Hesape QR encoding, and the
// application-owned view. A placeholder or legacy Framework module cannot
// produce the SVG this assertion reaches.
func TestNativeTwoFactorSetupRendersARealQRCode(t *testing.T) {
	booted := tests.Boot(t)
	const (
		email    = "qr-reader@example.test"
		password = "a-long-enough-password"
	)
	if _, err := booted.App.Users.EnsureUser(context.Background(), bootstrap.Tenant(),
		"QR Reader", email, password, []string{models.RoleMember}, true); err != nil {
		t.Fatalf("seeding the verified account: %v", err)
	}

	booted.Client.Get("/auth/login").OK()
	booted.Client.Post("/auth/login", map[string]string{
		"email": email, "password": password,
	}).RedirectsTo("/")
	booted.Client.Get("/auth/two-factor/setup").RedirectsTo("/auth/password/confirm")
	booted.Client.Get("/auth/password/confirm").OK()
	booted.Client.Post("/auth/password/confirm", map[string]string{
		"password": password,
	}).RedirectsTo("/auth/two-factor/setup")
	booted.Client.Get("/auth/two-factor/setup").OK()
	body := booted.Client.Post("/auth/two-factor/setup", nil).OK().Body()
	if !strings.Contains(body, "<svg") || !strings.Contains(body, "type this key") {
		t.Fatalf("native setup did not render QR provisioning material: %s", first200(body))
	}
}
