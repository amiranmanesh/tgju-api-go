# Contributing

Thank you for looking. This project is small on purpose, and the guidance below is mostly
about keeping it that way.

## Getting started

```bash
git clone https://github.com/amiranmanesh/tgju-api-go
cd tgju-api-go
make ci
```

Go 1.26 or newer, and nothing else. `make help` lists every target.

## The development loop

```bash
make test        # the suite, race detector on
make cover       # coverage
make bench       # benchmarks
make fuzz        # fuzz the number parsers for thirty seconds
make live        # hit the real tgju.org
make run         # serve on :8080
make golangci    # the full linter, if you have it installed
```

## What a good change looks like

**A test that fails without it.** For a parsing change this is easy: put the markup that
broke into a test, watch it fail, fix it. `internal/scrape/scrape_test.go` is full of
examples that use a string of HTML inline rather than a whole fixture.

**A doc comment on anything exported.** Say why the thing exists and what a caller should
know, not what the signature already says. The comments in `options.go` and `errors.go`
are the house style.

**An entry in `CHANGELOG.md`** under `Unreleased`, unless the change is documentation or
CI only.

**No new dependency.** The policy is the standard library plus `golang.org/x`, enforced by
`make deps-check`. It is not asceticism: this library is imported into other people's
services, and every dependency it carries becomes theirs. If you genuinely need one, open
an issue and make the case first.

## Commit messages

[Conventional Commits](https://www.conventionalcommits.org/). The release notes are
grouped by prefix, so the prefix matters.

```
feat(server): add /v1/snapshot
fix(scrape): follow the header when tgju reorders columns
perf(numfmt): avoid an allocation per cell
docs(wiki): document the coin keys
test(client): cover a cancelled context during a cache miss
ci: pin actions/checkout to v5
refactor(dom): return a Node alias instead of html.Node
```

Use `!` or a `BREAKING CHANGE:` footer for anything that changes the public API or the
wire format.

## Where things belong

| Change | Where |
| --- | --- |
| tgju changed its markup | `internal/scrape` — and nowhere else |
| A new field on `Item` | `snapshot.go` and `convert.go` |
| A new market | `market.go`, plus a fixture — see the wiki |
| A new endpoint | `server/`, plus `server/openapi.yaml` |
| Persian number handling | `internal/numfmt` |

The separation between `internal/scrape` and the rest is the load-bearing decision of this
codebase. If a fix for an upstream redesign has to touch `client.go` or `server/`, the
abstraction has sprung a leak; say so in the pull request rather than quietly working
around it.

## Fixtures

The tests parse saved tgju pages in `internal/fixture/testdata`. To refresh them:

```bash
make fixtures
go test ./...
```

The tool refetches all three boards, strips the tooltip attributes and inline CSS, and
keeps a handful of rows per table. **The diff is the review.** It should be small and it
should be about markup; if it is large, look at what else changed on the page before
trusting it. Tests that pin exact prices will need updating too — that is deliberate, so
that a fixture refresh is a conscious act.

## Adding a market

See [Adding a market](https://github.com/amiranmanesh/tgju-api-go/wiki/Adding-a-Market) in
the wiki. It is about ten lines plus a fixture, provided tgju server-renders the page.

## Reporting an upstream break

If prices stop parsing, use the
[tgju changed its markup](https://github.com/amiranmanesh/tgju-api-go/issues/new?template=upstream_change.yml)
template. It asks for the one thing that makes the fix a single commit: the new markup.
One `<table>` element with a couple of rows is worth more than any description of it.

## Releasing

Maintainers only.

1. `CHANGELOG.md`: move `Unreleased` into a dated version section.
2. `version.go`: bump `Version`. It must match the tag without the `v`.
3. `make release-check`.
4. `git tag -a v1.2.0 -m "..."` and push the tag.

The release workflow re-runs the suite on the tagged commit, refuses to publish if the
tag, `version.go` and the changelog disagree, then builds the binaries and the multi-arch
image and warms the module proxy.

## One-time repository setup

Two things cannot be configured from a file in this repository, and both are noted here so
a new maintainer is not left guessing:

- **GitHub Pages** — Settings → Pages → Source: **GitHub Actions**. Without it the Pages
  workflow fails at `configure-pages`.
- **Container package visibility** — a package pushed to GHCR is private on first
  publish, whatever the repository's own visibility is, and GitHub exposes no REST
  endpoint to change that. Once, at
  `https://github.com/users/<owner>/packages/container/tgju-api-go/settings`, set the
  visibility to **Public**. Until then `docker pull ghcr.io/…` fails for everyone else
  and the repository sidebar reports no packages, even though the release workflow
  published one.

- **`WIKI_TOKEN`** — `GITHUB_TOKEN` cannot write to a wiki repository; no permission scope
  grants it, so the automatic token gets a 403. Create a fine-grained personal access
  token with **Contents: read and write** on this repository and add it as the
  `WIKI_TOKEN` secret. Without it the wiki workflow explains itself and passes, and the
  wiki has to be published by hand:

  ```bash
  git clone git@github.com:amiranmanesh/tgju-api-go.wiki.git
  cp wiki/*.md tgju-api-go.wiki/
  cd tgju-api-go.wiki && git add -A && git commit -m "docs: sync the wiki" && git push
  ```

## Code of conduct

Be decent. The full text is in [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md).
