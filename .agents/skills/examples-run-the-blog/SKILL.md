---
name: examples-run-the-blog
description: Get this Arandu (Go) example application running, seeded and answering, and diagnose the ways a fresh clone fails. Use when the request is to "run the example", "start the blog", "aru dev", "how do I try this", "seed the database", "reset the data", "log in as the admin", "it will not start", "undefined renderHome", "no view named", "the form came back empty", "the migration will not run", "view:build fails", "the socket will not connect", "the debug console is 404", "docker compose up", or when a boot error, an APP_KEY, a DATABASE_URL or a port is involved. Covers the boot sequence in the order it has to happen, the seeded accounts, what answers on which port, and the three failures that are not your code.
license: MIT
---

# Running it

Five steps, and the order is not negotiable — the views are compiled before
anything Go-shaped will build. `aru` here is v0.62.0, the version this tree was
last measured with; `go run github.com/arandu-io/aru@v0.62.0 <command>` runs it
without installing anything.

```sh
export GOWORK=off
aru view:build                       # 34 views compiled, then the stylesheet
docker compose up -d postgres        # or skip it and use SQLite, below
cp .env.example .env && aru key:generate
aru migrate && aru db:seed
aru dev
```

Then `http://localhost:8080`.

## Without Postgres, which is faster and is what the tests do

SQLite is a file, so nothing has to be installed and nothing has to be running.
Every value below is what `sqliteEnv` at `tests/Feature/Commands_test.go:23`
sets, and the whole application runs on them:

```sh
export GOWORK=off
export APP_ENV=dev APP_KEY=0123456789abcdef0123456789abcdef
export DATABASE_URL="sqlite://$(mktemp -d)/blog.sqlite"
export ARANDU_TENANT_ID=11111111-1111-4111-8111-111111111111
go run . migrate && go run . db:seed && go run . serve
```

`APP_ENV` takes `dev`, `staging` or `prod` and nothing else. `production` is
refused at boot with `invalid APP_ENV: "production" (expected dev, staging or
prod)`, which is the kind of mistake worth making on a laptop rather than in a
deploy.

`go run .` and `aru` reach the same code: `bootstrap.Dispatch` is the entry
point either way, so a command is never a second, subtly different program.

## What the seed leaves you

`db:seed` in development writes, measured:

```
  using the development password "arandu-demo-password" -- set ARANDU_ADMIN_PASSWORD to change it
administrator admin@example.com ready in tenant 11111111-1111-4111-8111-111111111111
  using the development password "arandu-demo-password" -- set ARANDU_READER_PASSWORD to change it
reader reader@example.com ready, verified, no roles
4 section(s) written, 0 already there
6 post(s) written, 0 already there
4 comment(s) written, 0 already there
Database seeding completed successfully.
```

Both accounts get the development password `arandu-demo-password`, and the
seeder says so on the line above each. `ARANDU_ADMIN_EMAIL`,
`ARANDU_ADMIN_PASSWORD` and `ARANDU_READER_PASSWORD` override. It is repeatable:
run it twice and the second run reports `0 section(s) written, 4 already
there` and the same for the posts and comments, rather than duplicating
anything.

`admin@example.com` carries the role the moderation area asks for.
`reader@example.com` carries none, which is what makes it useful — it is how you
see a refusal rather than reading about one.

## What answers, and where

Measured against the booted application with `framework v0.51.0`, asking for
HTML. The status depends on `APP_ENV`, and that is the point of two of these:

| path | prod | dev |
| --- | --- | --- |
| `/` | 200 | 200 |
| `/auth/login` | 200 | 200 |
| `/sitemap.xml` | 200 | 200 |
| `/_arandu/health` | 200 | 200 |
| `/_arandu/debug` | 404 | 200 |
| `/admin/` signed out | 303 to sign-in | 303 |
| `/app/{appKey}` with no session | 401 | 401 |

The console being 404 in production is the wiring, not a route guard:
`k.Recorder()` is nil outside development and `middleware.Observe` records
nothing. `TestDebugConsoleIsDevelopmentOnly` holds it.

The socket key is `examples-app-key`, a constant in `bootstrap/app.go` beside
`SocketAppID`. It is in every page that connects and secret from nobody. The
eight HTTP routes joaju also answers are deliberately not mounted — this process
holds the broker in memory, so publishing is a method call.

The full route table is `go run . routes`: 61 routes across four modules — 32
this application's, 23 the auth screens', 5 the framework's and 1 the view
layer's.

## The three failures that are not your code

**1. `undefined: renderHome`, or `no view named home`.** The views were not
built. `resources/views/*.kyse.go` is the source; `storage/framework/views/` is
build output and `.gitignore` keeps it out of the tree, so a fresh clone has no
views at all. Run `aru view:build`. The same omission makes
`bash tests/test-layout-guard.sh` fail saying it could not ask the toolchain a
question, which is correct and is not what the guard is for.

**2. `aru view:build` fails with `does not parse -- this is a bug in the
generator`.** Your `aru` is older than this tree. Run the version it was
measured with, `go run github.com/arandu-io/aru@v0.62.0 view:build`, rather
than working around the message — it is telling the truth about the generator
it has. A binary built by `go install` or `go run` reports its version as
`dev`, so name the version rather than asking `aru --version`.

**3. A CSRF failure on a form you posted by hand.** The hidden field is
`_token`, and the token is bound to the cookie the form page set. Read it out
of the form and send the cookie back:

```sh
curl -s -c jar -o form.html http://127.0.0.1:8080/auth/login
TOK=$(grep -oE 'name="_token" value="[^"]*"' form.html | sed -E 's/.*value="([^"]*)".*/\1/')
curl -s -b jar -c jar -o /dev/null -w '%{http_code}\n' -X POST \
  --data-urlencode "_token=$TOK" --data-urlencode 'email=admin@example.com' \
  --data-urlencode 'password=arandu-demo-password' http://127.0.0.1:8080/auth/login
```

`303` is the success. `419` is the missing or stale token, `401` is the wrong
credentials — the two are told apart on purpose. Measured against
`go run . serve` on a seeded SQLite database.

## What this deployment is not

One process, and every line of it says so. `SESSION_DRIVER=memory`,
`CACHE_STORE=memory`, an in-process rate limiter, an in-memory socket broker,
and an outbox relay and a scheduler whose `Locker` is nil. Behind two replicas
half the requests land on the one that never saw the login, every replica runs
every scheduled task and publishes every stored event, and the socket counts
describe one binary rather than the fleet.
`bootstrap/app.go` names the line to swap in each case; none of them is a bug to
report.

Nothing is mailed. `MAIL_URL=log://` is the default, so the verification code
and the password-reset code are written to the output of `aru dev`. That is
where you go to finish a registration.
