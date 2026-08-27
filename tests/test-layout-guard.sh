#!/usr/bin/env bash
# The mechanical guard for the test layout, in order of importance.
#
# The first check is the one that matters most, and it is not a style rule: go
# test only runs a file whose name ends in _test.go. A file named DiskTest.go --
# or disk_Test.go, which is the same mistake with a different hand -- compiles
# into the package as ordinary code and none of its tests ever run. No error, no
# warning, a green build with the suite switched off.
#
# That check earns its keep here more than in any other repository of the
# project. The suite is named by category and by subject -- tests/Feature,
# tests/Unit, Commands_test.go, Registration_test.go -- and a developer arriving
# from a framework that capitalises the same names writes RegistrationTest.go
# out of muscle memory in one stroke, and the toolchain would
# not say a word about it.
#
# Every check that asks the toolchain a question treats a question that could
# not be asked as a failure. A guard that goes green because `go list` broke has
# checked nothing and said everything was fine.
set -uo pipefail

cd "$(dirname "$0")/.." || exit 1

fail=0

# The modules of this repository, by where their go.mod sits. There is one
# today; the loop is what makes the answer survive a second one, because every
# rule below is relative to a module and not to this directory.
#
# Held as lines rather than an array: the bash macOS ships is 3.2, which has no
# mapfile, and a guard that only runs on the build machine is a guard nobody
# runs before pushing.
modules=$(git ls-files '*go.mod' | xargs -n1 dirname | sort -u)
if [ -z "$modules" ]; then
	echo "[FAILED] no go.mod is tracked, so there is no module to check"
	exit 1
fi

# nearest_module answers the module a path belongs to: the closest go.mod at or
# above its directory.
#
# This is the whole of check 2. Anchoring the tests/ directory at the top of the
# repository is wrong the moment a second module exists -- its tests/ tree is
# not at the top -- and matching tests/ anywhere in the path is wrong in the
# other direction, because it accepts app/Services/tests/ as a test tree.
nearest_module() {
	local dir=$1
	while :; do
		if [ -f "$dir/go.mod" ]; then
			printf '%s\n' "$dir"
			return 0
		fi
		[ "$dir" = "." ] && return 1
		dir=$(dirname "$dir")
	done
}

# module_relative answers a path as its module sees it, or nothing if no module
# claims it.
module_relative() {
	local file=$1 root
	root=$(nearest_module "$(dirname "$file")") || return 1
	if [ "$root" = "." ]; then
		printf '%s\n' "$file"
	else
		printf '%s\n' "${file#"$root"/}"
	fi
}

# Nothing below may pass by having nothing to look at. Every one of these checks
# is a statement about a set of files, and every one of them is true of the
# empty set: pointed at an empty tree the guard reports success and has read
# nothing. The counts are what turn that into a failure.
sources=$(git ls-files '*.go' | grep -c '')
suite=$(git ls-files '*_test.go' | grep -c '')

if [ "$sources" -eq 0 ] || [ "$suite" -eq 0 ]; then
	echo "[FAILED] $sources Go files and $suite test files are tracked here."
	echo "         Every check below is true of nothing, so none of them ran."
	exit 1
fi

# 1. A test file the toolchain does not recognise as one.
#
# The pattern is Tests?\.go with a capital T, which is every shape of the
# mistake -- DiskTest.go, disk_Test.go, DiskTests.go, Test.go -- and no false
# positive: latest.go ends in lowercase test.go and a real test file does too.
if offenders=$(git ls-files '*.go' | grep -E 'Tests?\.go$'); then
	echo "[FAILED] go test does not run these, and will not say so:"
	printf '%s\n' "$offenders" | sed 's/^/    /'
	fail=1
fi

