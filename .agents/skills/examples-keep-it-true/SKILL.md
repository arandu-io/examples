---
name: examples-keep-it-true
description: Change this Arandu (Go) example application, or move it onto a newer framework, without quietly retiring something it demonstrates. Use when the request is to "upgrade the framework", "bump the dependency", "go get -u", "update the example", "the framework changed and this broke", "add a feature to the blog", "add a page", "regenerate the module", "fix the README numbers", "this comment is out of date", "a test started failing after the upgrade", "import-not-canonical", "which package is Grant imported from", "a rejected form", or when a version in go.mod, a count written in prose, or a doc comment that describes behaviour is involved. Covers the upgrade procedure, the numbers that have to be re-measured together, the claims currently known to be stale, and the rule that a demonstration without a test named after it is gone.
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

Today that is `framework v0.51.0`, `hesape v0.50.1`, the `pgx` and `sqlite`
connectors at `v0.11.0`, `joaju v0.7.1` and `kyse v0.30.0`.

**2. Bump one at a time and run the gates between.** A single `go get -u` across
all six turns one legible failure into a bisect.

```sh
go get github.com/arandu-io/framework@vX.Y.Z && go mod tidy
aru model:build --check && aru view:build && go build ./... && go vet ./... && go test -race -count=1 ./...
aru doctor && bash tests/test-layout-guard.sh
```

**3. Move `aru` with it, and say which one.** The generator, the view compiler
and the doctor move with the framework, and a stale binary fails in a way that
reads like a bug in this tree. This tree was last measured with `aru` v0.62.0,
and `go run github.com/arandu-io/aru@v0.62.0 doctor` runs that version without
touching the one installed. A build by `go install` or `go run` reports its
version as `dev`, so the version is the one you named, not what `aru --version`
prints. `.github/workflows/ci.yml` installs v0.58.0, which predates
`import-not-canonical` and passes on a tree that v0.62.0 warns about.

**4. Read what the doctor says about imports.** A framework release can move a
symbol from its bridge packages to hesape. `aru doctor` names each file that
still spells it the old way as `import-not-canonical`, and
`aru imports:catalog` prints the path every exported symbol should be named by
for the version `go.mod` requires. A symbol the framework only re-exports goes
to hesape (`auth.Grant`, `auth.Tenant`, `database.DB`, `hhttp.Context`,
`log.For`); what the framework declares or wraps keeps its path
(`fhttp.Router`, `security.SessionStore`, `data.Repository`). The catalog has no
mode that rewrites the files, so the move is a change you make, and the doc
comments and this directory spell the same names as the code afterwards.

**5. Read the doc comments that describe the framework's behaviour, not just the
ones over the code you touched.** This is the step that gets skipped. Several
comments here are the record of a framework defect and its workaround, and the
defect being fixed upstream is exactly the event that makes them wrong — with
nothing failing.

**6. Re-measure every number written in prose, all of them, together.** They are
in `README.md`, in `AGENTS.md`, in this directory, and in the comments of
`tests/test-layout-guard.sh`, which states two of them about itself.

```sh
find resources/views -name '*.kyse.go' | wc -l                            # 34
find . -name '*_test.go' -not -path './storage/*' | wc -l                 # 50
grep -rhoE '^func Test[A-Za-z0-9_]*' --include='*_test.go' . | wc -l      # 195
find . -name '*.go' -not -path './storage/*' -not -name '*_test.go' -exec cat {} + | wc -l   # 17592
find . -name '*_test.go' -not -path './storage/*' -exec cat {} + | wc -l  # 9525
ls app/Policies | wc -l                                                   # 7
grep -E '^type .*Policy struct' app/Policies/*.go | wc -l                 # 8
```

The route count needs a configuration to boot, and these are the values the
tests use:

```sh
APP_ENV=dev APP_KEY=0123456789abcdef0123456789abcdef \
DATABASE_URL="sqlite://$(mktemp -d)/routes.sqlite" \
ARANDU_TENANT_ID=11111111-1111-4111-8111-111111111111 \
GOWORK=off go run . routes | grep -cE '^  (GET|POST|PUT|PATCH|DELETE)'     # 61
```

The README's figures were checked against these: 17,592 lines of production
code, 9,525 of test, 50 test files and eight policies in seven files.

