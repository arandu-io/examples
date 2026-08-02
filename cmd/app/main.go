// Command app is the example application.
//
// It is the same shape as the skeleton's main -- explicit composition, no
// container, no magic -- with two real modules and a guided tour on top. Read it
// top to bottom and you know the whole application, which is the property the
// wiring exists to preserve.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/arandu-io/framework/config"
	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/httpx/middleware"
	"github.com/arandu-io/framework/kernel"
	"github.com/arandu-io/framework/modules/auth"
	"github.com/arandu-io/framework/observability/errorpage"
	"github.com/arandu-io/framework/security"

	"github.com/arandu-io/examples/database/seeders"
	"github.com/arandu-io/examples/modules/customer"
	"github.com/arandu-io/examples/modules/demo"
	"github.com/arandu-io/examples/modules/invoice"

	// Drivers register themselves, and they live here rather than in the
	// framework: that is what keeps the core at two dependencies.
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

// appModule is this project's module path. The error page uses it to tell your
// frames from the framework's, and shows yours expanded.
const appModule = "github.com/arandu-io/examples"

// The two tenants this example runs with.
//
// They are constants because security.SystemGrant refuses an empty tenant: a
// system grant with no tenant reads across every customer of the system. The
// second one exists so the isolation demonstration has somewhere to fail.
const (
	defaultTenant = "00000000-0000-4000-8000-000000000001"
	otherTenant   = "00000000-0000-4000-8000-000000000002"
)

func tenantID() string {
	if id := os.Getenv("ARANDU_TENANT_ID"); id != "" {
		return id
	}
	return defaultTenant
}

func main() {
	command := "serve"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}
	if err := dispatch(command, os.Args[2:]); err != nil {
		log.Fatal(err)
	}
}

func dispatch(command string, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	db, closeDB, err := open(cfg)
	if err != nil {
		return err
	}
	defer closeDB()

	app := build(cfg, db)
	ctx := context.Background()

	switch command {
	case "serve":
		if err := app.kernel.Boot(ctx); err != nil {
			return err
		}
		return app.kernel.Run(ctx)

	case "migrate":
		return migrate(ctx, db, app.kernel.Migrations())
	case "migrate:rollback":
		return rollback(ctx, db, app.kernel.Migrations())
	case "migrate:status":
		return migrateStatus(ctx, db, app.kernel.Migrations())
	case "migrate:fresh":
		return fresh(ctx, db, app.kernel.Migrations())

	case "routes":
		if err := app.kernel.Boot(ctx); err != nil {
			return err
		}
		fmt.Print(kernel.FormatRoutes(app.kernel.Routes()))
		return nil

	case "db:seed":
		return seeders.Run(ctx, seeders.Deps{
			Auth:        app.auth,
			Customers:   app.customers,
			Invoices:    app.invoices,
			Tenant:      tenantID(),
			OtherTenant: otherTenant,
		}, args)

	default:
		return fmt.Errorf("unknown command: %s (expected serve, migrate, migrate:rollback, migrate:status, migrate:fresh, routes or db:seed)", command)
	}
}

// application is what build returns: the kernel plus the services the commands
// need. Returning them beats reaching into a module later to fetch one.
type application struct {
	kernel    *kernel.Kernel
	auth      *auth.Service
	customers *customer.Service
	invoices  *invoice.Service
}

func build(cfg config.Config, db *data.DB) application {
	csrf := security.NewCSRF(cfg.AppKey, cfg.CSRFTTL)
	sessions := security.NewSessionStore(cfg.AppKey, cfg.SessionTTL, !cfg.IsDev(), security.NewMemoryBackend())
	limiter := middleware.NewMemoryLimiter()

	// The subject of a request comes from the session, and only from there.
	subject := func(r *http.Request) (security.Subject, error) {
		return sessions.Load(r.Context(), r)
	}

	authService := auth.NewService(auth.NewUserRepo(db), sessions, csrf)
	customerService := customer.NewService(customer.NewRepo(db))
	invoiceService := invoice.NewService(invoice.NewRepo(db))

	k := kernel.New(cfg)

	k.
		// The pipeline order is the order of execution. Recover comes FIRST, or a
		// panic in any middleware below it escapes without a page.
		Use(
			middleware.Recover(cfg.IsDev(), errorpage.Options{
				Editor:    cfg.Editor,
				AppModule: appModule,
				// What the modules know about the state of the system, next to
				// the failure somebody is already looking at.
				Diagnose: k.Diagnose,
			}),
			// k.Recorder() is the buffer behind /_arandu/debug, and nil outside
			// development -- which is what makes the console free in production.
			middleware.Observe(cfg.IsDev(), cfg.TracingSecret, k.Recorder()),
			middleware.SecurityHeaders(cfg.IsDev()),
			middleware.RateLimit(limiter, 300, time.Minute, middleware.KeyBySession(sessions.IDFromRequest)),
			middleware.CSRFProtect(csrf, sessions.IDFromRequest),
		).
		Register(
			auth.New(authService, auth.FixedTenant(tenantID())),
			customer.New(customerService, subject),
			invoice.New(invoiceService, subject),
		)

	// The tour panics on purpose and prints what a policy refused, so it exists
	// only in development -- same rule as the debug page.
	if cfg.IsDev() {
		k.Register(demo.New(customerService, invoiceService, subject, otherTenant))
	}

	return application{kernel: k, auth: authService, customers: customerService, invoices: invoiceService}
}

// open connects using whatever DB_CONNECTION says.
func open(cfg config.Config) (*data.DB, func(), error) {
	if path := cfg.Database.SQLitePath(); path != "" {
		// SQLite creates the file but never the directory above it.
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, nil, fmt.Errorf("creating the database directory: %w", err)
		}
	}

	sqldb, err := sql.Open(cfg.Database.Connection.Driver(), cfg.Database.DSN())
	if err != nil {
		return nil, nil, fmt.Errorf("opening %s: %w", cfg.Database.Redacted(), err)
	}

	if cfg.Database.Connection == data.DialectSQLite {
		// One writer: SQLite serializes writes anyway, and a larger pool only
		// turns the wait into "database is locked".
		sqldb.SetMaxOpenConns(1)
	} else {
		sqldb.SetMaxOpenConns(25)
		sqldb.SetMaxIdleConns(5)
		sqldb.SetConnMaxLifetime(time.Hour)
	}

	return data.Wrap(sqldb, cfg.Database.Connection), func() { _ = sqldb.Close() }, nil
}
