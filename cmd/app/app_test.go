package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arandu-io/framework/config"
	"github.com/arandu-io/framework/security"
)

// These tests run the real thing against a real database, because SQLite is a
// file: migrate, seed, sign in, walk the tour. Nothing installed, no service
// container, and no mock standing in for the part that matters.

func env(t *testing.T) {
	t.Helper()
	t.Setenv("APP_ENV", "dev")
	t.Setenv("APP_KEY", "0123456789abcdef0123456789abcdef")
	t.Setenv("DB_CONNECTION", "sqlite")
	t.Setenv("DB_DATABASE", filepath.Join(t.TempDir(), "test.sqlite"))
	t.Setenv("ARANDU_ADMIN_EMAIL", "admin@example.test")
	t.Setenv("ARANDU_ADMIN_PASSWORD", "a-long-enough-password")
}

// prepared migrates, seeds and returns a signed-in client against the real app.
func prepared(t *testing.T) (http.Handler, []*http.Cookie) {
	t.Helper()
	env(t)

	if err := dispatch("migrate", nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := dispatch("db:seed", nil); err != nil {
		t.Fatalf("db:seed: %v", err)
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	db, closeDB, err := open(cfg)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(closeDB)

	app := build(cfg, db)
	if err := app.kernel.Boot(context.Background()); err != nil {
		t.Fatalf("Boot: %v", err)
	}
	handler := app.kernel.Handler()

	form := httptest.NewRecorder()
	handler.ServeHTTP(form, httptest.NewRequest(http.MethodGet, "/auth/login", nil))
	token := csrfToken(t, form.Body.String())

	body := url.Values{
		"_csrf":    {token},
		"email":    {"admin@example.test"},
		"password": {"a-long-enough-password"},
	}
	req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(body.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	login := httptest.NewRecorder()
	handler.ServeHTTP(login, req)

	if login.Code != http.StatusOK {
		t.Fatalf("login status = %d, want 200", login.Code)
	}
	return handler, login.Result().Cookies()
}

func get(t *testing.T, h http.Handler, path string, cookies []*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, path, nil)
	for _, c := range cookies {
		r.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

// TestSeededDataIsScopedToTheTenant is the isolation claim against real rows: the
// seeder writes into two tenants, and a signed-in subject sees only one of them.
func TestSeededDataIsScopedToTheTenant(t *testing.T) {
	handler, cookies := prepared(t)

	rec := get(t, handler, "/customers?limit=100", cookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	var customers []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &customers); err != nil {
		t.Fatalf("decoding: %v -- body: %s", err, rec.Body.String())
	}

	if len(customers) != 6 {
		t.Fatalf("got %d customers, want the 6 of this tenant (the other tenant has 2 more)", len(customers))
	}
	for _, c := range customers {
		if strings.Contains(c["name"].(string), "Concorrente") {
			t.Fatalf("a customer from the other tenant leaked: %v", c)
		}
	}
}

// TestTheDocumentIsNotInTheListing: the entity refuses to serialize it, so no
// handler has to remember.
func TestTheDocumentIsNotInTheListing(t *testing.T) {
	handler, cookies := prepared(t)

	rec := get(t, handler, "/customers", cookies)

	if strings.Contains(rec.Body.String(), "11222333000181") {
		t.Fatalf("the document reached the response: %s", rec.Body.String())
	}
}

// TestUnauthenticatedIsRefused covers the door itself.
func TestUnauthenticatedIsRefused(t *testing.T) {
	handler, _ := prepared(t)

	rec := get(t, handler, "/customers", nil)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

// TestTheTourIsWalkable checks every demonstration route end to end, because a
// guided tour with a broken stop is worse than no tour.
func TestTheTourIsWalkable(t *testing.T) {
	handler, cookies := prepared(t)

	cases := []struct {
		path       string
		wantStatus int
		wantBody   string
	}{
		{"/demo", http.StatusOK, "guided tour"},
		{"/demo/batched", http.StatusOK, "2 queries"},
		{"/demo/other-tenant", http.StatusOK, "not found"},
		{"/demo/no-grant", http.StatusOK, "It does not compile"},
		// The three below panic on purpose, and the debug page renders them.
		{"/demo/n-plus-one", http.StatusInternalServerError, "Likely N"},
		{"/demo/slow-query", http.StatusInternalServerError, "Check the index"},
		{"/demo/panic", http.StatusInternalServerError, "deliberate"},
		// Dump-and-die is a deliberate stop, not a failure.
		{"/demo/dump", http.StatusOK, "stopping here on purpose"},
	}

	for _, c := range cases {
		t.Run(c.path, func(t *testing.T) {
			rec := get(t, handler, c.path, cookies)
			if rec.Code != c.wantStatus {
				t.Fatalf("status = %d, want %d\n%s", rec.Code, c.wantStatus, truncate(rec.Body.String()))
			}
			if !strings.Contains(rec.Body.String(), c.wantBody) {
				t.Fatalf("body does not mention %q:\n%s", c.wantBody, truncate(rec.Body.String()))
			}
		})
	}
}

// TestTheNPlusOnePageCarriesTheEvidence is the promise of the observability
// pillar: nobody instrumented this route, and the page still names the problem,
// shows the repeated statement, and carries the stack that reached it.
//
// Note what is asserted and what is not: the Origin column points at the
// repository that issued the query, and the loop that called it six times shows
// up in the stack. Pointing the Origin column at the loop would need the
// Collector to keep several frames -- a phase 3 improvement, not a claim to make
// today.
func TestTheNPlusOnePageCarriesTheEvidence(t *testing.T) {
	handler, cookies := prepared(t)

	body := get(t, handler, "/demo/n-plus-one", cookies).Body.String()

	if !strings.Contains(body, "FROM invoices") {
		t.Error("the repeated query is not on the page")
	}
	if !strings.Contains(body, "modules/invoice/invoice.go") {
		t.Error("the page does not name the repository that issued the repeated query")
	}
	if !strings.Contains(body, "modules/demo/module.go") {
		t.Error("the stack does not reach the loop that caused the N+1")
	}
	if !strings.Contains(body, "Likely N") {
		t.Error("the page shows the data but not the diagnosis")
	}
}

// TestTheTourIsDevelopmentOnly: it panics on purpose and prints what a policy
// refused, so it must not exist in production.
func TestTheTourIsDevelopmentOnly(t *testing.T) {
	env(t)
	t.Setenv("APP_ENV", "prod")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	db, closeDB, err := open(cfg)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(closeDB)

	app := build(cfg, db)
	if err := app.kernel.Boot(context.Background()); err != nil {
		t.Fatalf("Boot: %v", err)
	}

	rec := get(t, app.kernel.Handler(), "/demo", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 outside development", rec.Code)
	}
}

// TestSeedingIsIdempotent: a seeder that fails on the second run cannot be part
// of a deploy.
func TestSeedingIsIdempotent(t *testing.T) {
	env(t)
	if err := dispatch("migrate", nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	for i := range 3 {
		if err := dispatch("db:seed", nil); err != nil {
			t.Fatalf("db:seed run %d: %v", i+1, err)
		}
	}
}

func TestMigrationCommands(t *testing.T) {
	env(t)

	for _, command := range []string{"migrate", "migrate:status", "migrate:rollback", "migrate:fresh"} {
		if err := dispatch(command, nil); err != nil {
			t.Fatalf("%s: %v", command, err)
		}
	}
}

// TestSystemGrantWithoutTenantIsUseless guards RULE 14 from the application side.
func TestSystemGrantWithoutTenantIsUseless(t *testing.T) {
	if err := security.SystemGrant("customer.view", "").Check("customer.view"); err == nil {
		t.Fatal("a system grant with no tenant passed Check")
	}
}

func csrfToken(t *testing.T, html string) string {
	t.Helper()
	const marker = `name="_csrf" value="`
	i := strings.Index(html, marker)
	if i < 0 {
		t.Fatalf("no CSRF field in the form:\n%s", html)
	}
	rest := html[i+len(marker):]
	end := strings.IndexByte(rest, '"')
	if end < 0 {
		t.Fatal("the CSRF field is not terminated")
	}
	return rest[:end]
}

func truncate(s string) string {
	if len(s) > 600 {
		return s[:600] + "…"
	}
	return s
}
