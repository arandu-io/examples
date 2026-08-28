// Package bootstrap composes the application.
//
// It is the single place where everything is wired. The wiring is explicit and
// visible -- no dependency appears by magic. If you want to know where the
// user repository comes from, it is written here.
//
// `aru make:module` does NOT edit this file. It writes the code and prints the
// three lines to paste, because a generator that edited it behind your back
// would be a generator whose output nobody can account for -- and this file
// saying what the application is, exactly, is the point.
//
// Everything below is ordinary Go: read it top to bottom and you know the
// whole application.
package bootstrap

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/events"
	fhttp "github.com/arandu-io/framework/http"
	"github.com/arandu-io/framework/http/middleware"
	"github.com/arandu-io/framework/jobs"
	"github.com/arandu-io/framework/kernel"
	"github.com/arandu-io/framework/mail"
	"github.com/arandu-io/framework/modules/auth"
	"github.com/arandu-io/framework/observability"
	"github.com/arandu-io/framework/observability/errorpage"
	"github.com/arandu-io/framework/scheduler"
	"github.com/arandu-io/framework/security"
	"github.com/arandu-io/framework/view"
	"github.com/arandu-io/hesape/cache"
	"github.com/arandu-io/hesape/queue"
	hmiddleware "github.com/arandu-io/hesape/routing/middleware"
	"github.com/arandu-io/joaju"
	"github.com/arandu-io/joaju/protocols/pusher"

	controllers "github.com/arandu-io/examples/app/Http/Controllers"
	authui "github.com/arandu-io/examples/app/Http/Controllers/Auth"
	listeners "github.com/arandu-io/examples/app/Listeners"
	policies "github.com/arandu-io/examples/app/Policies"
	providers "github.com/arandu-io/examples/app/Providers"
	repositories "github.com/arandu-io/examples/app/Repositories"
	services "github.com/arandu-io/examples/app/Services"
	appconfig "github.com/arandu-io/examples/config"
	"github.com/arandu-io/examples/routes"

	// This application's own schema changes. Importing them is what registers
	// them: each one calls migrations.Register from init(), and a package
	// nothing imports is not in the binary at all -- so without this line `aru
	// migrate` finds nothing and says so only by creating no tables.
	_ "github.com/arandu-io/examples/database/migrations"

	// Importing the views is what registers them: every generated view calls
	// view.Register from init(), the same shape a database/sql driver has. Drop
	// this import and ctx.View("home") answers "no view named home".
	// The compiled stylesheet, embedded. Without this import the browser gets
	// the framework's default and every class written in a view of this project
	// is silently absent from the page.
	_ "github.com/arandu-io/examples/assets"

	// The engines this binary can speak, in bootstrap rather than in main
	// because bootstrap is what composes the application -- and the tests
	// compose it too: with them in main every feature test opened a connection
	// to a driver nobody had registered.
	_ "github.com/arandu-io/hesape/database/connectors/pgx"
	_ "github.com/arandu-io/hesape/database/connectors/sqlite"

	_ "github.com/arandu-io/examples/storage/framework/views"
	_ "github.com/arandu-io/examples/storage/framework/views/admin"
	_ "github.com/arandu-io/examples/storage/framework/views/auth"
	_ "github.com/arandu-io/examples/storage/framework/views/auth/passwords"
	_ "github.com/arandu-io/examples/storage/framework/views/comments"
	_ "github.com/arandu-io/examples/storage/framework/views/layouts"
	_ "github.com/arandu-io/examples/storage/framework/views/mail"
	_ "github.com/arandu-io/examples/storage/framework/views/posts"
)

// AppModule is this project's module path. The error page uses it to tell your
// frames from the framework's, and shows yours expanded.
const AppModule = "github.com/arandu-io/examples"

