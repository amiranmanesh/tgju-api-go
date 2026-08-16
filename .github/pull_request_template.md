## What changed

<!-- One paragraph. What does this do that the code did not do before? -->

## Why

<!-- The problem, not the patch. Link the issue if there is one. -->

Closes #

## How it was verified

- [ ] `make ci` passes
- [ ] New behaviour has a test that fails without the change
- [ ] `make live` still works against the real tgju.org (if the scraper changed)

## Notes for the reviewer

<!--
Anything that would take you ten minutes to rediscover: a decision you went back
and forth on, a fixture you refreshed, a piece of tgju markup that surprised you.
-->

## Checklist

- [ ] Exported identifiers have doc comments
- [ ] `CHANGELOG.md` has an entry under `Unreleased` (skip for docs and CI only changes)
- [ ] `server/openapi.yaml` matches the routes (if an endpoint changed)
