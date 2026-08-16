package unit_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// What the repository is allowed to contain, checked by reading the files rather
// than by trusting a name.
//
// Each check reads for a shape of mistake rather than for a name. A pattern
// written for one shape misses the next one, and the next one is what ships.

// sqliteMagic is the first sixteen bytes of every SQLite file, whatever it is
// called. https://sqlite.org/fileformat.html
var sqliteMagic = []byte("SQLite format 3\x00")

// TestNoDatabaseIsTracked.
//
// A bare name in DATABASE_URL -- arandu_blog, plus its -shm and -wal -- is a
// file with no extension, which SQLite writes relative to wherever the process
// started. So *.sqlite misses it, *.db misses it, and database/* misses it
// because it lands in the project root.
//
// This reads the first bytes of every tracked file instead. A database cannot
// hide behind a name nobody predicted, because it is not the name being read.
//
// The schema is database/migrations. A committed database disagrees with it by
// the second day, and the diff of a binary that changes on every run is a diff
// nobody can review.
func TestNoDatabaseIsTracked(t *testing.T) {
	for _, name := range tracked(t) {
		f, err := os.Open(name)
		if err != nil {
			// Tracked and absent is a different problem, and not this test's.
			continue
		}
		head := make([]byte, len(sqliteMagic))
		n, _ := f.Read(head)
		f.Close()

		if n == len(sqliteMagic) && bytes.Equal(head, sqliteMagic) {
			t.Errorf(`%s is a SQLite database and it is committed.

    git rm --cached %s

The path in DATABASE_URL is a PATH. A bare name puts the file wherever the
process started, which is the project root, where no ignore rule expects it --
the config refuses that value now, so this file predates the refusal.`, name, name)
		}
	}
}

// TestNoEnvFileIsTracked: .env carries the application key and every credential,
// and a repository is the one place it must never be.
func TestNoEnvFileIsTracked(t *testing.T) {
	for _, name := range tracked(t) {
		base := filepath.Base(name)
		if base == ".env" || (strings.HasPrefix(base, ".env.") && base != ".env.example") {
			t.Errorf("%s is committed, and it carries the application key", name)
		}
	}
}

// TestTheCompiledViewsAreNotTracked.
//
// storage/framework/views is build output: `aru view:build` writes it, every
// file in it opens with DO NOT EDIT, and a generated file in a repository is one
// people read diffs of, resolve conflicts in, and eventually edit.
//
// assets/app.css and assets/fonts are the deliberate exception and stay: `go
// build` has to work on a fresh clone, before any tool has run.
func TestTheCompiledViewsAreNotTracked(t *testing.T) {
	for _, name := range tracked(t) {
		if strings.HasPrefix(name, "storage/framework/views/") && !strings.HasSuffix(name, ".gitignore") {
			t.Errorf("%s is a compiled view and it is committed -- `aru view:build` writes it", name)
		}
	}
}

// tracked is what git has, which is the only list that matters: an ignored file
// on disk is nobody's problem, and a tracked one is in every clone.
func tracked(t *testing.T) []string {
	t.Helper()

	// ../.. and not ..: a Go test runs with the package directory as its working
	// directory, so from tests/Unit the project root is two levels up. One level
	// reached tests/, where git ls-files answers with paths relative to the root
	// anyway -- so every name was joined to the wrong prefix, every file failed
	// to open, and the loop skipped all of them. The test passed by reading
	// nothing, which is the worst way for a guard to pass.
	const root = "../.."

	out, err := exec.Command("git", "-C", root, "ls-files", "-z").Output()
	if err != nil {
		t.Skipf("git is not available, so what is tracked cannot be read: %v", err)
	}

	var names []string
	for _, name := range strings.Split(strings.TrimRight(string(out), "\x00"), "\x00") {
		if name != "" {
			names = append(names, filepath.Join(root, name))
		}
	}
	if len(names) == 0 {
		t.Skip("this is not a git checkout")
	}
	return names
}

// TestANullableColumnIsNotScannedIntoAPlainValue.
//
// Every column added by a later migration is nullable -- the rule the migrations
// state in their own doc comments, because during a rollout the previous binary
// is still inserting rows that know nothing about it. A scan that reads such a
// column straight into an int, a string or a bool answers
// "converting NULL to int is unsupported" on every read of the table, for every
// row written before the migration and every row the old binary writes during
// the rollout.
//
// This reads the ALTER statements out of the migrations and the scan out of the
// repository, so it fails when the next column is added without its pair.
func TestANullableColumnIsNotScannedIntoAPlainValue(t *testing.T) {
	const root = "../.."

	added := map[string]bool{}
	migrations, err := filepath.Glob(filepath.Join(root, "database", "migrations", "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range migrations {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range addColumn.FindAllStringSubmatch(string(body), -1) {
			// A column with a DEFAULT is never NULL, so a plain scan is correct.
			if !strings.Contains(strings.ToUpper(m[0]), "DEFAULT") {
				added[m[2]] = true
			}
		}
	}
	if len(added) == 0 {
		t.Skip("no nullable column is added by any migration")
	}

	repos, err := filepath.Glob(filepath.Join(root, "app", "Repositories", "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range repos {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		text := string(body)
		for column := range added {
			field := "&" + "p." + exportedName(column)
			if strings.Contains(text, field) {
				t.Errorf("%s scans the nullable column %q straight into a field (%s).\n"+
					"Read it through sql.Null... the way the column beside it is read, or give the column a DEFAULT.",
					filepath.Base(path), column, field)
			}
		}
	}
}

// exportedName turns a column name into the Go field the generator would emit.
func exportedName(column string) string {
	parts := strings.Split(column, "_")
	for i, p := range parts {
		if p != "" {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, "")
}

var addColumn = regexp.MustCompile(`(?i)ALTER TABLE (\w+) ADD COLUMN (\w+)[^;]*;`)
