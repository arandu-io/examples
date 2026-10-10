package feature_test

import (
	"context"
	"html"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/arandu-io/hesape/database"

	"github.com/arandu-io/examples/bootstrap"
	"github.com/arandu-io/examples/tests"
)

// configuredName is the APP_NAME these tests boot with.
//
// It is a word no screen is titled with and no fixture writes, and it is not
// arandu-app, which is what the configuration answers when APP_NAME is unset:
// a brand slot holding the default would pass a test that booted with the
// default. Only a value that can have come from nowhere but the configuration
// proves the configuration reached the page.
const configuredName = "Quillstone Ledger"

// The two places the layout writes the brand: the word beside the mark in the
// masthead, and the og:site_name a link preview names the site by. Each is
// read out of its own slot rather than searched for anywhere in the body --
// the landing page is titled with the application's name, so a body that
// carries it in <title> and nowhere else would satisfy a plain search.
var (
	brandWord = regexp.MustCompile(`<span class="brand-word">([^<]*)</span>`)
	siteName  = regexp.MustCompile(`<meta property="og:site_name" content="([^"]*)">`)
	pageTitle = regexp.MustCompile(`<title>([^<]*)</title>`)
)

// landing is the two addresses HomeController answers. It titles the page with
// the application's name, as the starter kit publishes it, so on these two the
// title and the brand hold the same string and only the slots tell them apart.
var landing = map[string]bool{"/": true, "/dashboard": true}

// TestEveryPageCarriesTheConfiguredApplicationName.
//
// The framework puts APP_NAME on every request before any route runs, and the
// header reads it there: no controller is handed the name, and none assigns it.
// So this boots with the name set and reads the brand on every screen a person
// can open -- the blog's and the sign-in kit's, as a guest and signed in --
// because a screen whose chrome was filled some other way is exactly the one
// that would draw a blank corner, and nothing else on the page would show it.
func TestEveryPageCarriesTheConfiguredApplicationName(t *testing.T) {
	t.Setenv("APP_NAME", configuredName)
	client, db := tests.App(t)
	post := seedJourneyPost(t, db)
	section := seedBrandSection(t, db)

	check := func(path, body string) {
		t.Helper()
		title := slot(pageTitle, body)
		if title == "" {
			t.Errorf("%s: the page has no title, so it was not drawn by the layout", path)
			return
		}
		if !landing[path] && title == configuredName {
			t.Errorf("%s: the page is titled %q, the configured name, so the brand cannot be told from the title", path, title)
		}
		if got := slot(brandWord, body); got != configuredName {
			t.Errorf("%s: the masthead brand is %q, want the configured APP_NAME %q", path, got, configuredName)
		}
		if got := slot(siteName, body); got != configuredName {
			t.Errorf("%s: og:site_name is %q, want the configured APP_NAME %q", path, got, configuredName)
		}
	}

	// The two challenge screens of a second factor are not here: they open only
	// in the middle of a sign-in to an account that has one, and they are drawn
	// by the same function as every other screen of the kit below.
	guest := []string{
		"/", "/posts", "/posts/" + post, "/c/brand-section",
		"/auth/login", "/auth/register", "/auth/password", "/auth/password/reset", "/auth/verify",
	}
	for _, path := range guest {
		check(path, client.Get(path).OK().Body())
	}

	signInAs(t, client, db, "Ada Lovelace", "admin")
	signedIn := []string{
		"/", "/dashboard", "/posts", "/posts/create", "/posts/" + post, "/posts/" + post + "/edit",
		"/c/brand-section", "/categories", "/categories/create",
		"/categories/" + section, "/categories/" + section + "/edit",
		"/auth/password/confirm",
	}
	for _, path := range signedIn {
		check(path, client.Get(path).OK().Body())
	}

	// Setting up a second factor asks for the password again first.
	client.Post("/auth/password/confirm", map[string]string{"password": accountPassword}).Status(303)
	check("/auth/two-factor/setup", client.Get("/auth/two-factor/setup").OK().Body())

	// The moderation area is the one place that draws another brand, and on
	// purpose: its sidebar is headed "Admin", which is what the framework
	// allows a page to do over the name it carries. It is held here so the
	// exception is stated rather than inferred from the list above.
	for _, path := range []string{"/admin/", "/admin/comments", "/admin/sockets"} {
		body := client.Get(path).OK().Body()
		if !strings.Contains(body, ">Admin</a>") {
			t.Errorf("%s: the moderation area no longer draws its own brand", path)
		}
	}
}

// slot is the first capture of re in body, unescaped, or empty.
func slot(re *regexp.Regexp, body string) string {
	m := re.FindStringSubmatch(body)
	if m == nil {
		return ""
	}
	return html.UnescapeString(m[1])
}

// seedBrandSection writes one section of the tenant the application serves,
// for the screens that draw one, and answers its id.
func seedBrandSection(t *testing.T, db *database.DB) string {
	t.Helper()

	const id = "00000000-0000-4000-8000-0000000000cc"
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO categories (id, tenant_id, name, slug, description, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		id, bootstrap.Tenant(), "A section", "brand-section", "A section.", time.Now())
	if err != nil {
		t.Fatalf("seeding a section: %v", err)
	}
	return id
}
