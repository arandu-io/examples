package feature_test

import (
	"context"
	"html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/arandu-io/framework/modules/auth"

	"github.com/arandu-io/examples/bootstrap"
)

// SESSION_DRIVER says where session state is kept, and the proof that it does
// is not that the value parses.
//
// It parsed, it validated, it had an error message of its own -- and the wiring
// built the in-process backend whatever it said. A deployment that asked for
// shared sessions got one session store per replica, reported itself healthy,
// and signed half its visitors out on every request. So what is checked below
// is behaviour: what the in-process backend actually does to a second instance,
// and a boot that refuses the drivers this application cannot deliver.

// hiddenField reads one hidden input out of a rendered form.
var hiddenField = regexp.MustCompile(`<input type="hidden" name="_csrf" value="([^"]*)"`)

// signInOn drives the sign-in form of one instance and returns the cookies a
// browser would be holding afterwards.
//
// Through the form and not through the service, because the service
// authenticates and writes no session at all: the handler is what rotates the
// id and puts it in the backend, which is the write these tests are about.
func signInOn(t *testing.T, handler http.Handler, email, password string) []*http.Cookie {
	t.Helper()

	form := httptest.NewRecorder()
	handler.ServeHTTP(form, httptest.NewRequest(http.MethodGet, "/auth/login", nil))
	if form.Code != http.StatusOK {
		t.Fatalf("GET /auth/login = %d, want 200. Body:\n%s", form.Code, form.Body.String())
	}
	token := hiddenField.FindStringSubmatch(form.Body.String())
	if token == nil {
		t.Fatalf("the sign-in form carries no _csrf field, so nothing below can post to it:\n%s", form.Body.String())
	}

	body := url.Values{
		"email":    {email},
		"password": {password},
		"_csrf":    {html.UnescapeString(token[1])},
	}
	post := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(body.Encode()))
	post.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// The token is issued against the session the form set, so the post carries
	// it back. Without this the answer is 419 and nothing below is about
	// sessions any more.
	for _, c := range form.Result().Cookies() {
		post.AddCookie(c)
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, post)
	if rec.Code < 300 || rec.Code > 399 {
		t.Fatalf("POST /auth/login = %d, want a redirect. Body:\n%s", rec.Code, rec.Body.String())
	}

	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("the sign-in set no cookie, so there is no session for anything to read")
	}
	return cookies
}

// guardedPage asks one instance for the page that exists for signed-in people,
// as the holder of these cookies.
//
// The guarded one rather than the front page, because the front page renders
// for everybody and tells the two states apart only by what is drawn in it. A
// route behind RequireAuth answers with a status, and a status is not a string
// somebody can rename.
func guardedPage(t *testing.T, handler http.Handler, cookies []*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()

	r := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	for _, c := range cookies {
		r.AddCookie(c)
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, r)
	return rec
}

// TestASessionWrittenByOneInstanceIsNotReadByTheOther.
//
// What SESSION_DRIVER=memory means, shown rather than described: two instances
// over one database, a sign-in on the first, and the second refusing the same
// cookie. Behind a load balancer that is half the requests, and it is what the
// wiring used to do for every value of the setting -- including the one that
// asked for the opposite.
//
// It is the harness the shared driver would flip: the day this application can
// build a store both instances read, the second assertion becomes 200 and the
// test becomes the proof that it does.
func TestASessionWrittenByOneInstanceIsNotReadByTheOther(t *testing.T) {
	sqliteEnv(t)
	t.Setenv("CACHE_STORE", "memory")
	t.Setenv("SESSION_DRIVER", "memory")
	t.Setenv("REDIS_URL", "")

	if err := bootstrap.Dispatch("migrate", nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	first := bootedInstance(t)
	second := bootedInstance(t)

	const (
		email    = "ana@example.test"
		password = "a-long-enough-password"
	)
	if _, err := first.Auth.Register(context.Background(), bootstrap.Tenant(), auth.RegisterRequest{
		Name:                 "Ana",
		Email:                email,
		Password:             password,
		PasswordConfirmation: password,
	}); err != nil {
		t.Fatalf("registering: %v", err)
	}

	cookies := signInOn(t, first.Kernel.Handler(), email, password)

	// The assertion can tell the two states apart. Without this the test would
	// pass just as well against a guarded page that turns everybody away.
	if rec := guardedPage(t, first.Kernel.Handler(), cookies); rec.Code != http.StatusOK {
		t.Fatalf("the instance that signed this person in answered %d on the guarded page, "+
			"so nothing below distinguishes anything", rec.Code)
	}

	if rec := guardedPage(t, second.Kernel.Handler(), cookies); rec.Code == http.StatusOK {
		t.Fatal("the second instance admitted a session the first one wrote inside its own process: " +
			"either the backend is not the in-process one, or the guard is not reading a session")
	}
}

// TestASessionOverAStoreThisApplicationCannotBuildIsRefusedAtTheBoot.
//
// SESSION_DRIVER=kv named the store every replica reads, and this application
// defines no such store. Before, that combination loaded, validated, booted,
// and produced the deployment the test above measures -- with nothing anywhere
// saying so.
//
// It has to refuse rather than fall back. An in-process backend satisfies the
// type it is handed to and none of what was asked for, and the deployment it
// produces reports itself healthy.
func TestASessionOverAStoreThisApplicationCannotBuildIsRefusedAtTheBoot(t *testing.T) {
	sqliteEnv(t)
	t.Setenv("CACHE_STORE", "memory")
	t.Setenv("SESSION_DRIVER", "kv")
	t.Setenv("REDIS_URL", "redis://127.0.0.1:6379")

	cfg, db, _ := openForTest(t)

	if _, err := bootstrap.Build(cfg, db); err == nil {
		t.Fatal("the application started with its sessions in a store no other replica can read")
	} else {
		for _, want := range []string{"SESSION_DRIVER", `"kv"`, `"redis"`, "REDIS_URL"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("the refusal does not name %s, and whoever reads it has to guess which two settings disagree: %v", want, err)
			}
		}
	}
}

// TestAnUnknownSessionDriverIsRefusedAtTheBoot.
//
// The same door, from the other side: a driver nobody recognises must not fall
// through to the in-process backend. Falling through is how a typo becomes a
// fleet of replicas each holding its own sessions, with nothing in the log.
//
// Load refuses the value first, and a configuration assembled in Go skips Load
// entirely -- which is every test in this repository. The refusal that matters
// is the one in the wiring, because it is the one nothing can go around.
func TestAnUnknownSessionDriverIsRefusedAtTheBoot(t *testing.T) {
	sqliteEnv(t)
	t.Setenv("CACHE_STORE", "memory")
	t.Setenv("REDIS_URL", "")

	cfg, db, _ := openForTest(t)
	cfg.Session.Driver = "kev"

	if _, err := bootstrap.Build(cfg, db); err == nil {
		t.Fatal("the application started on a session driver it does not implement")
	} else if !strings.Contains(err.Error(), "SESSION_DRIVER") || !strings.Contains(err.Error(), `"kev"`) {
		t.Errorf("the refusal names neither the setting nor the value: %v", err)
	}
}
