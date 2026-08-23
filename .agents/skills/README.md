# Skills

Procedures an assistant follows when working with this application.

They live in `.agents/skills/<name>/SKILL.md`, which is the path the coding
assistants read from — Cursor, Codex, Cline, Copilot, Gemini CLI, Amp, OpenCode,
Warp, Zed and the rest all look there. It is one directory rather than a file
per vendor, so a skill written once is read by whatever this project is being
written with.

Each file opens with frontmatter carrying a `name` and a `description`. The
description is what a tool reads to decide whether the skill is relevant, so it
names the situation you are in rather than the topic it covers.

This repository has two audiences and they want opposite things. Somebody
learning wants to find where one thing is done and copy the shape into their own
application. Somebody maintaining wants to change this one without quietly
retiring a claim it makes. The skills split along that line, with the third
covering the state everybody hits first: getting it to run.

| skill | when it fires |
| --- | --- |
| `examples-find-the-pattern` | reading this application to learn how something is done, and lifting the shape into another one |
| `examples-run-the-blog` | starting it, migrating it, seeding it, and the three ways a fresh clone fails |
| `examples-keep-it-true` | changing it, or upgrading the framework under it, without leaving a demonstration behind |

## Why these exist

An example application is read by a model as a bag of snippets, and this one is
not that. Nearly every file here carries the reason its shape is what it is —
why the sitemap authorizes as a guest instead of holding a system grant, why the
socket screen has a policy nothing else can reach, why a listener returns
nothing. Lift the code and drop the reason and you have copied the half that was
already obvious.

The other half of the answer is that this repository is built to be checked
rather than trusted. The claims it makes have tests named after them, the gates
are seven commands, and the numbers in its prose were measured with commands
that are written down beside them. An assistant that runs those is not guessing.

## Adding your own

A skill in this directory travels with the repository. Keep it a procedure
rather than a description: a file that says "read the documentation" never
changes what anybody does. Every command in one has to run, and every number in
one has to have been measured.
