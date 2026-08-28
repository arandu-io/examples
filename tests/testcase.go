// Package tests is the base every test in this project builds on.
//
// It is a package the suites import rather than a base class they extend. What
// belongs here is what more than one suite needs -- booting the application,
// opening a database, reading a file from the project root -- and nothing else.
// A helper used by one test belongs beside it.
//
// The two suites mean what their names say:
//
//	tests/Feature/  boots the application and makes a request
//	tests/Unit/     checks one thing without booting anything
//
// The file is testcase.go, lowercase, because it is a package and not a test.
// Only a file ending in _test.go is run by go test: a TestCase.go one letter
// away from that pattern teaches the reader a name that, applied to a file with
// test functions in it, compiles into the package and runs nothing -- no error,
// no warning, a green build over a suite that never executed.
package tests

import (
	"context"
	"database/sql"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/arandu-io/framework/arandutest"
	"github.com/arandu-io/framework/data"
	fwbootstrap "github.com/arandu-io/framework/foundation/bootstrap"
	"github.com/arandu-io/framework/kernel"
	"github.com/arandu-io/framework/mail"
	"github.com/arandu-io/hesape/config"
	"github.com/arandu-io/hesape/database"

	"github.com/arandu-io/examples/bootstrap"
	appconfig "github.com/arandu-io/examples/config"
)

// Kernel needs no database. database/sql connects lazily, so the wiring, the
// pipeline and every route can be exercised without a server running -- which is
// what makes this a useful smoke test to keep in a project skeleton.
// Kernel boots the application for a test.
//
// Exported because both suites use it, which is the whole reason this package
// exists.
func Kernel(t *testing.T, env config.Env) *kernel.Kernel {
	t.Helper()

	cfg := fwbootstrap.Configuration{
		App: config.App{
			Name:     "test",
			Env:      env,
			HTTPAddr: ":0",
			// Absolute, with a scheme and a host, because App.Validate refuses
			// anything else -- and because appconfig.From reads it parsed rather
			// than from the environment.
			URL:      &url.URL{Scheme: "http", Host: "localhost:8080"},
			Timezone: time.UTC,
			Locale:   "en",
			Key:      []byte("0123456789abcdef0123456789abcdef"),
		},
		Database: database.Config{
			Connection: data.DialectPostgres,
			Host:       "127.0.0.1",
			Port:       "1",
			Database:   "does-not-exist",
			Username:   "user",
			Password:   "pass",
		},
		Observability: fwbootstrap.Observability{
			LogLevel: slog.LevelError,
			Editor:   "vscode",
		},
	}
	// Both halves, because each validates its own and neither validates the
	// other: an invalid key and an unreachable connection fail in different
	// places, and a test that only checked one would boot on the other.
	if err := cfg.App.Validate(); err != nil {
		t.Fatalf("the test configuration is not valid: %v", err)
	}
	if err := cfg.Database.Validate(); err != nil {
		t.Fatalf("the test connection is not valid: %v", err)
	}

	sqldb, err := sql.Open(cfg.Database.Connection.Driver(), cfg.Database.DSN())
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { _ = sqldb.Close() })

	appCfg, err := appconfig.From(cfg)
	if err != nil {
		t.Fatalf("loading the application configuration: %v", err)
	}
	app, err := bootstrap.Build(appCfg, data.Wrap(sqldb, cfg.Database.Connection))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	k := app.Kernel
	if err := k.Boot(context.Background()); err != nil {
		t.Fatalf("Boot: %v", err)
	}
	return k
}

// Root is the project root, from inside a suite directory.
//
// The tests that read a file -- the Dockerfile, the workflow, arandu.toml -- run
// two directories down from it, and a relative path written from anywhere else
// silently reads nothing: os.ReadFile returns an error the test reports as "the
// file does not say X", which is true and misleading.
func Root(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("tests.Root is not the project root: %v", err)
	}
	return root
}

