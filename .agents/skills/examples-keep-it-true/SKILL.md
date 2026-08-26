---
name: examples-keep-it-true
description: Change this Arandu (Go) example application, or move it onto a newer framework, without quietly retiring something it demonstrates. Use when the request is to "upgrade the framework", "bump the dependency", "go get -u", "update the example", "the framework changed and this broke", "add a feature to the blog", "add a page", "regenerate the module", "fix the README numbers", "this comment is out of date", "a test started failing after the upgrade", or when a version in go.mod, a count written in prose, or a doc comment that describes behaviour is involved. Covers the upgrade procedure, the numbers that have to be re-measured together, the claims currently known to be stale, and the rule that a demonstration without a test named after it is gone.
license: MIT
---

# Keeping the example true

Nobody imports this repository, so it cannot break a build downstream. What it
can do is go on stating something that stopped being true, and that is the only
failure mode worth designing against here. A green suite is not evidence of it:
the claims that go stale first are the ones written in prose.

## Moving onto a newer framework

**1. See what moved.** Six direct requires, all `arandu-io`, and 47 modules in
the graph including this one:

```sh
export GOWORK=off
go list -m -f '{{if not .Indirect}}{{.Path}} {{.Version}}{{end}}' all | grep -v '^$'
```

Today that is `framework v0.37.0`, `hesape v0.14.0`, the `pgx` and `sqlite`
connectors at `v0.5.0`, `joaju v0.4.0` and `kyse v0.12.1`.

**2. Bump one at a time and run the gates between.** A single `go get -u` across
all six turns one legible failure into a bisect.

```sh
go get github.com/arandu-io/framework@vX.Y.Z && go mod tidy
aru view:build && go build ./... && go vet ./... && go test -race -count=1 ./...
aru doctor && bash tests/test-layout-guard.sh
```

**3. Upgrade `aru` too, and from source.** The generator and the view compiler
move with the framework, and a stale binary fails in a way that reads like a bug
in the templates — the Homebrew `aru 0.29.1` currently cannot compile
`resources/views/auth/login.kyse.go` while a build from the `arandu-io/aru`
working tree compiles all 29. `aru --version` is the first thing to check when
`view:build` complains about the generator.

**4. Read the doc comments that describe the framework's behaviour, not just the
ones over the code you touched.** This is the step that gets skipped. Several
comments here are the record of a framework defect and its workaround, and the
defect being fixed upstream is exactly the event that makes them wrong — with
nothing failing.

**5. Re-measure every number written in prose, all of them, together.** They are
in `README.md`, in `AGENTS.md`, and in the comments of
`tests/test-layout-guard.sh`, which states two of them about itself.

```sh
find resources/views -name '*.kyse.go' | wc -l                            # 29
find . -name '*_test.go' -not -path './storage/*' | wc -l                 # 32
grep -rhoE '^func Test[A-Za-z0-9_]*' --include='*_test.go' . | wc -l      # 127
find . -name '*.go' -not -path './storage/*' -not -name '*_test.go' -exec cat {} + | wc -l   # 13253
find . -name '*_test.go' -not -path './storage/*' -exec cat {} + | wc -l  # 5469
ls app/Policies | wc -l                                                   # 6
GOWORK=off go run . routes | grep -cE '^  (GET|POST|PUT|PATCH|DELETE)'    # 51
```

The README's figures were checked against these and are right: 13,253 lines of
production code, 5,500 of test (5,469, rounded), 32 test files, six policies,
and hesape's 47 components — `ls -d */ | wc -l` in that repository, which counts
components and not the 153 Go packages under them.

## What is currently stale

Found by measurement, not by reading. Fix them where you touch them; each one is
a place the repository says something it can no longer show.

- **`routes/web.go`, the `upgradable` doc comment.** It states that
  `APP_ENV=dev` adds a live-reload recorder with no `Unwrap`, so "the socket is
  a production and staging feature". Measured against `framework v0.37.0`: with
  live reload active — `arandu-reload.js` is in the page — a signed-in handshake
  to `/app/examples-app-key` returns `101` and
  `pusher:connection_established`. The paragraph outlived the defect.
