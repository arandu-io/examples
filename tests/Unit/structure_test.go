package unit_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/arandu-io/examples/tests"
)

// TestEveryTestLivesInTheTestsDirectory.
//
// The suites mean what their names say: tests/Feature boots the application and
// makes a request, tests/Unit checks one thing without booting anything. A
// `_test.go` anywhere else is a source file and its test side by side, doubling
// every listing with files nobody opens on purpose.
//
// One exception, and it is not a preference: a test that reads an unexported
// identifier cannot live in another package. Go decides that, not this project.
// Such a test stays beside the code and says so in its name, _internal_test.go,
// so that "it is next to the code" is a claim the file makes and this test can
// check rather than a habit nobody notices.
//
// The suffix is the whole exception. A list of blessed directories is not: it
// grows one package at a time, each entry with a sentence explaining why that
// one is special, and the rule ends up describing the tree instead of shaping
// it. This repository had "assets" on such a list, justified as a module that
// ships on its own -- and there is one go.mod here, so it was not one.
//
// "tests/" means tests/ of the module the file belongs to, which is why the
// path is measured from the nearest go.mod up the tree rather than from here.
// There is one module in this project today, so the two are the same directory;
// the day a driver gets its own go.mod they stop being, and a rule anchored at
// the project root would then reject that module's own suite for sitting where
// it is supposed to sit.
func TestEveryTestLivesInTheTestsDirectory(t *testing.T) {
	root := tests.Root(t)

	seen := 0
	for _, rel := range goFiles(t, root) {
		if !strings.HasSuffix(rel, "_test.go") {
			continue
		}
		seen++

		inModule, err := relativeToModule(filepath.Join(root, rel), root)
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasPrefix(inModule, "tests/") {
			continue
		}
		if strings.HasSuffix(rel, "_internal_test.go") {
			continue
		}
		t.Errorf("%s is a test outside tests/: name it _internal_test.go if it needs an "+
			"unexported identifier, otherwise move it to tests/Feature if it boots the "+
			"application and tests/Unit if it does not", rel)
	}

	// A check about where tests live proves nothing if it examined no test, and
	// it says so rather than passing. This one walked the project itself until
	// it was found doing exactly that: pointed at an empty directory it reported
	// success, because a rule of the form "every file that is X must also be Y"
	// is satisfied by no files at all. The listing it now shares refuses to come
	// back empty, and this refuses to come back having seen no test.
	if seen == 0 {
		t.Fatal("no _test.go file was examined: this check passes on nothing, so a suite that " +
			"vanished would read as a suite that complies")
	}
}

// relativeToModule is path as its own module sees it: the slash-separated path
// from the nearest directory at or above it that holds a go.mod.
//
// The search stops at root, so a file outside any module is reported relative
// to root rather than walking off into whatever is above the project.
func relativeToModule(path, root string) (string, error) {
	dir := filepath.Dir(path)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			break
		}
		if dir == root || dir == filepath.Dir(dir) {
			dir = root
			break
		}
		dir = filepath.Dir(dir)
	}
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return "", err
	}
	return filepath.ToSlash(rel), nil
}

