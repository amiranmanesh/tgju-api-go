# Contributing

The full guide is [CONTRIBUTING.md](https://github.com/amiranmanesh/tgju-api-go/blob/main/CONTRIBUTING.md)
in the repository. This page is the short version.

## Setup

```bash
git clone https://github.com/amiranmanesh/tgju-api-go
cd tgju-api-go
make ci     # lint, dependency policy, spec check, tests
```

Go 1.26 or newer. There is nothing else to install.

## The loop

```bash
make test        # race detector on
make cover       # coverage
make live        # hit the real tgju.org
make run         # serve on :8080
make fixtures    # refresh the saved pages after an upstream change
```

## What a change should look like

- **A test that fails without it.** The fixtures make this easy for parsing changes: add
  the markup that broke, watch it fail, fix it.
- **A doc comment on anything exported**, saying why it exists rather than restating the
  signature.
- **A `CHANGELOG.md` entry** under `Unreleased`, unless the change is docs or CI only.
- **No new dependency.** The policy is the standard library plus `golang.org/x`, and
  `make deps-check` enforces it. If you genuinely need one, open an issue first.

## Commit messages

Conventional Commits, because the release notes are grouped by them:

```
feat(server): add /v1/snapshot
fix(scrape): follow the header when tgju reorders columns
docs(wiki): document the coin keys
test(numfmt): fuzz the change parser
```

## Where things live

Fragile HTML knowledge belongs in `internal/scrape` and nowhere else. If a fix to a tgju
redesign touches `client.go` or `server/`, the abstraction has sprung a leak and it is
worth a comment in the pull request explaining why.
