---
name: examples-find-the-pattern
description: Find how something is actually done in a complete, working Arandu (Go) application, and lift the shape into another one. Use when the request is to "show me an example", "how does Arandu do X", "is there a reference implementation", "how do I write a policy / a repository / a service / a controller / a view", "how is CSRF wired", "how does a guest read a public page", "how do I authorize a read", "where is the sitemap", "how do I count something and show it", "what does a real bootstrap look like", or "copy this into my app". Covers the question-to-file table, what each demonstration proves and the test named after it, and what breaks when a snippet is lifted without the paragraph above it.
license: MIT
---

# Reading this application to find an answer

This is a blog with 51 routes, 29 views and 127 tests, and every file in it is
an answer to a question somebody asked. Finding the answer is a lookup, not a
search, and the point of the lookup is that the file also says *why* the shape
is what it is.

Run the gates once before reading, because a stale build is a confusing read:

```sh
export GOWORK=off && aru view:build && go build ./...
```

## Where each answer is

| The question | Read |
| --- | --- |
| what does the whole wiring look like | `bootstrap/app.go` — one function, top to bottom, no container |
| how is a route declared, named and guarded | `routes/web.go`; `routes/admin.go` for a whole area behind a group |
| how does a policy decide | `app/Policies/PostPolicy.go` — the longest of the six, and the one with a guest in it |
| how does a repository take a Grant | `app/Repositories/PostRepository.go:47` (`Find`) and `:76` (`List`) |
| how does a service sit between them | `app/Services/PostService.go:60` — takes a `security.Subject`, asks the policy, passes the Grant down |
| how does a controller assemble a page | `app/Http/Controllers/PostController.go:254` (`Show`), `:332` (`Store`) |
| how is a form validated | `app/Http/Requests/PostRequest.go` — `StorePost.Validate` returns `validation.Errors` |
| how is a view written and typed | `resources/views/posts/show.kyse.go`, and `resources/views/layouts/app.kyse.go` for the layout |
| how is a schema change written | `database/migrations/`, seven of them, oldest first |
| how is a fixture written that has no request behind it | `database/seeders/PostSeeder.go:63` — `SystemGrant` with `//arandu:system-grant` and a reason |
| how does a command reach the same application a request does | `bootstrap/console.go`, `Dispatch` |
| what does a test of a claim look like | `tests/Unit/GrantRequired_test.go`, the whole file |

## The security demonstrations, and what each one proves

These are the reason the application exists. Each is wired, not described, and
each has a test named after the claim.

**A repository call without a Grant does not compile.**
`tests/Unit/testdata/missing_grant/main.go` calls `repo.Find(ctx, "some-id")`
and must fail. `TestRepositoryWithoutGrantDoesNotCompile` runs `go vet` on it
and requires the message `not enough arguments in call to repo.Find`. It is
checked on generated code rather than on the framework's own, which is the part
that matters: a guarantee holding only for hand-written code is not one.

**Carrying a Grant is not enough — it has to be the right one.**
`TestEveryMethodRequiresItsGrant`, same file, hands `Find`, `List`, `Create` and
`Update` a Grant issued for `PostDelete` and requires each to refuse. The
database handed in is `nil` on purpose: a method that panics checked the Grant
too late.

**The public read path is a policy, not a middleware.**
`app/Policies/PostPolicy.go` opens exactly three things to `s.IsGuest()`:
`PostPublicList`, `PostView` with the zero value (permission to look at all), and
`PostView` on a row whose `PublishedAt` is set. `PostList` stays closed, because
it pages through drafts too — so the guest listing is a separate action with a
query of its own rather than the same query with a filter.
`TestThePublicListingHidesADraft` and
`TestAGuestIsRefusedTheDraftBehindAKnownAddress` hold both halves.

**A public page does not need a system grant.**
`SitemapController.Index` calls `c.posts.Published(ctx.Ctx(), security.Guest(c.tenant), 1000)`.
The shortcut would be `security.SystemGrant`, which skips the policy — and would
list every draft, so the crawler finds a redirect behind each one. One rule
answers both "may this be served" and "may this be listed".

