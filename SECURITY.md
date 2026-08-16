# Security policy

## Supported versions

| Version | Supported |
| --- | --- |
| 1.x | Yes |
| < 1.0 | No |

## Reporting a vulnerability

Please report privately through
[GitHub Security Advisories](https://github.com/amiranmanesh/tgju-api-go/security/advisories/new).
Do not open a public issue for a vulnerability.

Include what you would want if you were fixing it: the version, what an attacker gains,
and the smallest input or request that demonstrates it.

You can expect an acknowledgement within a few days and an assessment within two weeks. If
the report is valid, a fix and an advisory follow, and you will be credited unless you ask
otherwise.

## What is in scope

This project fetches a third-party web page, parses HTML, and serves the result over
HTTP. The interesting attack surface follows from that:

- **Parser handling of hostile HTML.** The scraper runs on whatever tgju.org — or anything
  impersonating it — returns. A crash, a hang or unbounded memory growth from crafted
  markup is a vulnerability. Response bodies are capped (`WithMaxBodyBytes`, 16 MiB by
  default) and the number parsers are fuzzed, but this is the area most worth attacking.
- **Injection through scraped content.** Values from tgju end up in JSON responses, log
  lines and terminal output. Anything that lets scraped content forge a log entry or break
  out of its context is in scope.
- **The HTTP server.** Request smuggling, resource exhaustion, a bypass of the rate
  limiter, or a response that leaks internal detail. Upstream URLs are deliberately kept
  out of error messages; a path that exposes them is a bug.
- **The container image.** It runs as uid 65532 on `scratch` with no shell. A path to
  privilege escalation is in scope.
- **Supply chain.** The module has one dependency; releases carry build provenance
  attestation. Report anything that undermines that.

## What is not in scope

- **Wrong prices.** This project relays what tgju.org publishes and verifies nothing. If
  the numbers are wrong there, they are wrong here. That is a correctness limitation,
  documented in the README, not a vulnerability.
- **tgju.org itself.** Report anything about their site to them.
- **Denial of service by running the service without its defences.** The rate limiter, the
  body cap and the request deadline are on by default; turning them off is your decision.
- **Scraping being possible at all.** It is the stated purpose of the project.

## For operators

- Pin at least the minor version of the image; `latest` moves.
- Keep the rate limiter on, and forward `X-Forwarded-For` so it can tell clients apart.
- Terminate TLS at your proxy; the service speaks plain HTTP by design.
- The process needs no writable filesystem: run it `--read-only`, `--cap-drop ALL`,
  `--security-opt no-new-privileges`.
- `govulncheck` runs in CI on every push and weekly through CodeQL. Run it yourself
  against your build too: `make vuln`.
