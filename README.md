<h1 align="center">arandu-io/examples</h1>

<p align="center">A blog, built with Arandu, to read.</p>

<p align="center">
<a href="https://github.com/arandu-io/examples/actions/workflows/ci.yml"><img src="https://github.com/arandu-io/examples/actions/workflows/ci.yml/badge.svg" alt="Build Status"></a>
<a href="https://pkg.go.dev/github.com/arandu-io/examples"><img src="https://pkg.go.dev/badge/github.com/arandu-io/examples.svg" alt="Go Reference"></a>
<a href="https://github.com/arandu-io/examples/tags"><img src="https://img.shields.io/github/v/tag/arandu-io/examples?label=version" alt="Latest Version"></a>
<a href="LICENSE.md"><img src="https://img.shields.io/github/license/arandu-io/examples" alt="License"></a>
</p>

Posts, comments, a moderation panel and a password reset. Small enough to read
in one sitting, and almost every file in it was written by the toolchain rather
than by hand — which is the part worth checking.

---

## Run it

Five commands, nothing installed. It runs on SQLite, in a file under
`database/`.

```sh
git clone https://github.com/arandu-io/examples.git blog && cd blog

cp .env.example .env
aru key:generate                      # copy the line it prints into .env
aru migrate

ARANDU_ADMIN_EMAIL=you@example.com \
ARANDU_ADMIN_PASSWORD=a-very-long-password-here \
  aru db:seed

aru dev
```

Then <http://localhost:8080/posts>, and sign in with the address you seeded.

You will land on three posts, one of them a draft. There is a moderation panel
at `/admin`, a sitemap at `/sitemap.xml`, and a password reset at
`/auth/password` — the link it issues is printed in the log rather than sent,
because an example that needs an SMTP server is an example nobody runs.

### On PostgreSQL instead

One line, and no query changes:

```sh
docker compose up -d postgres
sed -i '' 's/^DB_CONNECTION=.*/DB_CONNECTION=pgsql/' .env
aru migrate
```

Every statement the generator writes uses the portable subset and `?`
placeholders, which the dialect rebinds to `$1, $2` on the way out.

---

## Do it yourself, from nothing

This is the whole application, in the order it was built. Nine commands and four
files written by hand — and the four are pointed out, because what a person
still has to decide is the interesting part.

### 1. The project

```sh
aru new blog --module=github.com/you/blog
cd blog
```

You get the tree a Laravel developer recognises: `app/Http/Controllers`,
`app/Models`, `app/Policies`, `routes/web.go`, `resources/views`,
`database/migrations`. It runs already — `aru dev` and there is a landing page.

### 2. The sign-in screens

```sh
go run github.com/arandu-io/ui@latest auth
```

Nine screens, in kyse, at the paths people look for. Nothing is added to your
`go.mod`: `go run <module>@latest` runs a published module without touching your
dependency graph, and the files are yours the moment they land.

It prints two lines to paste into `bootstrap/app.go`. Paste them — a publisher
that edits your wiring behind your back is one whose output nobody can account
for.

### 3. The components

```sh
go get github.com/arandu-io/kyse/components
```

Eleven components — button, field, textarea, card, dialog, avatar, badge,
alert, toast, empty, theme toggle — adapted from shadcn. **Imported, not
copied**: nothing lands in `resources/views/`, and `aru view:build` has nothing
extra to compile.

Each is an ordinary exported Go function:

```go
{!! components.Field(components.FieldProps{
	Name:  "title",
	Label: "Title",
	Value: .Form.Title,
	Error: .FieldError("title"),
}) !!}
```

A component that does not exist is `undefined: components.Buton`, and a prop
that does not exist is `unknown field Labl` — both at the line of the
`.kyse.go` you wrote.

### 4. The posts

```sh
aru make:module post --fields "title:string!,slug:string!u,body:text!,published_at:timestamp"
```

Twelve files: the model, the policy, the repository, the service, the request,
the controller, its test, the migration and four screens. It prints the three
lines of wiring to paste.

**Hand-written file 1 — `app/Policies/PostPolicy.go`.** The generated policy
denies everything, and that is deliberate: a generated policy that allowed
anything would be a hole shipped by default. Opening it is the only
authorization decision in this application, and it is one file.

### 5. The comments

```sh
aru make:module comment --fields "post_id:string!,author:string!,body:text!,approved:bool"
```

**Hand-written file 2 — `app/Policies/CommentPolicy.go`.** Four rules: anybody
signed in may read the thread and add to it; approving and deleting are the
administrator's; and you may delete your own comment while it is still awaiting
review, but not after somebody has replied under it.

