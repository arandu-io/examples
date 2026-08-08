<h1 align="center">arandu-io/examples</h1>

<p align="center">A blog, built with Arandu, to read.</p>

<p align="center">
<a href="https://github.com/arandu-io/examples/actions/workflows/ci.yml"><img src="https://github.com/arandu-io/examples/actions/workflows/ci.yml/badge.svg" alt="Build Status"></a>
<a href="https://pkg.go.dev/github.com/arandu-io/examples"><img src="https://pkg.go.dev/badge/github.com/arandu-io/examples.svg" alt="Go Reference"></a>
<a href="https://github.com/arandu-io/examples/tags"><img src="https://img.shields.io/github/v/tag/arandu-io/examples?label=version" alt="Latest Version"></a>
<a href="LICENSE.md"><img src="https://img.shields.io/github/license/arandu-io/examples" alt="License"></a>
</p>

## About this example

Posts, on PostgreSQL, with a sign-in screen. Small enough to read in one
sitting, and every file in it was written by the toolchain rather than by hand —
which is the part worth checking.

```sh
docker compose up -d postgres
cp .env.example .env
aru key:generate
aru migrate

ARANDU_ADMIN_EMAIL=you@example.com ARANDU_ADMIN_PASSWORD=a-long-password aru db:seed
aru dev
```

Then <http://localhost:8080/posts>, and sign in with the address you seeded.

## How it was made

Six commands, in this order. Nothing else was typed except the policy and the
seeder, and both are pointed out below.

```sh
aru new blog --module=github.com/arandu-io/examples
go run github.com/arandu-io/ui@latest auth
aru make:module post --fields "title:string!,slug:string!u,body:text!,published_at:timestamp"
aru view:build
aru migrate
aru db:seed
```

`make:module` wrote twelve files — the model, the policy, the repository, the
service, the request, the controller, its test, the migration and four screens —
and printed the three lines of wiring to paste. It does not paste them: a
generator that edits `bootstrap/app.go` behind your back is a generator whose
output nobody can account for.

## Moving to PostgreSQL was one line

The skeleton runs on SQLite so a fresh checkout needs nothing installed. This
example runs on PostgreSQL, and the whole difference is in `.env`:

```
DB_CONNECTION=pgsql
```

The driver is already blank-imported in `main.go`, the `compose.yml` already
brings a server, and no query changed: every statement the generator writes uses
the portable subset and `?` placeholders, which the dialect rebinds to `$1, $2`
on the way out.

## What to look at, and why

**`app/Policies/PostPolicy.go`** — the generated policy denies everything, and
that is deliberate: a generated policy that allowed anything would be a hole
shipped by default. Opening it is the only authorization decision in this
application, and it is one file.

**`app/Repositories/PostRepository.go`** — every method takes a
`security.Grant`. Delete the argument and the code does not compile. There is no
path from the controller to the table that does not carry one, and that is a
signature rather than a convention.

**`database/seeders/PostSeeder.go`** — the one place holding a system grant, and
it says why on the line: a seeder has no request behind it, so there is no
subject and no policy to ask. `aru doctor` accepts it here because the file is
under `database/seeders`; the same call in a controller is reported.

**`resources/views/posts/*.kyse.go`** — the sources, and the `.go` beside them is
what `aru view:build` generated. A field that does not exist is a build error at
the line you wrote in the `.kyse.go`.

## What this example does not do, and it matters

**There is no public reading.** A visitor who is not signed in is redirected to
the sign-in screen, including on `/posts`.

That is not an omission in this example — it is the shape of the framework
today. `security.Authorize` refuses an anonymous subject before it consults the
policy, there is no guest `Subject`, and the only way to reach a repository
without a session is `security.SystemGrant`, which `aru doctor` reports outside
a seeder, a job or a command.

So this is an authoring tool with a login, not a public blog. Serving a page to
somebody who is not signed in is a decision for the framework, and it is the
same decision a public website needs. `app/Policies/PostPolicy.go` says so where
the rule would go.

## Learning Arandu

The API reference is generated from the doc comments and lives on
[pkg.go.dev](https://pkg.go.dev/github.com/arandu-io/framework). Every exported
symbol carries one, and that is deliberate: it is the documentation that cannot
drift from the code, because it sits in the same file.

The CLI documents itself. `aru help` lists every command, and each one explains
what it writes and what to do with it. `aru doctor` explains what it found and
what breaks, not which rule was violated.

A guide and a website do not exist yet, and that is a decision rather than a
gap: a guide written against an API that still moves is work done twice, and the
second time is worse — there is wrong documentation published. The site is the
next phase, and it will be an Arandu application.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). Before opening a pull request, the three
commands at the top of that file have to pass, and CI runs exactly them.

## Security Vulnerabilities

Please review [our security policy](SECURITY.md) on how to report a
vulnerability. Never open a public issue for one.

## License

Open-sourced software licensed under the [MIT license](LICENSE.md).