# 2. A test outside tests/ has to need an unexported identifier, and says so in
#    its name. Anything else belongs in a category.
#
#    Nothing in this repository is outside tests/ today: all 34 test files sit
#    under tests/Feature or tests/Unit and not one is named _internal_test.go.
#    The check is not therefore idle -- it is the only thing standing between
#    that state and the first test dropped next to a handler in app/, which is
#    where a suite starts to grow a second layout.
while IFS= read -r file; do
	[ -z "$file" ] && continue

	if ! relative=$(module_relative "$file"); then
		echo "[FAILED] $file is under no module, so its place cannot be judged"
		fail=1
		continue
	fi

	case "$relative" in
	tests/*) continue ;;
	esac
	case "$file" in
	*_internal_test.go) continue ;;
	esac

	echo "[FAILED] $file is outside tests/ and is not _internal_test.go"
	fail=1
done < <(git ls-files '*_test.go')

# 3. The directories are capitalised; the package clause is not.
#
#    This pairing is load bearing here rather than decorative: tests/Feature
#    holds package feature_test and tests/Unit holds package unit_test, so the
#    directory reads as a category and the import name stays Go. A
#    capitalised clause is legal Go that nobody writes, and it would quietly put
#    the two conventions in the same place.
#
#    tests/Unit/testdata is read along with the rest, and that is safe: this is
#    a grep for a line, not a parse, so a fixture that is broken on purpose --
#    which is what testdata is for -- cannot turn the check into an error.
inspected=0
while IFS= read -r file; do
	[ -z "$file" ] && continue

	relative=$(module_relative "$file") || continue
	case "$relative" in
	tests/*) ;;
	*) continue ;;
	esac
	inspected=$((inspected + 1))

	if clause=$(grep -n '^package [A-Z]' "$file"); then
		echo "[FAILED] capitalised package clause in $file:"
		printf '%s\n' "$clause" | sed 's/^/    /'
		fail=1
	fi
done < <(git ls-files '*.go')

if [ "$inspected" -eq 0 ]; then
	echo "[FAILED] no module has a tests/ tree, so the package clauses of one were not read"
	fail=1
fi

# 4. Nothing outside the tests reaches the tests tree. It imports testing, and a
#    package that reaches it registers the flags of a test binary into whatever
#    imports it. tests/testcase.go is the one that could: it is an ordinary Go
#    file in package tests, and the only thing keeping it out of the binary is
#    that nothing in app/, bootstrap/ or routes/ imports it.
#
#    The question is asked of the PRODUCTION packages and not of ./..., which
#    also lists the test packages themselves -- and every one of those reaches
#    the tests tree, which is what it is for. Asked that way the check reports a
#    failure on any module that has a tests tree at all, which is every module
#    it is meant to protect.
#
#    This module cannot be listed from a bare clone, and that is this repository
#    and not a defect in the check. Every handler imports
#    storage/framework/views, which `aru view:build` generates out of
#    resources/views/*.kyse.go and which .gitignore keeps out of the tree,
#    because it is build output. Until that command has
#    run, `go list -deps` cannot resolve the import -- and this check then says
#    so and fails, rather than passing on a question it never got to ask. That
#    is why the CI step that runs this guard sits after the step that builds the
#    views.
#
#    The tags are passed because a suite behind one is invisible without them,
#    and so is a production package that ever grows a build tag of its own.
#    Nothing here uses either today; they are kept so the first tagged suite is
#    seen on the day it lands rather than on the day somebody counts.
#
#    kyse is deliberately not among them, and adding it would break this check
#    rather than widen it. The 29 files under resources/views are templates
#    whose build tag is the only thing excluding them from the compiler; listing
#    with that tag hands `go list` a file that opens with @extends and stops it
#    at a parse error.
asked=0
while IFS= read -r module; do
	[ -z "$module" ] && continue
	asked=$((asked + 1))

	# The cache is warmed before anything is measured, and it is not a
	# convenience. `go list` writes "go: downloading <module> <version>" to
	# stderr, which is captured here on purpose so that a module that does not
	# build is reported instead of read as an empty package list -- and those two
	# words then arrive as package names, which is what "no required module
	# provides package v0.12.0" is. CI has a cold cache every run, so this failed
	# there and passed here.
	#
	# Without the `all`, which is not a shortening of it. `go mod download all`
	# walks the whole module graph and writes a hash for every module it meets:
	# run in this repository it leaves go.sum modified, entries for modules
	# nothing here imports and `go mod tidy` removes again. A guard that dirties
	# the tree it is checking makes every later reading of `git status` wrong,
	# and the argument buys nothing: the plain form fetches what the main module
	# builds, which is exactly what `go list` below is about to ask for.
	(cd "$module" && GOWORK=off go mod download >/dev/null 2>&1) || true

	if ! packages=$(cd "$module" && GOWORK=off go list -tags 'integration e2e' ./... 2>&1); then
		echo "[FAILED] go list failed in $module, so nothing was checked there:"
		printf '%s\n' "$packages" | sed 's/^/    /'
		fail=1
		continue
	fi

	production=$(printf '%s\n' "$packages" | grep -vE '/tests(/|$)')
	if [ -z "$production" ]; then
		continue
	fi

	# One import path per line and none of them has a space: the split is wanted.
	# shellcheck disable=SC2086
	if ! dependencies=$(cd "$module" && GOWORK=off go list -tags 'integration e2e' -deps $production 2>&1); then
		echo "[FAILED] go list -deps failed in $module, so nothing was checked there:"
		printf '%s\n' "$dependencies" | sed 's/^/    /'
		fail=1
		continue
	fi

	if reached=$(printf '%s\n' "$dependencies" | grep -E '/tests(/|$)'); then
		echo "[FAILED] a production package in $module reaches the tests tree:"
		printf '%s\n' "$reached" | sed 's/^/    /'
		fail=1
	fi
done < <(printf '%s\n' "$modules")

if [ "$asked" -eq 0 ]; then
	echo "[FAILED] the toolchain was asked about no module, so check 4 did not run"
	fail=1
fi

exit $fail
