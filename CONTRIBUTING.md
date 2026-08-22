# Contributing

## Sign your commits

Every commit needs a `Signed-off-by` line:

```
git commit -s -m "what changed and why"
```

That line is the [Developer Certificate of Origin](https://developercertificate.org/):
you are stating that you wrote the change, or that you have the right to submit
it under this project's license. It is not a copyright assignment — you keep
your copyright, and this project can never be relicensed behind your back.

We use DCO rather than a CLA on purpose. A CLA would let the project relicense
later, and the price is that every contributor has to sign a legal document
before their first patch.

## Before you open a pull request

```
gofmt -l $(find . -name '*.go' -not -name '*.kyse.go')
go vet ./...
go test -race ./...
```

CI runs these and a handful of checks besides; the list is
`.github/workflows/ci.yml`, and that file is what decides -- not this paragraph,
which will fall behind it. One rule worth knowing before you push: the framework
depends on the standard library and `golang.org/x/crypto`, and nothing else. A
pull request that adds a dependency there needs to argue for it first, in an
issue.

## Where a test goes

In `tests/`, under `Feature/` or `Unit/`, declaring an external `_test` package.
That is the default, and it is where nearly every test belongs: it sees what a
caller sees, which is the point of testing a contract. The directory names are
capitalised and the package clause is not, so a file under `tests/Feature`
declares `package feature_test`.

The exception is a test that genuinely needs an identifier its package does not
export. It cannot live in `tests/`, because an external package reaches only
what is exported, so it sits beside the code it tests, inside that package, and
says so in its name: `*_internal_test.go`. The name is the load-bearing part --
it is what keeps the exception legible instead of ambient, so a test outside
`tests/` carries that name or it does not belong outside `tests/`.

The coverage argument lands on that exception rather than against the tree:
`go test` attributes coverage per directory, so the test compiled into the
package is the one that reports against it, and a suite under `tests/` is
measured against itself unless coverage is asked for across packages.

Take the exception only when you use it -- `plans/testpackages.go` in the
arandu-io working tree checks exactly that, by intersecting the identifiers a
test names with what its package declares unexported, and the checklist runs it
across every Go repository in the project.

A `package main` has no external form: it cannot be imported, so its tests are
internal -- beside `main.go`, and named `_internal_test.go` like any other.

## What the commit message says

What changed and why. The why is the part that is not in the diff, and it is the
part someone will need in two years.

No AI attribution of any kind: no `Co-Authored-By` for an assistant, no
"generated with" footer. Commits are authored by the people who submit them.

## Architecture decisions

The decisions this project has already made live at arandu.io/docs, and every
one that closed a door has an ADR. If your change contradicts one, say so in the
pull request and argue for the change of decision — that is a normal thing to
do, and it is better than a patch that quietly works around it.
