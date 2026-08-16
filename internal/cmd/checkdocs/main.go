// Command checkdocs enforces the supply-chain rules for the GitHub Pages site.
//
// The published site lives on a shared origin — every project page under one
// github.io account is the same origin — so a third-party script on the docs
// site is not sandboxed to this project. Two rules follow, and this tool is
// what keeps them true after the person who wrote them has moved on:
//
//  1. An external script must be pinned to an exact version. A floating tag
//     such as @1 or @latest means the bytes a visitor executes can change
//     without anyone reviewing them.
//
//  2. An external script must carry a Subresource Integrity hash, so a
//     compromised or hijacked artefact fails to execute rather than running.
//
// Same-origin scripts are exempt: they are in this repository and are reviewed
// like any other file.
//
//	go run ./internal/cmd/checkdocs docs
package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var (
	scriptRE  = regexp.MustCompile(`(?is)<script\b[^>]*\bsrc\s*=\s*"([^"]+)"[^>]*>`)
	srcRE     = regexp.MustCompile(`(?is)\bsrc\s*=\s*"([^"]+)"`)
	integrity = regexp.MustCompile(`(?is)\bintegrity\s*=\s*"\s*sha(256|384|512)-[A-Za-z0-9+/=]+\s*"`)

	// A pinned npm-style specifier ends in @<major>.<minor>.<patch>. Anything
	// shorter is a range that resolves at request time.
	pinnedRE = regexp.MustCompile(`@\d+\.\d+\.\d+(?:[-+][0-9A-Za-z.-]+)?(?:/|$)`)
)

func main() {
	dir := "docs"
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}

	problems, err := check(dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "checkdocs:", err)
		os.Exit(1)
	}
	if len(problems) > 0 {
		for _, p := range problems {
			fmt.Fprintln(os.Stderr, "checkdocs:", p)
		}
		os.Exit(1)
	}

	fmt.Printf("checkdocs: every external script in %s/ is pinned and has an integrity hash\n", dir)
}

func check(dir string) ([]string, error) {
	var problems []string

	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(path), ".html") {
			return nil
		}

		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		for _, tag := range scriptRE.FindAllString(string(body), -1) {
			src := ""
			if m := srcRE.FindStringSubmatch(tag); m != nil {
				src = m[1]
			}
			if !external(src) {
				continue
			}

			if !pinnedRE.MatchString(src) {
				problems = append(problems, fmt.Sprintf(
					"%s: %s is not pinned to an exact version; a floating tag can change under you", path, src))
			}
			if !integrity.MatchString(tag) {
				problems = append(problems, fmt.Sprintf(
					"%s: %s has no integrity hash", path, src))
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Strings(problems)
	return problems, nil
}

// external reports whether a src points off this origin. Protocol relative URLs
// count: they inherit the scheme, not the host.
func external(src string) bool {
	return strings.HasPrefix(src, "http://") ||
		strings.HasPrefix(src, "https://") ||
		strings.HasPrefix(src, "//")
}
