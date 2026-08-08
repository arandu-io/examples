<h1 align="center">arandu-io/examples</h1>

<p align="center">Applications built with Arandu, to read.</p>

<p align="center">
<a href="https://github.com/arandu-io/examples/actions/workflows/ci.yml"><img src="https://github.com/arandu-io/examples/actions/workflows/ci.yml/badge.svg" alt="Build Status"></a>
<a href="https://pkg.go.dev/github.com/arandu-io/examples"><img src="https://pkg.go.dev/badge/github.com/arandu-io/examples.svg" alt="Go Reference"></a>
<a href="https://github.com/arandu-io/examples/tags"><img src="https://img.shields.io/github/v/tag/arandu-io/examples?label=version" alt="Latest Version"></a>
<a href="LICENSE.md"><img src="https://img.shields.io/github/license/arandu-io/examples" alt="License"></a>
</p>


## About these examples

> **Note:** this repository is on an older tree, and porting it is an open
> decision. Read it for the shape of a module — the entity, the policy, the
> repository, the service — and read
> [arandu-io/arandu](https://github.com/arandu-io/arandu) for the tree a new
> project actually gets.

What it demonstrates is the part that is hard to believe until it is on screen:
a repository call that does not compile without a `Grant`, a tenant that cannot
come from the request, and a debug console that reconstructs a request after the
fact.

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