## What is currently stale

Found by measurement, not by reading. Fix them where you touch them; each one is
a place the repository says something it can no longer show.

- **`routes/web.go`, the `upgradable` doc comment.** Its last paragraph states
  that `APP_ENV=dev` adds a live-reload recorder with no `Unwrap`, so "the
  socket is a production and staging feature". Measured against
  `framework v0.51.0` with `APP_ENV=dev`: a signed-in handshake to
  `/app/examples-app-key` answers `101`. The paragraph outlived the defect.
- **`bootstrap/console.go:31`.** "tenantID is the tenant this deployment logs
  into", above `func Tenant()`. It is the leading line of an exported symbol's
  doc comment, so it publishes.
- **`.github/workflows/ci.yml`, the gofmt comment.** It says `testdata/` holds
  "a file that does not parse -- that one is the test". The only fixture here,
  `tests/Unit/testdata/missing_grant/main.go`, parses and is gofmt-clean; it
  fails at type-check, which is what `TestRepositoryWithoutGrantDoesNotCompile`
  asserts.
- **`.github/workflows/ci.yml`, the `aru` version.** It installs v0.58.0, behind
  the v0.62.0 this tree answers to.
- **`CONTRIBUTING.md` and `Taskfile.yml`** give the gofmt command without
  `-not -path '*/testdata/*'`. CI gives it with. It happens to pass either way
  today, which is why it has survived.

Two things look stale and are not:

- **The 422 entry in `resources/views/layouts/app.kyse.go`'s `htmx-config`.**
  It is for the sign-in screens the authentication kit published, which still
  answer a refused code or password with a 422 fragment of their own form. The
  blog's own forms never answer that way, and the layout says so. It leaves when
  the kit's screens answer through the router.
- **`CommentController`'s other six actions and the four `comments/` views.**
  `aru make:module` generated them and they compile, but `routes/web.go`
  registers only `Store`, under `posts.comments`, and says why: the thread hangs
  off the post, and a top-level `/comments` would be a second address for it.

## Adding to the application

The bar is not "does it work". It is: what does a reader learn from this that
they could not learn from the seven controllers already here?

**1. Generate the module.** Posts, comments and categories were written by
`aru make:module` end to end, and that is the claim the repository makes about
itself. Hand-writing the next one quietly retires it.

**2. Give the claim a test named after it.** Every demonstration here has one —
`TestAGuestIsRefusedTheDraftBehindAKnownAddress`,
`TestTheSocketCountsAreTheOperatorsAndNotAReaders`,
`TestARejectedPostGoesBackToTheFormWithTheMessage`. A claim without a test is a
paragraph, and a paragraph is what goes stale. Name it as a sentence about what
the application does.

**3. Let the router answer a rejected form.** The action returns the service's
`validation.Errors` as they are. The router sends a page back to the form with
the messages and what was typed in the flash, answers htmx with `HX-Redirect`,
and answers a client that asked for JSON with a 422 problem document. The page
draws them because `navigation.page` in `app/Http/Controllers/chrome.go` puts
the flash on every `view.Page`, and each kyse input is handed the page. A
controller that draws the form again with a 422 is a second way to answer the
same thing, and `tests/Feature/RejectedForms_test.go` is what it would break.

**4. Write the reason above the code, in terms of the code.** The comments are
the payload here. A doc comment documents its symbol and nothing beyond it: no
date, no decision-record number, no other repository's name — `pkg.go.dev`
publishes it, and its reader is a developer, not an archaeologist.

**5. Put the test in the right suite.** `tests/Feature` boots the application
and makes a request; `tests/Unit` checks one thing without booting anything.
External `_test` package, capitalised directory, lowercase package clause. The
one `_internal_test.go` is `app/Http/Controllers/Auth/redaction_internal_test.go`,
and it is there because it needs unexported redaction helpers.
`bash tests/test-layout-guard.sh` checks all four rules.

**6. A fixture that writes behind the policy says why.** `auth.SystemGrant`
carries a `//arandu:system-grant <reason>` line directly above it — nineteen in
production code and seeders (`app/Services/TwoFactorService.go`,
`app/Services/UserService.go` and the three content seeders), and nine more in
five test files, each with its own sentence. One without a reason is the
beginning of the habit this application argues against.

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
