package feature_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/arandu-io/hesape/auth"
	"github.com/arandu-io/hesape/database"
	"github.com/arandu-io/hesape/log"

	"github.com/arandu-io/examples/bootstrap"
)

// The console, through the real pipeline. Everything below goes through
// k.Handler(), which is the same handler `aru serve` binds to a port -- a test
// that drove the console directly would prove the console and not the wiring,
// and the wiring is where a Collector goes missing.

func bootedApp(t *testing.T) (http.Handler, *database.DB) {
	t.Helper()
	sqliteEnv(t)

	if err := bootstrap.Dispatch("migrate", nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	cfg, db, _ := openForTest(t)
	app, err := bootstrap.Build(cfg, db)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	k := app.Kernel
	if err := k.Boot(context.Background()); err != nil {
		t.Fatalf("boot: %v", err)
	}
	t.Cleanup(func() { _ = k.Shutdown() })
	return k.Handler(), db
}

// TestTheConsoleRecordsARealRequest is the shape of a debugging session: make a
// request, open the console, find it.
func TestTheConsoleRecordsARealRequest(t *testing.T) {
	handler, _ := bootedApp(t)

	first := httptest.NewRecorder()
	handler.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/auth/login", nil))
	id := first.Header().Get("X-Request-ID")
	if id == "" {
		t.Fatal("the response carries no request id")
	}

	list := httptest.NewRecorder()
	handler.ServeHTTP(list, httptest.NewRequest(http.MethodGet, log.ConsolePath, nil))
	if list.Code != http.StatusOK {
		t.Fatalf("the console answered %d", list.Code)
	}
	if !strings.Contains(list.Body.String(), "/auth/login") {
		t.Errorf("the request is not in the console:\n%s", list.Body.String())
	}

	detail := httptest.NewRecorder()
	handler.ServeHTTP(detail, httptest.NewRequest(http.MethodGet, log.ConsolePath+"/"+id, nil))
	if detail.Code != http.StatusOK {
		t.Fatalf("the detail page answered %d", detail.Code)
	}
	for _, want := range []string{id, "Timeline", "/auth/login"} {
		if !strings.Contains(detail.Body.String(), want) {
			t.Errorf("the detail page does not show %q", want)
		}
	}
}

// TestTheConsoleSeesTheQueriesOfTheRequest: the Collector reaching the recorder
// through the whole pipeline is the thing that breaks silently, and when it
// breaks the console shows a request with no queries -- which reads like the
// application not touching the database.
func TestTheConsoleSeesTheQueriesOfTheRequest(t *testing.T) {
	handler, _ := bootedApp(t)

	// A request that queries: the login form issues none, so this drives one
	// through a route that does.
	// The form first, for the CSRF token: without it the POST is refused by the
	// middleware and never reaches the database, which would make this test
	// pass or fail for the wrong reason.
	form := httptest.NewRecorder()
	handler.ServeHTTP(form, httptest.NewRequest(http.MethodGet, "/auth/login", nil))

	rec := post(t, handler, form, "nobody@example.test", "a-long-enough-password")
	// 303 back to the form is the refusal; a 419 would be the CSRF middleware
	// turning the request away before the handler ran.
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/auth/login" {
		t.Fatalf("the login was not attempted: status %d, Location %q", rec.Code, rec.Header().Get("Location"))
	}
	id := rec.Header().Get("X-Request-ID")

	detail := httptest.NewRecorder()
	handler.ServeHTTP(detail, httptest.NewRequest(http.MethodGet, log.ConsolePath+"/"+id+"?format=json", nil))

	body := detail.Body.String()
	if !strings.Contains(body, `from \"users\"`) {
		t.Errorf("the console shows no query for a login attempt:\n%s", body)
	}
	// The origin proves the query crossed the native Hesape model bridge rather
	// than the removed Framework auth module.
	if !strings.Contains(body, "database/dbmodel.go") {
		t.Errorf("the query has no origin pointing at the native model bridge:\n%s", body)
	}
}

func TestTheGrantStillComesFromTheSession(t *testing.T) {
	g := auth.SystemGrant("user.view", bootstrap.Tenant())
	if auth.Tenant(g) != bootstrap.Tenant() {
		t.Fatal("the tenant no longer comes from the Grant")
	}
}