**A read that crosses tenants has a name.**
`SocketsController` reads process-wide gauges, which are not scoped to a tenant.
It is a controller of its own, with `SocketMetricsPolicy` and the action
`SocketInspectAll` on `AllTenantSockets` — reachable from no other screen. The
authorization call in `Index` is the only thing between a session and every
tenant's numbers, because the registry it reads is a map and takes no Grant.
`TestTheSocketCountsAreTheOperatorsAndNotAReaders`.

**The tenant never arrives with the request.** `withSubject` in `routes/web.go`
puts the session's subject on the context and nothing else; joaju answers 401
when there is none. `tests/Feature/TenantScoping_test.go` is 513 lines of the
other tenant seeing nothing.

**A link is signed rather than stored.** `RegisterController.go:52` and
`PasswordController.go:58` declare the two purpose strings, and the same
application key signs both — so a verification link cannot be replayed as a
reset. `TestTheVerificationLinkIsSignedAndScoped` and
`TestTheSameLinkTwiceIsNotAnError`. The signing itself is
`framework/modules/auth`; what this repository demonstrates is the wiring and
the scoping, which is the half a project has to get right.

## The observability demonstrations

**The debug console, through the real pipeline.**
`middleware.Observe(cfg.App.IsDev(), fw.Observability.TracingSecret, k.Recorder())`
in `bootstrap/app.go` is the whole of it. `k.Recorder()` is nil outside
development and recording nothing is what production does.
`TestTheConsoleRecordsARealRequest` makes a request, reads `X-Request-ID` off
the response, and finds it at `observability.ConsolePath` — `/_arandu/debug`.
`TestTheConsoleSeesTheQueriesOfTheRequest` goes further and requires the query
to name its origin file, because a console showing a request with no queries
reads exactly like an application that never touched the database.

**The error page.** `middleware.Recover` is first in the pipeline, or a panic in
anything below it escapes without a page. It is given `AppModule`, so your
frames are told from the framework's, and `Diagnose: k.Diagnose`, so what the
modules know about the system right now — the outbox falling behind — appears
next to the failure somebody is already looking at.

**A number a screen draws.** `app/Listeners/SocketGauges.go` declares four
metric names as constants, writes them, and the screen reads them from the
registry. The producer and the reader never meet, which is why a screen can draw
a socket count without holding the socket server. `messages()` in
`SocketsController` reads zero however busy the server is, and the comment says
why that is a decision rather than a gap — half a count is worse than the zero
it replaces.

**Work that outlives the request.** `events.NewModule()` brings the outbox table
and `jobs.NewModule(queueStore)` the jobs table, both over this application's own
database, which is what lets an event commit in the same transaction as the row
it describes. `TestTheEventCommitsWithTheWrite` and `TestARolledBackWriteStoresNoEvent`
are the pair.

## Lifting a shape into your own application

**1. Take the paragraph with the code.** The comment above a function here is
usually the reason the signature is that shape. Lifted without it, the next
person edits it back into the shape it was written to avoid — which has already
happened once in this repository, and `bootstrap/console.go` says so at `db:seed`.

**2. Check what the shape depends on.** Three things travel with almost every
snippet here: a policy that issues the Grant, a tenant that came off the Grant,
and a view whose data is a struct. A handler copied without the first has
nothing to pass and will not compile — that is the design working, not a
porting problem.

**3. Do not copy `bootstrap/app.go` wholesale.** It is one deployment's answer.
`SESSION_DRIVER=memory`, `CACHE_STORE=memory`, an in-process rate limiter, an
in-memory socket broker and a nil scheduler `Locker` are all right for one
instance and wrong for two, and the file names the line to swap in each case.

**4. Generate rather than transcribe.** The posts, comments and categories here
were written by `aru make:module`. Copying the generated output by hand into
another project gets you the shape of an older generator; running the generator
gets you the current one.

**5. Then run the gates**, which are in `AGENTS.md` and start with
`aru view:build`.