// File reads one file from the project root.
func File(t *testing.T, name string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(Root(t), name))
	if err != nil {
		t.Fatalf("reading %s: %v", name, err)
	}
	return string(body)
}

// App boots the whole application on a throwaway SQLite database, migrated, and
// returns a browser for it.
//
// It is a migrated database and a client for it in one call, and it is the
// difference between a feature test worth writing and one nobody writes: every
// alternative starts with twelve lines of environment, connection and migration
// that have nothing to do with what is being proved.
//
// SQLite in a temporary directory, so the tests need nothing installed and two
// of them cannot see each other's rows. The file goes with t.TempDir.
func App(t *testing.T) (*arandutest.Client, *data.DB) {
	t.Helper()
	booted := Boot(t)
	return booted.Client, booted.DB
}

// AppWithMailbox is App plus what the application sent.
//
// The mailer is the array transport, so a test can read the message rather than
// asserting that a log line happened. That is the difference between proving a
// verification link works and proving a function was called: the link this
// returns is the one a person would click, produced by the same code path
// production takes.
func AppWithMailbox(t *testing.T) (*arandutest.Client, *data.DB, *mail.Array) {
	t.Helper()
	booted := Boot(t)
	return booted.Client, booted.DB, booted.Mail
}

// Booted is everything one boot produced.
//
// A struct rather than a fourth return value, for the reason bootstrap.App is
// one: the fifth is always the one that breaks every call site. App and
// AppWithMailbox are the two views of this that nearly every test wants, and
// they stay because they are the two that read well at the top of a test.
type Booted struct {
	// App is the wiring itself, for a test whose subject is something
	// bootstrap.Build produced rather than something a request reaches. The
	// relay is the case that forced this open: it publishes what the outbox
	// holds and is reachable from no route, so a test that built one of its own
	// would pass over an application that wires none.
	App bootstrap.App
	// Client is a browser for the booted application.
	Client *arandutest.Client
	// DB is the migrated throwaway database, for a fixture or an assertion about
	// a row.
	DB *data.DB
	// Mail is what the application sent.
	Mail *mail.Array
}

// Boot boots the whole application on a throwaway SQLite database, migrated.
//
// It is the one implementation behind App and AppWithMailbox: two ways to boot
// an application in one suite is two sets of environment variables to keep in
// step, and the one nobody runs daily is the one that drifts.
func Boot(t *testing.T) Booted {
	t.Helper()

	// Before appconfig.Load: this is what mailTransport switches on.
	t.Setenv("MAIL_URL", "array://")
	t.Setenv("APP_ENV", "dev")
	t.Setenv("APP_KEY", "0123456789abcdef0123456789abcdef")
	t.Setenv("DATABASE_URL", "sqlite://"+filepath.Join(t.TempDir(), "test.sqlite"))
	t.Setenv("ARANDU_TENANT_ID", "11111111-1111-4111-8111-111111111111")

	// The schema, through the same command a deploy runs. A test that creates
	// tables another way is a test that keeps passing after a migration breaks.
	if err := bootstrap.Dispatch("migrate", nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	cfg, err := appconfig.Load()
	if err != nil {
		t.Fatalf("loading the configuration: %v", err)
	}

	db, closeDB, err := bootstrap.Open(cfg)
	if err != nil {
		t.Fatalf("opening the database: %v", err)
	}
	t.Cleanup(closeDB)

	app, err := bootstrap.Build(cfg, db)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if err := app.Kernel.Boot(context.Background()); err != nil {
		t.Fatalf("Boot: %v", err)
	}

	box, ok := app.Mail.Transport().(*mail.Array)
	if !ok {
		// MAIL_URL is set above, so this is a wiring change rather than a
		// configuration mistake -- and a test that silently got the log
		// transport would assert nothing about what was sent.
		t.Fatalf("the test mailer is %s and not the array transport: nothing can read what was sent",
			app.Mail.Transport().Name())
	}
	return Booted{
		App:    app,
		Client: arandutest.NewClient(t, app.Kernel.Handler()),
		DB:     db,
		Mail:   box,
	}
}