// App is everything the wiring produced.
//
// A struct rather than four return values, because the fifth one is always the
// one that breaks every call site.
type App struct {
	// Kernel is the composed application: configuration, modules, the global
	// middleware pipeline and the router.
	Kernel *kernel.Kernel
	// Auth is returned as well as registered, because the seeders need it and
	// reaching into the module to fetch it later would be exactly the kind of
	// hidden coupling the explicit wiring exists to avoid.
	Auth *auth.Service
	// Scheduler runs what the modules declared. `aru schedule:list` reads it.
	Scheduler *scheduler.Module
	// Relay is what empties the outbox. It is returned as well as registered for
	// the reason Auth is, sharpened by what it guards: a test that built a relay
	// of its own would pass over an application that wires none, and an
	// application that wires none writes rows nothing ever reads.
	Relay *events.Relay
	// Queue is the job store `aru work` drains.
	Queue *queue.DatabaseQueue
	// Mail is what sends. It is returned as well as used, because a job that
	// sends is built outside this function and reaching back in for the mailer
	// later is the hidden coupling the explicit wiring exists to avoid.
	Mail *mail.Mailer
}

// Build wires the application and returns it ready to boot.
//
// It does not boot, listen or migrate. main.go decides which of those the
// requested command needs, which is what keeps `aru routes` from opening a
// socket and `aru work` from starting a scheduler.
//
// It returns an error because some configurations describe a deployment this
// binary cannot be: a cache store nothing here defines is one of them. Refusing
// at the boot, naming the setting, is the alternative to starting and behaving
// as though something else had been asked for.
func Build(cfg appconfig.Config, db *data.DB) (App, error) {
	fw := cfg.Framework

	csrf := security.NewCSRF(fw.App.Key, cfg.Session.CSRFTTL)

	// Every cache store this application has, by name. CACHE_STORE names the
	// one the rate limit below counts in, and a name nothing defines is refused
	// rather than quietly replaced.
	stores := newCacheStores(cfg.Cache)

	// The core ships the in-memory session backend, which is right for one
	// instance and wrong for two: behind a load balancer, half the requests land
	// on the replica that never saw the login. SESSION_DRIVER is what says which
	// one is expected.
	sessions := security.NewSessionStore(fw.App.Key, cfg.Session.TTL, cfg.Session.Secure, security.NewMemoryBackend())

	// The rate limit counts in a store rather than in this process, which is the
	// difference between one budget and one budget per replica -- on the
	// endpoints a limit is put there for.
	//
	// It counts in the store CACHE_STORE named, and the in-process one is the
	// honest answer for a single instance: what it must not be is a shared store
	// that everybody assumed was there. Which store it is says which.
	limitStore, err := stores.Default()
	if err != nil {
		return App{}, err
	}
	limiter := cache.NewRateLimiter(limitStore.GetStore())

	// The queue over the application's own database, which is what makes a job
	// commitable by the same transaction as the row it is about. For volume
	// beyond a table, github.com/arandu-io/hesape/queue/connectors/redis is the
	// same contract over RESP -- same Worker, same handlers, one line here.
	queueStore := queue.NewDatabaseQueue(db)

	// The relay that empties the outbox, and the listener it hands events to.
	//
	// events.NewModule() brings the table and publishes nothing, which is a
	// coherent state -- storing is what cannot be recovered later, publishing can
	// start the day there is somewhere to publish to. It stops being coherent the
	// moment something stores: the auth module writes a row for every
	// registration, every confirmed address and every reset password, and without
	// this line they accumulate in a table no process reads.
	//
	// # It runs in `aru serve`, and in no other command
	//
	// That is not decided here. The module's loop is a kernel.Background one, and
	// Start is called by Kernel.Run and never by Kernel.Boot -- so `aru work`,
	// `aru routes` and every migration command build this same application and
	// start no relay.
	//
	// It is also the right place rather than the convenient one. `aru work`
	// scales with the depth of the job queue, so a relay there is one publisher
	// per worker replica and the count is whatever the queue happened to need. A
	// relay in a command of its own would be a second deployable to build,
	// monitor, page on and forget to restart, for one loop that already has a
	// process to live in. The scheduler is here for that same reason and is the
	// precedent.
	//
	// # Locker is nil, and that is a claim about this deployment
	//
	// One pass reads every unpublished row and marks what it delivered, so N
	// replicas of the server are N publishers of the same row unless something
	// stops them. RelayOptions.Locker is that something, and it is the same
	// kernel.Locker the scheduler below takes -- one value wires into both:
	//
	//	kernel.NewLocker(cache.NewLocks(redis.NewRedisStore(conn)))
	//
	// Nil says one replica, which is what the in-memory session backend and the
	// in-memory limiter above already say about this deployment. What nil costs
	// behind two is a duplicate delivery and never a lost event, and a publisher
	// has to tolerate the repeat regardless: delivery is at-least-once by design,
	// so a mark that fails after a successful publish sends the event again.
	relay := events.NewRelay(events.NewOutbox(db), listeners.NewEventLog(), events.RelayOptions{})

	// A module that calls another service takes observability.Client, not one of
	// its own:
	//
	//	billing.New(svc, observability.Client(10*time.Second))
	//
	// Going through it is what puts the call on the request timeline and on the
	// console. A handler that builds its own http.Client is a handler whose
	// 800ms wait shows up as "other", and the timeout is not optional --
	// http.Client has none by default, and a call with no deadline is how one
	// slow dependency turns into every request of the process hanging.

	// The mailer, and which transport is configuration rather than a decision
	// the calling code makes. Development is the log transport, so `aru dev`
	// works with nothing installed -- an application that needs a mail server to
	// start is one nobody runs.
	mailer := mail.New(mailTransport(cfg.Mail), view.NewRenderer(), mail.Address{
		Email: cfg.Mail.FromAddress,
		Name:  cfg.Mail.FromName,
	})

	authService := auth.NewService(auth.NewUserRepo(db), sessions, csrf)

	// The controllers, built here and handed to the routes. A controller that
	// constructed its own collaborators would be a controller no test can pin.
	// The services, built once and shared. The post screen reads the comment
	// thread, so both controllers hold the same CommentService rather than one
	// each -- two of them would be two policies to keep in step.
	postService := services.NewPostService(repositories.NewPostRepository(db))
	commentService := services.NewCommentService(repositories.NewCommentRepository(db))
	// The sections take the handle rather than a repository: their statements
	// are CRUD over one table, so they are written through the model, and
	// *data.DB is a model connection with nothing in between.
	categoryService := services.NewCategoryService(db)

	// The Application is built here rather than below the controllers because
	// two of them read its gauge registry, and it opens nothing: no connection,
	// no port, no migration. Boot and Run are what do that.
	k := kernel.New(fw)

	// The numbers this process owns, in one place. It is the whole process's and
	// not the socket server's: a second registry would be a second place to look
	// for "what is this binary doing right now", and the first screen that read
	// the wrong one would show a number that is true of nothing.
	//
	// It comes from the Application because the Application is what mounts the
	// console, and the console draws this. One built here instead would be
	// filled correctly and read by nobody.
	gauges := k.Gauges()
	socket := buildSocket(cfg.Auth.Tenant, gauges)

	deps := routes.Deps{
		Home:     controllers.NewHomeController(cfg.App.Name, sessions, csrf, authService, cfg.Auth.Tenant),
		Post:     controllers.NewPostController(postService, commentService, categoryService, authService, sessions, csrf, cfg.App.Name, cfg.App.URL, cfg.Auth.Tenant),
		Comment:  controllers.NewCommentController(commentService, sessions, csrf, cfg.App.Name, authService, cfg.Auth.Tenant),
		Category: controllers.NewCategoryController(categoryService, sessions, csrf, cfg.App.Name, authService, cfg.Auth.Tenant),
		Admin:    controllers.NewAdminController(postService, commentService, sessions, csrf),
		// The operator's screen, and the socket server it reads. The screen is
		// given the registry rather than the server's counter: it draws what was
		// published, and buildSocket is what publishes it. A screen holding the
		// counter would be a screen reading the process directly, and the read it
		// makes crosses tenants.
		Sockets: controllers.NewSocketsController(gauges, policies.SocketMetricsPolicy{Tenant: cfg.Auth.Tenant}, sessions, csrf),
		Socket:  socket,
		// The origin the sitemap builds absolute URLs on. A sitemap of relative
		// paths is refused by every crawler that reads one, and the value cannot
		// come from the request: a Host header is what the client sent.
		Sitemap: controllers.NewSitemapController(postService, cfg.Auth.Tenant, cfg.App.URL),
		// What the route guards read. The same store the pipeline and the
		// controllers were given, and it has to be: two stores over one key
		// would agree about the signature and disagree about which sessions
		// exist.
		Sessions: sessions,
	}

	k.
		// The pipeline order is the order of execution. Recover comes FIRST, or
		// a panic in any middleware below it escapes without a page.
		Use(
			middleware.Recover(cfg.App.IsDev(), errorpage.Options{
				Editor:    fw.Observability.Editor,
				AppModule: AppModule,
				// What the registered modules know about the state of the
				// system right now -- the outbox falling behind, and whatever
				// the next module reports. It shows up next to the failure
				// somebody is already looking at.
				Diagnose: k.Diagnose,
			}),
			// k.Recorder() is the buffer behind /_arandu/debug. It is nil
			// outside development, and passing nil records nothing -- which is
			// what production does.
			middleware.Observe(cfg.App.IsDev(), fw.Observability.TracingSecret, k.Recorder()),
			middleware.SecurityHeaders(cfg.App.IsDev()),
			// The budget and the window are one value, which is what a named
			// limiter resolves to. The refusal is passed rather than assumed:
			// how a 4xx is written belongs to the request layer, and this one
			// adds HX-Refresh, without which somebody over the limit presses
			// the button and the screen does not change.
			//
			// The key is unchanged, and it has to be: a counter in a shared
			// store is keyed by the string KeyBySession returns, so a different
			// one would hand every caller a fresh budget on deploy.
			hmiddleware.Throttle(limiter, cache.PerMinute(300),
				middleware.KeyBySession(sessions.IDFromRequest), fhttp.Refuse),
			middleware.CSRFProtect(csrf, sessions.IDFromRequest),
		).
		Register(
			// The view layer. It brings the renderer ctx.View needs, through the
			// optional kernel.RendererProvider interface, and serves the
			// embedded assets. Without it every page answers with an error that
			// names this missing line, and every stylesheet 404s.
			view.NewModule(),
			// Single tenant: every login belongs to one constant. A multi-tenant
			// application swaps this for a resolver that reads the host name --
			// same code path, same queries, one line different.
			// The starter kit's screens, in place of the framework's. The
			// framework ships the minimum markup that exists so authentication
			// could be tested at all; this one has a page. Register one or the
			// other, never both -- they answer the same path.
			authui.New(authService, sessions, csrf, mailer, fw.App.Key, cfg.App.Name, cfg.App.URL, auth.FixedTenant(cfg.Auth.Tenant)),
			// The outbox table, and the relay built above that empties it. A
			// module that records domain events stores them in the same
			// transaction as the write, and this is what brings the table those
			// rows land in -- see doc 27.
			events.WithRelay(relay),
			// The jobs table. Work that happens after the response, drained by
			// `aru work` -- the same image with another argument, which is what
			// keeps the deploy at one artifact.
			//
			// The driver goes in unwrapped: the module discovers the schema by
			// asking it, and anything standing in between would answer that
			// question for a driver it does not know -- a queue that looks wired
			// with no table behind it.
			jobs.NewModule(queueStore),
			// This application: its routes, from routes/web.go, and its own
			// migrations, from database/migrations.
			providers.NewAppServiceProvider(deps),
			// `aru make:module` adds the next modules here.
		)

	// The scheduler goes last, because it collects the tasks the modules above
	// declared. A module never starts its own goroutine; it declares work, and
	// this is what runs it.
	//
	// Locker is nil here: one replica. Behind more than one, build one over a
	// store every replica shares, or every replica runs every task:
	//
	//	kernel.NewLocker(cache.NewLocks(redis.NewRedisStore(conn)))
	//
	// Tenants is nil too: a PerTenant task needs to know which tenants exist,
	// and only the application knows where that list lives. Wire it and the
	// scheduler expands the task to each of them, with its own Grant.
	// Recorder for the same reason as the worker: a scheduled task is
	// investigated on the same page as a request, and costs nothing when
	// nothing is recording.
	sched := scheduler.NewModule(k.Tasks(), scheduler.Options{Recorder: k.Recorder()})
	k.Register(sched)

	return App{Kernel: k, Auth: authService, Scheduler: sched, Relay: relay, Queue: queueStore, Mail: mailer}, nil
}

