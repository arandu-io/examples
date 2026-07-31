# arandu examples

An application that demonstrates the Arandu framework, and a guided tour that
makes each claim visible on screen.

It has a second job, and that one matters more: **this is the shape
`aru make:module` will generate.** The module below is written by hand so that
the generator has something proven to copy, rather than a template someone
thought looked good.

## Run it

Nothing to install. The default connection is SQLite, a file under `database/`.

```
cp .env.example .env
aru key:generate          # paste the line into .env
aru migrate
aru db:seed
aru serve
```

Then open `/demo` and sign in at `/auth/login` with the credentials from
`.env` (`admin@example.test`).

Moving to PostgreSQL is `.env` and nothing else — no code changes, because the
portability lives in the SQL rather than in an abstraction over it:

```
DB_CONNECTION=pgsql
DB_DATABASE=arandu
DB_HOST=127.0.0.1
DB_USERNAME=arandu
DB_PASSWORD=arandu
```

## The tour

| Route | The claim it makes visible |
|---|---|
| `/demo/n-plus-one` | the page names the N+1 by itself, with the repeated statement and the stack that led to it |
| `/demo/batched` | the same page done right: two queries whatever the number of customers |
| `/demo/slow-query` | the diagnosis says which statement to look at |
| `/demo/dump` | values recorded with origin and timing, and the customer document absent |
| `/demo/panic` | your frames expanded, the framework's collapsed, queries still there |
| `/demo/other-tenant` | a real id from another tenant answers "not found", not "forbidden" |
| `/demo/no-grant` | the half a running program cannot show: it does not compile |

The last one is worth reading even without running anything:

```
go test ./modules/customer/ -run TestRepositoryWithoutGrantDoesNotCompile -v
```

Two fixtures under `testdata/`. One omits the Grant argument; the other tries to
forge a Grant and cannot, because every field of `security.Grant` is unexported.
The test runs the toolchain over both and **requires** the failure, with the
specific message — a fixture that fails for an unrelated reason would prove
nothing.

## The module

`modules/customer/` is the canonical shape. One feature, one directory:

```
module.go            registration, routes, migrations, health
customer.entity.go   the entity, and what it refuses to reveal
customer.policy.go   who may do what; denies by default
customer.repo.go     data access, requires a Grant
customer.service.go  business rules; Authorize -> Grant -> Repository
customer.request.go  input types and Validate
handlers.go          thin: extract, delegate, render
```

Three decisions in there are worth copying into your own modules:

**Reading a record and reading every field of it are different permissions.**
`customer.view` lets support see a customer; `customer.view_full_document` is
what it takes to see the unmasked registration number, and reading it is always
an audit event. Most systems learn this distinction during an incident.

**The entity refuses to serialize its own secret.** `MarshalJSON` and `LogValue`
are methods on the type, so the document stays out of responses, logs and the
debug page without any handler having to remember.

**Read before write.** `Update` loads the stored row and runs the policy against
that, not against what the client claims the row is. Skipping it is how a check
passes on attacker-supplied data.

## What is not here

No view layer — that is `porang`, in phase 2, and it has its own specification
(templ, Tailwind standalone, zero Node). The handlers answer JSON, and when the
view layer lands they gain a branch that returns an HTML fragment: the module
contract does not change.

No generator either. This repository is the generator's input, not its output.

## License

MIT, the same license Laravel uses. See `LICENSE.md`.
