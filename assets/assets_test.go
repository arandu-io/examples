package assets_test

import (
	"crypto/md5"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	fhttp "github.com/arandu-io/framework/http"
	"github.com/arandu-io/framework/view"

	_ "github.com/arandu-io/examples/assets"
)

// TestTheBrowserGetsThisProjectsStylesheet is the regression guard for a whole
// pipeline whose output nobody consumed.
//
// `aru view:build` ran Tailwind over resources/css/app.css and wrote
// assets/app.css, and nothing embedded it. The browser kept receiving the
// framework's default -- byte for byte, same md5 -- so every class written in a
// view of this project was absent from the page, with no error anywhere.
//
// The check is the md5 of what comes over HTTP against the md5 of the file on
// disk. Anything weaker passes in the broken state: the framework's default is
// also valid CSS, also served with a 200, and also has Tailwind's banner.
func TestTheBrowserGetsThisProjectsStylesheet(t *testing.T) {
	onDisk, err := os.ReadFile("app.css")
	if err != nil {
		t.Fatalf("assets/app.css is missing: it is committed so that `go build` works on a fresh clone: %v", err)
	}

	r := fhttp.NewRouter()
	view.NewModule().Routes(r)
	server := httptest.NewServer(r)
	defer server.Close()

	resp, err := http.Get(server.URL + view.URL(view.Stylesheet))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	served, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("the stylesheet answered %d", resp.StatusCode)
	}

	if sum(served) != sum(onDisk) {
		t.Errorf("the browser is served a different stylesheet than assets/app.css.\n  served   %s (%d bytes)\n  on disk  %s (%d bytes)\nThis is the framework's default: nothing registered this project's.",
			sum(served), len(served), sum(onDisk), len(onDisk))
	}
}

func sum(b []byte) string {
	h := md5.Sum(b)
	return hex.EncodeToString(h[:])
}

// TestTheStylesheetCarriesTheClassesTheMarkupRenders is the guard for the half
// of the pipeline the test above cannot see.
//
// That one proves the browser is served this project's stylesheet. It says
// nothing about what is inside it, and for weeks the answer was: not the classes
// the components render. Tailwind reads class names only out of the files an
// @source names, resources/css/app.css can only name directories of this
// project, and the components are an imported module (ADR 0027) whose source
// lives in the module cache. So .text-destructive appeared zero times in 193 KB
// of compiled CSS while components.Field wrote it on the error line of every
// form: a rejected sign-in drew "that password is wrong" in the same colour as
// the label above it. The theme picker's swatches were `size-3 rounded-full`
// spans with no rule for either, which is a span of no size -- an empty menu
// with six invisible entries.
//
// Nothing failed. The build was green, `aru view:build` reported success, and
// the page rendered. That is why this test exists and why it names the class
// rather than the component: the class is what has to reach the file.
func TestTheStylesheetCarriesTheClassesTheMarkupRenders(t *testing.T) {
	css, err := os.ReadFile("app.css")
	if err != nil {
		t.Fatalf("assets/app.css is missing: it is committed so that `go build` works on a fresh clone: %v", err)
	}
	stylesheet := string(css)

	for _, want := range []struct {
		class  string
		drawn  string
		broken string
	}{
		{".text-destructive", "the validation message under a components.Field",
			"a rejected form explains itself in the body colour, so nothing on the screen says which field was refused"},
		{".w-44", "the menu of components.ThemeToggle",
			"the theme menu has no width and collapses onto its trigger"},
		{".size-3", "each colour swatch of components.ThemeToggle",
			"the swatches have no size, so the menu offers six choices with nothing to look at"},
		{".rounded-full", "each colour swatch of components.ThemeToggle",
			"the swatches are squares"},
		{".min-h-dvh", "the page column in layouts/app",
			"a short page stops at its content and the footer floats halfway up the window"},
		{".text-right", "every number in a components.StatCard, on admin/sockets",
			"the counts sit against the left edge under headings that are right-aligned, so no column lines up with the one above it"},
	} {
		if !strings.Contains(stylesheet, want.class) {
			t.Errorf("%s is not in the compiled stylesheet, and it is what draws %s: %s.\nRun `aru view:build`, and if that does not put it there, the file it is written in is not one the stylesheet declares as a source.",
				want.class, want.drawn, want.broken)
		}
	}
}