Watch the shape of the admin rule:

```go
if s.HasRole("admin") && (a == CommentUpdate || a == CommentDelete) {
	return nil
}
```

Named actions, not "an administrator may do anything". Written the second way,
the policy answers `nil` for an action nobody has defined yet — a hole that
opens itself the next time somebody adds one. The generated test caught exactly
that.

The thread hangs off the post rather than living at `/comments`:

```go
r.Action("POST", "/posts/{id}/comments", d.Comment.Store).Name("posts.comments")
```

and the post and the author come from the route and the session, never from the
form. A hidden field carrying either is a hidden field somebody edits.

### 6. The moderation panel

**Hand-written file 3 — `app/Http/Controllers/AdminController.go`**, with
`routes/admin.go` and `resources/views/admin/`.

An area rather than a prefix: one group, one middleware, its own layout with a
sidebar. It owns no data — the posts and the comments are the same records the
public screens read, through the same services and the same policies. An
administration screen that reached the database another way would be a second
enforcement point, and the second one is always the one that is wrong.

### 7. The password reset

**Hand-written file 4 — `app/Http/Controllers/Auth/PasswordController.go`.**

The kit publishes the three screens and stops there, on purpose: the handlers
write to your users table, send through your mailer and decide your rules.

What is here is worth reading for what it refuses to do. The token is random,
single-use and expires in an hour. It is stored **hashed**, so the store is not
a store of live reset links, and it is compared with `hmac.Equal` because the
comparison is against a secret. And asking for a link answers the same thing
whether the address is registered or not — a form that says "no such account" is
an oracle for which addresses exist, one request at a time.

### 8. Build and run

```sh
aru view:build      # kyse -> Go, and the stylesheet
aru migrate
aru db:seed
aru dev
```

---

## What to look at, and why

**`resources/views/`** — the tree you wrote, and the compiled `.go` beside each
source. `.vscode/settings.json` hides the generated ones, so the folder shows
what you edit. They stay in git: a project whose generated files are missing is
a project that does not build from a fresh clone.

**`app/Repositories/PostRepository.go`** — every method takes a
`security.Grant`. Delete the argument and it does not compile. There is no path
from a controller to a table that does not carry one, and that is a signature
rather than a convention.

**`app/Repositories/grant_required_test.go`** — the proof, as a test: a
repository call without a Grant is a build failure, checked by compiling one.

**`app/Http/Controllers/SitemapController.go`** — the one place in the
application that holds a system grant, and it says why on the line. A crawler
has no session, and `security.Authorize` refuses an anonymous subject before it
consults a policy, so the choice was between a sitemap that is empty for
everybody and a named, single-action grant. `aru doctor` reports this call
outside a seeder, a job or a command; the marker above it declares the exception
out loud instead of working around the rule.

**`resources/css/basecoat/`** — the design system, vendored with its licence.
It is a directory of this project, not a dependency: changing how a button looks
is editing a file you can see. `components.css` lists the twenty-one components
in use rather than importing all thirty-eight — 183 kB minified, 18.7 kB
gzipped, smaller than Bootstrap.

**No `package.json`, no `node_modules`, no lockfile.** The CSS is compiled by a
single standalone binary the CLI downloads and pins; the scripts are embedded in
your binary. The Content-Security-Policy is `script-src 'self'`, so a CDN would
not run even if one were referenced.

---

## What this example does not do, and it matters

**There is no public reading.** A visitor who is not signed in is redirected to
the sign-in screen, including on `/posts`.

That is not an omission here — it is the shape of the framework today.
`security.Authorize` refuses an anonymous subject before it consults the policy,
there is no guest `Subject`, and the only way to reach a repository without a
session is `security.SystemGrant`, which `aru doctor` reports outside a seeder,
a job or a command.

So this is an authoring tool with a login, not a public blog. Serving a page to
somebody who is not signed in is a decision for the framework, and it is the
same decision a public website needs. `app/Policies/PostPolicy.go` says so where
the rule would go.

**The reset store is in memory.** Right for one instance and wrong for two, in
exactly the way the session store is: behind a load balancer the link works only
on the replica that issued it. Moving it is one type, and saying so is better
than a store that looks distributed and is not.

**The reset does not write the password.** The framework's auth service owns the
users table and exposes no `SetPassword`, deliberately: a minimum length, a
history, a notification are decisions about your rules. The flow up to that
point is complete, and the token is consumed before the gap.

---

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