// TestEveryDirectoryThatMustExistIsKept.
//
// git does not track a directory, only files, so an empty one is not in a clone
// at all -- and storage/framework/cache missing is a runtime failure on the
// first request, not a build error anybody sees.
//
// The two answers are different on purpose: storage keeps a .gitignore that
// ignores everything but itself, because the contents are produced and must
// never be committed; a source directory that starts empty keeps a .gitkeep,
// because its contents are written by hand and belong in git.
func TestEveryDirectoryThatMustExistIsKept(t *testing.T) {
	root := tests.Root(t)

	for _, d := range []struct {
		path string
		file string
	}{
		{"storage/app/private", ".gitignore"},
		{"storage/app/public", ".gitignore"},
		{"storage/framework/cache", ".gitignore"},
		{"storage/framework/sessions", ".gitignore"},
	} {
		body, err := os.ReadFile(filepath.Join(root, d.path, d.file))
		if err != nil {
			t.Errorf("%s has no %s: the directory will not be in a clone, and the first "+
				"request that writes there fails at run time", d.path, d.file)
			continue
		}
		if !strings.Contains(string(body), "*") || !strings.Contains(string(body), "!.gitignore") {
			t.Errorf("%s/%s does not ignore its contents while keeping itself:\n%s", d.path, d.file, body)
		}
	}

	// A source directory that starts empty keeps a .gitkeep. Nothing is produced
	// in these, so ignoring their contents would ignore the code.
	for _, d := range []string{
		"app/Enums", "app/Events", "app/Jobs", "app/Listeners", "app/Mail", "app/Http/Middleware",
	} {
		full := filepath.Join(root, d)
		entries, err := os.ReadDir(full)
		if err != nil {
			t.Errorf("%s does not exist", d)
			continue
		}
		if len(entries) > 0 {
			continue // it has files of its own, and does not need a keeper
		}
		if _, err := os.Stat(filepath.Join(full, ".gitkeep")); err != nil {
			t.Errorf("%s is empty and has no .gitkeep: it will not be in a clone", d)
		}
	}
}

