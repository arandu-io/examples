# Working in this repository

This is a complete Arandu application, kept as something to read. It is a blog:
posts filed into sections, a comment thread that needs a confirmed address, a
moderation area, a password reset, a sitemap and a WebSocket. Nobody imports it
and nothing depends on it, so the only thing it produces is understanding — and
that makes a wrong line here worse than a wrong line in a library, because a
library that misleads you fails a build and this one gets copied.

Two people read it. One is learning how something is done and will copy the
shape into their own application. The other is keeping it true while the
framework underneath it moves. `.agents/skills/` holds a procedure for each,
named by the situation.

## The gates

Nothing is finished until all of these exit zero. Measured on this tree.

```sh
export GOWORK=off
aru view:build
gofmt -l $(find . -name '*.go' -not -path '*/testdata/*' -not -name '*.kyse.go')
go build ./...
go vet ./...
go test -race -count=1 ./...
aru doctor
bash tests/test-layout-guard.sh
```

`aru view:build` is first and is not optional on a fresh clone. The 29 files
under `resources/views/*.kyse.go` compile to 29 files under
`storage/framework/views/`, and `.gitignore` keeps that output out of the tree —
so before it has run, `go build ./...` fails with `undefined: renderHome` rather
than with anything about views, and the layout guard fails saying it could not
ask the toolchain a question. That is why the CI step that builds the views sits
above every other step.

Both filters on `gofmt` are load-bearing and are the project's rather than this
repository's: `gofmt` is the only tool in the chain that ignores build tags, so
without `-not -name '*.kyse.go'` it reports a syntax error on every view.
`CONTRIBUTING.md` and `Taskfile.yml` state the command without the `testdata/`
filter; `.github/workflows/ci.yml` states it with. Copy the one above.

`aru doctor` exits zero here with two warnings, both
`sensitive-field-not-redacted`, at `app/Http/Controllers/Auth/page.go:24` and
`app/Http/Controllers/Auth/render.go:33`. A third finding is a regression.

The suite needs no database server. Every feature test sets `DATABASE_URL` to a
SQLite file in its own `t.TempDir()` — `sqliteEnv` at
`tests/Feature/Commands_test.go:23` — and the whole suite passes with a
`DATABASE_URL` pointing at a closed port.

## The tree

| path | what it holds |
| --- | --- |
| `app/Policies/` | six policies. The only place in the application that decides anything |
| `app/Repositories/` | three repositories. Every method that reads or writes a row takes a `security.Grant` before the id |
| `app/Services/` | three services. They take a `security.Subject`, ask the policy, and hand the Grant down |
| `app/Http/Controllers/` | seven controllers over a shared `Controller` base. They load a session, call a service, render a view |
| `app/Http/Controllers/Auth/` | not a controller: a `foundation.Module` with its own `Routes()`, which is how `arandu-io/ui` publishes the auth screens |
| `resources/views/` | 29 `.kyse.go` templates. Source; `storage/framework/views/` is the build output |
| `routes/web.go`, `routes/admin.go` | 51 registered routes across four modules, 32 of them this application's |
| `bootstrap/app.go` | the whole wiring, top to bottom, in one function |
| `database/migrations/`, `database/seeders/` | seven each |
| `tests/Feature/`, `tests/Unit/` | 32 files, 127 test functions — 69 feature and 58 unit |

Counted with:

```sh
find resources/views -name '*.kyse.go' | wc -l                      # 29
find . -name '*_test.go' -not -path './storage/*' | wc -l           # 32
grep -rhoE '^func Test[A-Za-z0-9_]*' --include='*_test.go' . | wc -l  # 127
ls app/Policies | wc -l                                             # 6
GOWORK=off go run . routes | grep -cE '^  (GET|POST|PUT|PATCH|DELETE)'  # 51
```

`bin/` and `.env` are untracked and are on the machine, not in the repository.

## What does not exist here

Reaching for one of these is the fastest way to write a line this application
exists to argue against. None of them is missing by accident.