// The names this application's cache stores are known by.
//
// They are the values CACHE_STORE takes, so the word in the environment and the
// word in the wiring are one word.
const (
	memoryStore = string(appconfig.CacheMemory)
	respStore   = string(appconfig.CacheRedis)
)

// errRESPStoreAbsent is why nothing here defines the store CACHE_STORE spells
// redis and SESSION_DRIVER spells kv.
//
// The driver that builds it, and the session handler written over the same
// connection, are in a module this one does not require, so no store defined
// here reaches a server another replica can also read. Naming that store is
// therefore a deployment asking for something this binary cannot be, and the
// answer belongs at the boot rather than at the first request that finds no
// entry where one was written.
//
// It is a value rather than a sentence repeated at each caller, so the two
// settings that can name the store are refused in the same words.
var errRESPStoreAbsent = errors.New(
	"the RESP store is built by the hesape/redis module, which this application does not require, " +
		"so nothing here can define it: REDIS_URL names an endpoint that no store in this binary opens")

// cacheStores is every cache store this application has, by name.
//
// It is what stands where a nullable connection would, and the difference is
// what a caller has to do in order to be wrong. A nullable connection is nil
// for the in-process store and every consumer branches on it by hand; a branch
// somebody forgets is a lock that locks nothing, because what a shared store
// buys is state every replica sees and a store inside the process has none to
// share.
//
// A name is not a branch, and the danger does not come back through it. The
// manager answers with a store or with an error naming the store, never with
// nil. The store named memory is always defined and it is the only one: see
// errRESPStoreAbsent for the one this application cannot build.
type cacheStores struct {
	manager  *cache.CacheManager
	settings appconfig.Cache
}