// TestTheApplicationTreeHasOneOwnerPerResponsibility freezes the part of this
// application a reader is allowed to learn as architecture. Required paths are
// active responsibilities, optional paths appear only with specialized code,
// and refused paths would introduce a second owner or a Node build.
func TestTheApplicationTreeHasOneOwnerPerResponsibility(t *testing.T) {
	root := tests.Root(t)

	requiredDirectories := []string{
		"app",
		"app/Http/Controllers",
		"app/Http/Middleware",
		"app/Http/Requests",
		"app/Listeners",
		"app/Mail",
		"app/Models",
		"app/Policies",
		"app/Providers",
		"app/Services",
		"assets",
		"bootstrap",
		"config",
		"database/factories",
		"database/migrations",
		"database/seeders",
		"public",
		"resources/css",
		"resources/views",
		"routes",
		"storage/app/private",
		"storage/app/public",
		"storage/framework/cache",
		"storage/framework/sessions",
		"tests/Feature",
		"tests/Unit",
	}
	for _, path := range requiredDirectories {
		info, err := os.Stat(filepath.Join(root, path))
		switch {
		case err != nil:
			t.Errorf("required directory %s is missing: %v", path, err)
		case !info.IsDir():
			t.Errorf("required directory %s is not a directory", path)
		}
	}

	requiredFiles := []string{
		"go.mod",
		"main.go",
		"arandu.toml",
		"app/Providers/AppServiceProvider.go",
		"assets/assets.go",
		"bootstrap/app.go",
		"bootstrap/console.go",
		"config/app.go",
		"database/migrations/migrations.go",
		"public/public.go",
		"routes/console.go",
		"routes/web.go",
		"tests/testcase.go",
	}
	for _, path := range requiredFiles {
		info, err := os.Stat(filepath.Join(root, path))
		switch {
		case err != nil:
			t.Errorf("required file %s is missing: %v", path, err)
		case info.IsDir():
			t.Errorf("required file %s is a directory", path)
		}
	}

	// Services is the one home of use-case orchestration. Repositories remains
	// available only for specialized queries, and Commands appears with the first
	// structured command. Optional means absent or a directory, never a second
	// kind of file at the canonical path.
	for _, path := range []string{"app/Repositories", "app/Console/Commands"} {
		info, err := os.Stat(filepath.Join(root, path))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Errorf("reading optional directory %s: %v", path, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("optional path %s is not a directory", path)
		}
	}

	refusedPaths := []string{
		"app/Actions",
		"app/Data",
		"app/Rules",
		"app/Support",
		"bootstrap/providers.go",
		"bootstrap/cache",
		"routes/api.go",
		"routes/channels.go",
		"resources/js",
		"public/assets/manifest.json",
		"storage/logs",
	}
	for _, path := range refusedPaths {
		if _, err := os.Lstat(filepath.Join(root, path)); err == nil {
			t.Errorf("refused path %s exists in the application", path)
		} else if !os.IsNotExist(err) {
			t.Errorf("checking refused path %s: %v", path, err)
		}
	}

	nodeManifests := map[string]bool{
		"package.json":        true,
		"package-lock.json":   true,
		"npm-shrinkwrap.json": true,
		"pnpm-lock.yaml":      true,
		"yarn.lock":           true,
		"bun.lock":            true,
		"bun.lockb":           true,
	}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		switch {
		case d.IsDir() && d.Name() == "node_modules":
			t.Errorf("refused Node directory %s exists in the application", rel)
			return filepath.SkipDir
		case !d.IsDir() && nodeManifests[d.Name()]:
			t.Errorf("refused Node manifest %s exists in the application", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestEachSuiteHoldsWhatItsNameSays.
//
// The two suites are not two folders to spread files across. Each means
// something:
//
//	Feature  boots the application, or drives more than one piece of it
//	Unit     checks one thing, with nothing running
//
// The distinction earns its keep when a suite is slow: `go test ./tests/Unit/`
// is the one somebody runs on every save, and it stops being that the first time
// a test in it opens a database.
//
// It is checked by what a file reaches for, not by what it is called. A Unit
// test that boots the kernel is in the wrong suite whatever its name is.
func TestEachSuiteHoldsWhatItsNameSays(t *testing.T) {
	// What only a Feature test may reach for.
	//
	// Booting and serving, and also opening a database: the relay tests make no
	// HTTP request and are Feature all the same, because they drive the outbox,
	// the store and the publisher together against a real schema. "Feature"
	// means more than one piece interacting, and HTTP is the common case rather
	// than the definition.
	//
	// httptest.NewServer belongs on this list next to NewRequest, and was
	// missing from it: a test that listens on a port and dials it back is not
	// one somebody runs on every save, whatever it is checking.
	//
	// tests.Boot is the implementation App and AppWithMailbox are two views of,
	// and it is named here as well as they are. A booter left off this list is a
	// hole rather than an omission: it is the one way into tests/Unit for a test
	// that migrates a database, and the suite somebody runs on every save is
	// exactly the one that cannot afford it.
	//
	// openForTest is on it for the same reason migratedDB is: both are helpers
	// of the Feature suite that open a connection, and the check reads the text
	// of a file rather than what it calls, so a helper that is not named here
	// hides every test that reaches the database through it.
	boots := regexp.MustCompile(`tests\.App\(|tests\.Boot\(|tests\.Kernel\(|bootstrap\.Dispatch\(|bootstrap\.Open\(|httptest\.NewRequest\(|httptest\.NewServer\(|migratedDB\(|openForTest\(`)

	for _, suite := range []string{"Feature", "Unit"} {
		dir := filepath.Join(tests.Root(t), "tests", suite)
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("reading tests/%s: %v", suite, err)
		}

		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), "_test.go") {
				continue
			}
			body, err := os.ReadFile(filepath.Join(dir, e.Name()))
			if err != nil {
				t.Fatal(err)
			}

			switch found := boots.Match(body); {
			case suite == "Unit" && found:
				t.Errorf("tests/Unit/%s boots the application: move it to tests/Feature, or the suite "+
					"nobody waits for becomes one they do", e.Name())
			case suite == "Feature" && !found:
				t.Errorf("tests/Feature/%s boots nothing: move it to tests/Unit, or Feature stops "+
					"meaning what it says", e.Name())
			}
		}
	}
}

// goFiles is every .go file in the project, as a path relative to the root.
//
// Generated views and compiled templates are in here too, on purpose: they are
// files the compiler reads, and a rule about Go files that skipped the ones
// nobody typed by hand would miss the ones a generator can get wrong at scale.
func goFiles(t *testing.T, root string) []string {
	t.Helper()

	var found []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		found = append(found, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatalf("walking the project: %v", err)
	}
	if len(found) == 0 {
		t.Fatal("no .go file was found: the walk is looking in the wrong place, and every check built on it would pass on nothing")
	}
	return found
}

