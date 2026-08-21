<p align="center">
  <img src=".github/logo.png" alt="Arandu" width="140" height="140">
</p>

<h1 align="center">arandu-io/examples</h1>

<p align="center">A complete Arandu application, small enough to read in one sitting.</p>

<p align="center">
<a href="https://github.com/arandu-io/examples/actions/workflows/ci.yml"><img src="https://github.com/arandu-io/examples/actions/workflows/ci.yml/badge.svg" alt="Build Status"></a>
<a href="https://pkg.go.dev/github.com/arandu-io/examples"><img src="https://pkg.go.dev/badge/github.com/arandu-io/examples.svg" alt="Go Reference"></a>
<a href="https://github.com/arandu-io/examples/tags"><img src="https://img.shields.io/github/v/tag/arandu-io/examples?label=version" alt="Latest Version"></a>
<a href="LICENSE.md"><img src="https://img.shields.io/github/license/arandu-io/examples" alt="License"></a>
</p>

A blog: posts filed into sections, a comment thread that requires a confirmed
address, a moderation panel, and a password reset. Almost every file in it was
written by the toolchain rather than by hand, which is the part worth reading —
two policies (`PostPolicy`, `CommentPolicy`) and three controllers
(`AdminController`, `PasswordController`, `RegisterController`) are the only
files a person had to decide, everything else came out of `aru make:module`.

## What it delivers

- **A repository call that cannot skip authorization** — every method on
  `PostRepository` and `CommentRepository` takes a `security.Grant`, and
  `tests/Unit/GrantRequired_test.go` proves the absence does not compile — on
  the generated code itself, not on the framework's own.
- **A public read path decided entirely by policy** — a guest gets the
  published listing and any published post; a draft answers 403; none of it
  is middleware, all of it is `PostPolicy.Can`.
- **A moderation panel that owns no data of its own** — `/admin` reads the
  same posts and comments through the same services and the same policies as
  the public screens, rather than opening a second path to the database.
- **A verification link that is signed, not stored** — no table, no cleanup
  job; the purpose is part of the HMAC, so a verification link cannot be
  replayed as a password reset even though the same key signs both.
- **A sitemap that cannot leak a draft** — it authorizes as a guest through
  the same `PostPolicy` a browser uses, instead of holding a system grant that
  would list everything.

18,060 lines of production code and 3,721 of test, across 26 test files.
Built against `arandu-io/framework`, `arandu-io/kyse` and `arandu-io/hesape`,
with a PostgreSQL and a SQLite driver both wired through `.env` — an example
is meant to show the shape of a real deployment, so it defaults to Postgres
rather than the skeleton's SQLite default.

Nothing is actually mailed: `MAIL_URL=log://`, the default, writes verification
and password-reset links to the output of `aru dev` instead of sending them, so
the example runs without an SMTP server.

Known limit: the password-reset token store is in memory, correct for one
instance and wrong for two — right behind a load balancer, the link only
works on the replica that issued it.

## Run it

```sh
git clone https://github.com/arandu-io/examples.git blog && cd blog
docker compose up -d postgres
cp .env.example .env && aru key:generate && aru migrate && aru db:seed
aru dev
```

Then `http://localhost:8080`. `aru db:seed` prints two accounts — one that
writes, publishes and moderates, one that only comments — both created with a
password that is refused outside development.

## The rest of Arandu

`aru make:module` is what generated every file here but the five listed
above; `arandu-io/framework` is what it ran against; `arandu-io/arandu` is the
skeleton this application started from; `hesape` is the 47-package collection
underneath the framework.

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