- **`bootstrap/console.go:149`.** "open connects using whatever `DB_CONNECTION`
  says", above `func Open`. That variable appears nowhere else in the
  repository; the configuration reads `DATABASE_URL`, and `.env.example` says
  the `DB_*` names are refused rather than ignored. It is a leading paragraph of
  an exported symbol's doc comment, so it publishes.
- **`bootstrap/console.go:30`.** "tenantID is the tenant this deployment logs
  into", above `func Tenant()`.
- **The Alpine tag.** `resources/views/layouts/app.kyse.go:62` and
  `resources/views/admin/layout.kyse.go:100` link `alpine.min.js`; the framework
  registers no such asset, so `view.URL` emits
  `/_arandu/assets/missing/alpine.min.js` and every page 404s once. No view in
  the repository uses a single Alpine directive.
  `TestTheOnlyScriptsServedAreTheEmbeddedOnes` checks the `/_arandu/assets/`
  prefix, which this URL has, so it passes — its own comment says a tag pointing
  anywhere else "is a 404", and this is the 404 it lets through.
- **`.github/workflows/ci.yml`, the gofmt comment.** It says `testdata/` holds
  "a file that does not parse -- that one is the test". The only fixture here,
  `tests/Unit/testdata/missing_grant/main.go`, parses and is gofmt-clean; it
  fails at type-check, which is what `TestRepositoryWithoutGrantDoesNotCompile`
  asserts.
- **`CONTRIBUTING.md` and `Taskfile.yml`** give the gofmt command without
  `-not -path '*/testdata/*'`. CI gives it with. It happens to pass either way
  today, which is why it has survived.

## Adding to the application

The bar is not "does it work". It is: what does a reader learn from this that
they could not learn from the seven controllers already here?

**1. Generate the module.** Posts, comments and categories were written by
`aru make:module` end to end, and that is the claim the repository makes about
itself. Hand-writing the next one quietly retires it.

**2. Give the claim a test named after it.** Every demonstration here has one —
`TestAGuestIsRefusedTheDraftBehindAKnownAddress`,
`TestTheSocketCountsAreTheOperatorsAndNotAReaders`,
`TestTheConsoleSeesTheQueriesOfTheRequest`. A claim without a test is a
paragraph, and a paragraph is what goes stale. Name it as a sentence about what
the application does.

**3. Write the reason above the code, in terms of the code.** The comments are
the payload here. A doc comment documents its symbol and nothing beyond it: no
date, no decision-record number, no other repository's name — `pkg.go.dev`
publishes it, and its reader is a developer, not an archaeologist.

**4. Put the test in the right suite.** `tests/Feature` boots the application
and makes a request; `tests/Unit` checks one thing without booting anything.
External `_test` package, capitalised directory, lowercase package clause. The
`_internal_test.go` exception exists and nothing here takes it.
`bash tests/test-layout-guard.sh` checks all four rules.

**5. A fixture that writes behind the policy says why.** `security.SystemGrant`
carries a `//arandu:system-grant <reason>` line directly above it — eleven of
them in this repository, in the three seeders and in three test files, each with
its own sentence. A twelfth without a reason is the beginning of the habit this
application argues against.

## What may be a dependency here, and what may not

This is an application, not the core, so it is allowed drivers — the pgx and
sqlite connectors are both wired, and the README calls that deliberate, because
an example should show the shape of a real deployment. The core's rule
(standard library plus `golang.org/x/crypto`) does not bind this repository.

What does bind it is Node: there is none, in any form, and two tests walk the
tree to say so. `TestResourcesHoldNoJavaScript` and
`TestTheOnlyScriptsServedAreTheEmbeddedOnes` are the pair. CSS is Tailwind
through the standalone binary `aru view:build` downloads and pins in
`arandu.toml`.

Adding a third-party Go dependency is not forbidden and is worth arguing for
first, in an issue — `CONTRIBUTING.md` says so, and the argument is what stops
the example teaching a dependency along with the pattern.