| A model reaches for | What is here instead |
| --- | --- |
| a service container, dependency injection | `bootstrap/app.go`. Reading it tells you what every route was given |
| middleware that authorizes a record | a Policy, called in the handler. `adminOnly` in `routes/admin.go` sets two headers and decides nothing |
| a system grant to make a public page work | `security.Guest(tenant)` through the same policy a browser goes through — `SitemapController.Index` |
| a repository call without a Grant, "just for the read path" | nothing. `tests/Unit/testdata/missing_grant/main.go` is the fixture that must not compile |
| a second query filtered "for guests" | a named action with a query of its own — `PostPublicList` beside `PostList` in `app/Policies/PostPolicy.go` |
| a tenant read off a path, a body, a query or a header | `data.Tenant(g)`. `withSubject` in `routes/web.go` takes the subject off the session cookie and nothing else |
| an ORM, a query builder on the model, `$fillable` | an entity struct and parameterised SQL in the repository |
| a template engine with runtime lookup | `.kyse.go`, compiled to Go. A missing field is a build error |
| npm, a bundler, `node_modules`, a CDN script | nothing. `TestResourcesHoldNoJavaScript` and `TestTheOnlyScriptsServedAreTheEmbeddedOnes` walk the tree and the response |
| a fixture that writes rows behind the policy without saying so | `security.SystemGrant` with a `//arandu:system-grant <reason>` line directly above it. Eleven of them in this repository, and every one carries its reason |

## The two rules everything else follows from

**Authorization is a value.** `security.Grant` has only unexported fields, so a
handler that reaches the database without asking a Policy has nothing to pass and
does not compile. `tests/Unit/GrantRequired_test.go` proves it twice: once by
compiling a fixture that must fail, and once by handing every method of
`PostRepository` a Grant issued for the wrong action and requiring each to
refuse. The second is the sharper of the two — carrying *a* Grant is not enough.

**The tenant comes from the Grant.** `data.Tenant(g)`, never from the request.
`tests/Feature/TenantScoping_test.go` is 513 lines of one tenant failing to see
another's rows, and it is the largest test file here for that reason.

The one screen that crosses tenants says so out loud: `SocketsController` reads
process-wide gauges, and its crossing has a name — `SocketInspectAll` on
`AllTenantSockets`, in `app/Policies/SocketMetricsPolicy.go`, reachable from no
other screen.

## Changing something here changes a demonstration

Every file in this application is an answer to "how is that done". So a change
that makes the code work and leaves the reason unwritten has broken the thing
this repository produces, even with all the gates green.

Three consequences:

- A comment explaining *why a shape is what it is* is the payload, not decoration.
  When you change the shape, change the paragraph above it in the same commit.
- A demonstration that stops being demonstrated is a regression. If a test named
  after a claim is deleted or weakened, the claim leaves with it.
- A number written in prose — in `README.md`, in a comment, in this file — is a
  measurement. Re-run the command before trusting it, and fix every copy
  together. `tests/test-layout-guard.sh` states two of them in its own comments
  ("all 32 test files", "the 29 files under resources/views") and both are
  currently right.

## Writing code

Comments, identifiers, error messages, log lines, CLI output and test names are
in English. A test name is a sentence about what the application does:
`TestAGuestIsRefusedTheDraftBehindAKnownAddress`,
`TestTheConsoleSeesTheQueriesOfTheRequest`.

A doc comment documents its symbol and nothing beyond it. Two here have drifted
off theirs and are worth not copying: `bootstrap/console.go:30` opens
"tenantID is…" above `func Tenant()`, and `bootstrap/console.go:149` says the
connection is made by "whatever `DB_CONNECTION` says" — that variable appears
nowhere else in the repository, and the configuration reads `DATABASE_URL`.

Every test lives under `tests/Feature` or `tests/Unit`, in an external `_test`
package, with a capitalised directory and a lowercase package clause. The
exception — a test needing an unexported identifier — sits beside its code as
`*_internal_test.go`, and nothing here takes it today.
`bash tests/test-layout-guard.sh` checks all four rules; `CONTRIBUTING.md` has
the reasoning and the sign-off requirement.
