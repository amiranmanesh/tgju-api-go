package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// site writes one HTML file into a temporary directory and returns the
// directory.
func site(t *testing.T, body string) string {
	t.Helper()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "page.html"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestAcceptsAPinnedScriptWithIntegrity(t *testing.T) {
	t.Parallel()

	const page = `<script
	  src="https://cdn.jsdelivr.net/npm/thing@1.65.1"
	  integrity="sha384-NAMzfHXRsxRYhcKmRnZGVLvlBeXTWtpYd0jWgeZ7fk89X95GIJBK1H4bUwkP4IZJ"
	  crossorigin="anonymous"></script>`

	problems, err := check(site(t, page))
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if len(problems) != 0 {
		t.Errorf("rejected a correctly fenced script: %v", problems)
	}
}

func TestRejectsAFloatingVersion(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"major range": `https://cdn.jsdelivr.net/npm/thing@1`,
		"minor range": `https://cdn.jsdelivr.net/npm/thing@1.65`,
		"latest":      `https://cdn.jsdelivr.net/npm/thing@latest`,
		"no version":  `https://cdn.jsdelivr.net/npm/thing`,
		"caret range": `https://cdn.jsdelivr.net/npm/thing@^1.2.3`,
	}

	for name, src := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			page := `<script src="` + src + `" integrity="sha384-AAAA"></script>`
			problems, err := check(site(t, page))
			if err != nil {
				t.Fatalf("check: %v", err)
			}
			if !strings.Contains(strings.Join(problems, "\n"), "not pinned") {
				t.Errorf("%s was accepted: %v", src, problems)
			}
		})
	}
}

func TestRejectsAMissingIntegrityHash(t *testing.T) {
	t.Parallel()

	page := `<script src="https://cdn.jsdelivr.net/npm/thing@1.65.1"></script>`

	problems, err := check(site(t, page))
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if !strings.Contains(strings.Join(problems, "\n"), "no integrity hash") {
		t.Errorf("a script without an integrity hash was accepted: %v", problems)
	}
}

func TestRejectsAMalformedIntegrityHash(t *testing.T) {
	t.Parallel()

	// "sha1" is not an accepted SRI algorithm, and an empty digest is not a
	// hash at all.
	for _, bad := range []string{`sha1-abc`, `sha384-`, `not-a-hash`} {
		page := `<script src="https://cdn.example/thing@1.0.0" integrity="` + bad + `"></script>`

		problems, err := check(site(t, page))
		if err != nil {
			t.Fatalf("check: %v", err)
		}
		if len(problems) == 0 {
			t.Errorf("integrity=%q was accepted", bad)
		}
	}
}

// TestSameOriginScriptsAreExempt keeps the rule proportionate: a file in this
// repository is reviewed like any other, and demanding a hash for it would only
// mean the hash goes stale on every edit.
func TestSameOriginScriptsAreExempt(t *testing.T) {
	t.Parallel()

	const page = `<script src="./api-init.js"></script>
	<script src="/assets/app.js"></script>`

	problems, err := check(site(t, page))
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if len(problems) != 0 {
		t.Errorf("same-origin scripts were flagged: %v", problems)
	}
}

// TestProtocolRelativeCountsAsExternal — "//cdn.example/x" inherits the scheme,
// not the host, so it is somebody else's script.
func TestProtocolRelativeCountsAsExternal(t *testing.T) {
	t.Parallel()

	const page = `<script src="//cdn.example/thing@1"></script>`

	problems, err := check(site(t, page))
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if len(problems) == 0 {
		t.Error("a protocol relative script was treated as same-origin")
	}
}

func TestReportsAMissingDirectory(t *testing.T) {
	t.Parallel()

	if _, err := check(filepath.Join(t.TempDir(), "absent")); err == nil {
		t.Fatal("a missing directory was not reported")
	}
}

// TestTheRealSite is the check that matters: the site this repository publishes
// must satisfy its own rules.
func TestTheRealSite(t *testing.T) {
	t.Parallel()

	problems, err := check(filepath.Join("..", "..", "..", "docs"))
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if len(problems) != 0 {
		t.Errorf("docs/ violates the supply-chain rules:\n%s", strings.Join(problems, "\n"))
	}
}
