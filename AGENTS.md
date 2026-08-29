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

`aru view:build` is first and is not optional on a fresh clone. The 34 files
under `resources/views/*.kyse.go` compile to 34 files under
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

`aru doctor` exits zero here with no findings. Any finding is a regression.

The suite needs no database server. Tests that exercise rows use temporary
SQLite through `tests.Boot` (`tests/testcase.go:193-200`) or `sqliteEnv`
(`tests/Feature/Commands_test.go:23-27`). Wiring-only tests use `tests.Kernel`
with PostgreSQL at the closed address `127.0.0.1:1`, without connecting.

## The tree

| path | what it holds |
| --- | --- |
| `app/Policies/` | seven files defining eight Policy structs. Application authorization decisions live here |
| `app/Repositories/` | three repositories. Twenty-four of twenty-six exported methods take a `security.Grant`; the two `Health` methods only ping the connection |
| `app/Services/` | five services plus `TenantResolver`. Row access normally follows Subject → Policy → Grant; pre-authentication user and factor flows use annotated system grants |
| `app/Http/Controllers/` | seven application controllers over a shared `Controller` base. HTML, XML and process-wide gauges deliberately have different collaborators |
| `app/Http/Controllers/Auth/` | not one of the seven application controllers: a `kernel.Module` with its own `Routes()`, published by `arandu-io/ui` |
| `resources/views/` | 34 `.kyse.go` templates. Source; `storage/framework/views/` is the build output |
| `routes/web.go`, `routes/admin.go` | 61 registered routes across four modules, 32 of them this application's |
| `bootstrap/app.go` | the whole wiring, top to bottom, in one function |
| `database/migrations/`, `database/seeders/` | ten migrations and seven seeders, plus one registry file in each directory |
| `tests/`, `app/Http/Controllers/Auth/redaction_internal_test.go` | 48 files, 179 test functions — 47 files and 178 functions in the mirrored tree, plus one colocated internal test |

Counted with:

```sh
find resources/views -name '*.kyse.go' | wc -l                      # 34
git ls-files '*_test.go' | wc -l                                    # 48
grep -rhoE '^func Test[A-Za-z0-9_]*' --include='*_test.go' . | wc -l  # 179
git ls-files 'app/Policies/*.go' | wc -l                             # 7
rg '^type .*Policy struct' app/Policies/*.go | wc -l                  # 8
GOWORK=off go run . routes | grep -cE '^  (GET|POST|PUT|PATCH|DELETE)'  # 61
```

`bin/` and `.env` are untracked and are on the machine, not in the repository.

## What does not exist here

Reaching for one of these is the fastest way to write a line this application
exists to argue against. None of them is missing by accident.

| A model reaches for | What is here instead |
| --- | --- |
| a service container, dependency injection | `bootstrap/app.go`. Reading it tells you what every route was given |
| middleware that authorizes a record | a Policy, normally called in the Service. `adminOnly` in `routes/admin.go` sets two headers and decides nothing |
| a system grant to make a public page work | `security.Guest(tenant)` through the same policy a browser goes through — `SitemapController.Index` |
| a repository row read or write without a Grant | nothing. `tests/Unit/testdata/missing_grant/main.go` is the fixture that must not compile; the two `Health` methods only ping |
| a second query filtered "for guests" | a named action with a query of its own — `PostPublicList` beside `PostList` in `app/Policies/PostPolicy.go` |
| a tenant read off a path, a body, a query or a header | `data.Tenant(g)`. `withSubject` in `routes/web.go` takes the subject off the session cookie and nothing else |
| a hand-written repository for routine CRUD | `Model[T]` and `Builder[T]`; parameterised SQL repositories remain for complex queries and operations |
| a template engine with runtime lookup | `.kyse.go`, compiled to Go. A missing field is a build error |
| npm, a bundler, `node_modules`, a CDN script | nothing. `TestResourcesHoldNoJavaScript` and `TestTheOnlyScriptsServedAreTheEmbeddedOnes` walk the tree and the response |
| production or seed data that writes behind a Policy without saying so | `security.SystemGrant` with a `//arandu:system-grant <reason>` line directly above it. Nineteen production and seeder calls carry that reason; tests use additional grants to arrange cases |

## The two rules everything else follows from

**Authorization is a value.** Normal row paths carry a `security.Grant` issued
by a Policy. `security.SystemGrant` is the explicit administrative and fixture
escape hatch, and `aru doctor` requires its reason and constrains its use.
`tests/Unit/GrantRequired_test.go` compiles a fixture that must fail without a
Grant, then hands `Find`, `List`, `Create` and `Update` a Grant issued for the
wrong action and requires each to refuse. Carrying *a* Grant is not enough.

**The tenant comes from the Grant.** `data.Tenant(g)`, never from the request.
`tests/Feature/TenantScoping_test.go` is 987 lines of one tenant failing to see
another's rows, and it is the largest test file here for that reason. Its
fixture writes two rows of the second tenant that name the first's — a post
filed under our section id, a comment hung off our article — because a key that
matches while the tenant does not is what a nested query, an eager load and an
aggregate are each wrong about in their own way. The fourth shape, a pivot, has
no table here to run against, and the file says so where the other three are
tested rather than leaving the absence to be inferred.

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
  ("47 test files under tests/ plus one colocated internal test", "the 34 files
  under resources/views") and both are currently right.

## Writing code

Comments, identifiers, error messages, log lines, CLI output and test names are
in English. A test name is a sentence about what the application does:
`TestAGuestIsRefusedTheDraftBehindAKnownAddress`,
`TestTheConsoleSeesTheQueriesOfTheRequest`.

A doc comment documents its symbol and nothing beyond it. One here has drifted
off its own and is worth not copying: `bootstrap/console.go:32` opens
"tenantID is…" above `func Tenant()`. There were two — the doc on `Open` said
the connection was made by "whatever `DB_CONNECTION` says", a variable that
appears nowhere else here — and that one went when `Open` was rewritten to hand
the adapter the whole parsed connection.

Every externally scoped test lives under a capitalised category in `tests/`, in
an external `_test` package with a lowercase package clause. The one exception
needs an unexported identifier and sits beside its code as
`app/Http/Controllers/Auth/redaction_internal_test.go`.
`bash tests/test-layout-guard.sh` checks all four rules; `CONTRIBUTING.md` has
the reasoning and the sign-off requirement.