// newCacheStores defines the stores the configuration describes.
//
// It opens nothing and reaches no server. The in-process store has none to
// reach, and a store that is defined is not a store that has been resolved.
func newCacheStores(cfg appconfig.Cache) *cacheStores {
	return &cacheStores{
		manager: cache.NewCacheManager(cache.Config{
			Default: string(cfg.Store),
			Prefix:  cfg.Prefix,
			Stores: map[string]cache.StoreConfig{
				// In-process, which is what CACHE_STORE=memory says: a single
				// replica, caching inside itself.
				memoryStore: {Driver: "array"},
			},
		}),
		settings: cfg,
	}
}

// Store returns the named store, building it the first time it is asked for.
//
// A name the configuration does not define is an error naming it. That is the
// property the whole type rests on: nothing stands in for the store that was
// asked for.
func (c *cacheStores) Store(name string) (*cache.Repository, error) {
	store, err := c.manager.Store(name)
	if err == nil {
		return store, nil
	}
	if name == respStore {
		return nil, fmt.Errorf("the cache store %q is not one this application defines: %w", name, errRESPStoreAbsent)
	}
	return nil, err
}

// Default returns the store CACHE_STORE named.
func (c *cacheStores) Default() (*cache.Repository, error) {
	return c.Store(string(c.settings.Store))
}

