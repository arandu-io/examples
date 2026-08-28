package feature_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/arandu-io/examples/bootstrap"
)

// The limit the pipeline declares, per minute and per caller.
//
// Written out here rather than read from anywhere, because a test that asked
// the code what the limit was would agree with it whatever it became. If this
// number and the wiring disagree, one of them is a decision somebody took
// without the other.
const requestsPerMinute = 300

// bootedInstance builds and boots one instance of the application.
//
// One instance, built the way the commands build it. Everything that is
// per-process is separate between two of them, and everything they share, they
// share through the stores the configuration named.
func bootedInstance(t *testing.T) bootstrap.App {
	t.Helper()

	cfg, db, _ := openForTest(t)
	app, err := bootstrap.Build(cfg, db)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if err := app.Kernel.Boot(context.Background()); err != nil {
		t.Fatalf("Boot: %v", err)
	}
	t.Cleanup(func() { _ = app.Kernel.Shutdown() })
	return app
}

// countedRequest asks for a page that costs nothing to render and returns what
// the application answered.
//
// Any route does: the throttle is global middleware, so it counts before the
// router has chosen anything. The cheapest one keeps the loop below to the
// thing being measured, and it reaches no database.
func countedRequest(t *testing.T, handler http.Handler) *httptest.ResponseRecorder {
	t.Helper()

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/robots.txt", nil))
	return rec
}

// TestTheLimitIsCountedAndReportedOnEveryAnswer.
//
// The headers are not decoration: a client that has to back off reads them, and
// they are the only way to tell "this application has a limit" from "this
// application had one and it never counted".
func TestTheLimitIsCountedAndReportedOnEveryAnswer(t *testing.T) {
	sqliteEnv(t)
	t.Setenv("CACHE_STORE", "memory")
	t.Setenv("REDIS_URL", "")

	if err := bootstrap.Dispatch("migrate", nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	app := bootedInstance(t)

	first := countedRequest(t, app.Kernel.Handler())
	if first.Code != http.StatusOK {
		t.Fatalf("the first request answered %d", first.Code)
	}
	if got := first.Header().Get("X-RateLimit-Limit"); got != strconv.Itoa(requestsPerMinute) {
		t.Fatalf("X-RateLimit-Limit = %q, want %d: nothing counted this request", got, requestsPerMinute)
	}

	remaining, err := strconv.Atoi(first.Header().Get("X-RateLimit-Remaining"))
	if err != nil {
		t.Fatalf("X-RateLimit-Remaining = %q: %v", first.Header().Get("X-RateLimit-Remaining"), err)
	}
	if remaining != requestsPerMinute-1 {
		t.Fatalf("X-RateLimit-Remaining = %d after one request, want %d", remaining, requestsPerMinute-1)
	}

	// It goes down. A counter reporting the same number on every answer is a
	// counter that is not counting, and the header alone cannot tell them apart.
	second := countedRequest(t, app.Kernel.Handler())
	next, err := strconv.Atoi(second.Header().Get("X-RateLimit-Remaining"))
	if err != nil {
		t.Fatalf("X-RateLimit-Remaining = %q: %v", second.Header().Get("X-RateLimit-Remaining"), err)
	}
	if next != remaining-1 {
		t.Fatalf("X-RateLimit-Remaining went %d then %d: the budget is not being spent", remaining, next)
	}
}

// TestTheBudgetRunsOutAndTheRefusalCanBeActedOn.
//
// The budget is spent to the last request and the next one is turned away. A
// limit that reports a number and never refuses is a limit in the headers only,
// and the counting test above passes against exactly that.
func TestTheBudgetRunsOutAndTheRefusalCanBeActedOn(t *testing.T) {
	sqliteEnv(t)
	t.Setenv("CACHE_STORE", "memory")
	t.Setenv("REDIS_URL", "")

	if err := bootstrap.Dispatch("migrate", nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	app := bootedInstance(t)
	handler := app.Kernel.Handler()

	// The whole budget, as the same anonymous caller: the key falls back to the
	// peer address, and httptest hands every request the same one.
	for i := range requestsPerMinute {
		if rec := countedRequest(t, handler); rec.Code != http.StatusOK {
			t.Fatalf("request %d of the budget answered %d, and the budget is %d", i+1, rec.Code, requestsPerMinute)
		}
	}

	rec := countedRequest(t, handler)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("the request after the budget answered %d, want 429: the limit counts and never refuses", rec.Code)
	}
	if got := rec.Header().Get("Retry-After"); got == "" {
		t.Error("the refusal says to wait and does not say how long")
	}
	if got := rec.Header().Get("X-RateLimit-Limit"); got != strconv.Itoa(requestsPerMinute) {
		t.Errorf("X-RateLimit-Limit = %q, want %d", got, requestsPerMinute)
	}
	if got := rec.Header().Get("X-RateLimit-Remaining"); got != "0" {
		t.Errorf("X-RateLimit-Remaining = %q on the refusal, want 0", got)
	}

	// The refusal is one a person can act on. htmx swaps no 4xx, so without
	// HX-Refresh somebody presses the button, the screen does not change, and
	// the limit reads as a broken page. The header is asked for the way a
	// browser would ask.
	boosted := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/robots.txt", nil)
	request.Header.Set("HX-Request", "true")
	handler.ServeHTTP(boosted, request)

	if boosted.Code != http.StatusTooManyRequests {
		t.Fatalf("the boosted request answered %d, want 429", boosted.Code)
	}
	if boosted.Header().Get("HX-Refresh") == "" {
		t.Error("the refusal carries no HX-Refresh, so over the limit is a button that does nothing")
	}
}
