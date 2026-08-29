package unit_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arandu-io/examples/tests"
)

// TestTheStylesheetCarriesTheClassesTheMarkupRenders is the guard for the half
// of the pipeline that serving the file cannot see.
//
// The Feature suite proves the browser is served this project's stylesheet. It
// says nothing about what is inside it. Tailwind reads class names only out of
// the files an @source names, resources/css/app.css can only name directories of
// this project, and the components are an imported module whose source lives in
// the module cache -- so a class only a component writes can be absent from the
// compiled CSS while every form renders it. Without .text-destructive, a
// rejected sign-in draws "that password is wrong" in the same colour as the
// label above it; without a rule for `size-3 rounded-full`, the theme picker's
// swatches are spans of no size, an empty menu with six invisible entries.
//
// Nothing fails when that happens. The build is green, `aru view:build` reports
// success, and the page renders. That is why this test exists and why it names
// the class rather than the component: the class is what has to reach the file.
func TestTheStylesheetCarriesTheClassesTheMarkupRenders(t *testing.T) {
	css, err := os.ReadFile(filepath.Join(tests.Root(t), "assets", "app.css"))
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
		{".w-40", "the menu of components.ThemeToggle",
			"the theme menu has no width and collapses onto its trigger"},
		{".size-4", "the icon beside each option of components.ThemeToggle",
			"the icons have no size, so a menu of three modes shows three words and no marks"},
		{".min-h-dvh", "the application page column in layouts/app",
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