// TestNoTestIsInvisibleToGoTest is the one check here whose absence is silent.
//
// go test compiles a file into its package like any other unless the name ends
// in _test.go, and runs the test functions only of the ones that do. So a
// BrokerTest.go full of func TestX(t *testing.T) builds, links, reports no
// error, and executes nothing. Every other kind of broken test is loud; this
// one leaves a green build over a suite that never ran, and the count of tests
// executed is the only place it shows.
//
// The check is what the file holds, not what it is called, because the name is
// the part that varies: TestBroker.go, BrokerTests.go and broker.go all fail
// the same way once a test function is inside one.
func TestNoTestIsInvisibleToGoTest(t *testing.T) {
	// Go's own rule for a function it will run: the Test, Benchmark or Fuzz
	// prefix, a capital or an underscore after it, and one parameter of the
	// matching testing type. tests/testcase.go takes a *testing.T in every
	// helper it exports and matches none of this, which is the distinction.
	runnable := regexp.MustCompile(`(?m)^func (Test|Benchmark|Fuzz)[A-Z_][A-Za-z0-9_]*\([A-Za-z_][A-Za-z0-9_]* \*testing\.[TBF]\)`)

	root := tests.Root(t)
	for _, rel := range goFiles(t, root) {
		if strings.HasSuffix(rel, "_test.go") {
			continue
		}
		body, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatal(err)
		}
		if m := runnable.FindString(string(body)); m != "" {
			t.Errorf("%s holds a test function and does not end in _test.go, so go test compiles it "+
				"and runs nothing in it:\n  %s\nRename the file, or the suite is quietly shorter than it looks", rel, m)
		}
	}
}

// TestEveryPackageClauseIsLowercase.
//
// The directories under tests/ are capitalised and the package clauses inside
// them are not, and that is not an inconsistency to tidy up: a directory name
// is a label, an identifier is code. Every import of a capitalised package
// reads as an exported name that is not one, and go vet has nothing to say
// about it.
func TestEveryPackageClauseIsLowercase(t *testing.T) {
	clause := regexp.MustCompile(`(?m)^package ([A-Za-z_][A-Za-z0-9_]*)`)

	root := tests.Root(t)
	for _, rel := range goFiles(t, root) {
		body, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatal(err)
		}
		m := clause.FindStringSubmatch(string(body))
		if m == nil {
			continue
		}
		if strings.ToLower(m[1][:1]) != m[1][:1] {
			t.Errorf("%s declares `package %s`: a package identifier is lowercase, whatever "+
				"the directory holding it is called", rel, m[1])
		}
	}
}

// TestTheTestBaseNeverShipsInTheBinary.
//
// tests/ imports bootstrap, opens databases and calls t.Fatal. It is built to be
// linked into a test binary and nothing else. One import of it from a file the
// compiler puts in the application -- a handler reaching for a helper that
// already does the thing -- and the testing package, the fixtures and the
// throwaway database wiring are all inside what gets deployed.
//
// A direct import is the whole check, and it is enough: nothing outside this
// module can import a package of it back into this binary.
//
// The base package and anything under it, not the base package alone. A
// subdirectory of tests/ holding ordinary Go rather than _test.go files is the
// one part of the suite a production file can import with the compiler raising
// no objection -- the base itself is imported by the suites that bootstrap
// imports, so reaching for that one from the application is an import cycle and
// stops at the compiler before it ever reaches here.
func TestTheTestBaseNeverShipsInTheBinary(t *testing.T) {
	root := tests.Root(t)
	scaffolding := regexp.MustCompile(`"github\.com/arandu-io/examples/tests(/[^"]*)?"`)

	for _, rel := range goFiles(t, root) {
		if strings.HasSuffix(rel, "_test.go") {
			continue
		}
		// Production is what is left after the suite, and the suite is more than
		// its _test.go files: Helpers and the base are ordinary Go, and one
		// helper importing another is the tree working rather than leaking.
		// Asked the other way this check reports the suite for being itself.
		inModule, err := relativeToModule(filepath.Join(root, rel), root)
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasPrefix(inModule, "tests/") {
			continue
		}

		body, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatal(err)
		}
		if m := scaffolding.FindString(string(body)); m != "" {
			t.Errorf("%s imports %s, which is test scaffolding: it would be linked into the "+
				"application, testing and all", rel, strings.Trim(m, `"`))
		}
	}
}