// The two identifiers joaju's routes carry. They are not credentials.
//
// SocketAppKey is the {appKey} of GET /app/{appKey} -- the address a browser
// opens, so it is in every page that connects and secret from nobody. SocketAppID
// is the {appId} of the HTTP API, which this application does not mount (see
// routes/web.go). They are constants rather than configuration because a value
// nothing keeps secret and nothing varies is configuration for its own sake, and
// because joaju has no app secret at all: this application authenticates at its
// own front door, and the socket server reads the subject that door left behind.
const (
	SocketAppID  = "examples"
	SocketAppKey = "examples-app-key"
)

// buildSocket wires the realtime server and the observer that publishes what it
// is holding into gauges.
//
// The observer counts nothing unless the server was built with it -- a server
// without one publishes nothing for the life of the process, and the screen that
// draws those absent numbers looks exactly like the screen of a quiet
// deployment. So it is created here and handed to ServerConfig.Observer AND to
// the protocol: the two halves are announced from different places, sockets by
// the server and channels by the protocol.
//
// Nothing is handed back for the screen to read. It reads the registry, which is
// the reason the observer exists rather than a bare counter.
//
// The broker is the same value in two places for the same kind of reason: the
// protocol reaches channels through it, and the server's own routes do too.
//
// There is no Relay. A relay is what makes several instances agree on who is
// connected where, and this is one process -- with one, every number on the
// operator's screen would be an answer for the fleet instead of for this binary,
// and there is no fleet. It is the field to fill in the day there is one.
func buildSocket(tenant string, gauges *observability.Gauges) *joaju.Server {
	counts := listeners.NewSocketGauges(gauges)
	broker := pusher.NewMemoryBroker()

	// Both policies are this application's, in app/Policies, beside the policy
	// that decides who may read a comment. Who may open a socket and who may
	// hear a channel are business rules of the application that holds them, and
	// joaju refuses to start without both rather than defaulting to something
	// that admits everybody.
	subscribe := policies.SocketSubscribePolicy{Tenant: tenant}

	server, err := joaju.NewServer(joaju.ServerConfig{
		AppID:     SocketAppID,
		AppKey:    SocketAppKey,
		Broker:    broker,
		Connect:   policies.SocketConnectPolicy{Tenant: tenant},
		Subscribe: subscribe,
		Protocol:  pusher.NewPusher(broker, subscribe, pusher.PusherConfig{Observer: counts}),
		Observer:  counts,
	})
	if err != nil {
		// A config joaju refuses is a wiring mistake in the lines above, and it
		// is refused at boot for the reason mailTransport refuses an unknown
		// transport there: an application that starts without the piece it was
		// built with is one that finds out from a customer.
		panic("bootstrap: the socket server could not be built: " + err.Error())
	}

	return server
}

// mailTransport picks the transport the configuration asked for.
//
// A switch here rather than a registry: there are four, they are all in this
// file, and a name that matches nothing is refused at boot rather than at the
// first message. An application that starts and cannot send is one that finds
// out from a customer.
func mailTransport(cfg appconfig.Mail) mail.Transport {
	switch cfg.Mailer {
	case appconfig.MailerSMTP:
		return mail.SMTP{
			Host:     cfg.Host,
			Port:     strconv.Itoa(cfg.Port),
			Username: cfg.Username,
			Password: cfg.Password,
		}
	case appconfig.MailerArray:
		return &mail.Array{}
	case appconfig.MailerResend:
		return mail.Resend{Key: cfg.Key}
	case appconfig.MailerSendGrid:
		return mail.SendGrid{Key: cfg.Key}
	default:
		return mail.Log{}
	}
}
