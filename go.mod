module github.com/amiranmanesh/tgju-api-go

go 1.26

require golang.org/x/net v0.58.0

// A retraction is published by the versions that follow it, so this directive
// is what tells the module proxy — and therefore everyone's `go get` — not to
// select the withdrawn version.
// Requires golang.org/x/net v0.46.0, which carries five vulnerabilities
// that govulncheck traces into html.Parse. That is the whole HTML parsing
// path, so every scrape reached them. Use v1.0.1 or later.
retract v1.0.0
