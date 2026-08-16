// Command checkspec sanity checks the OpenAPI document before it is published.
//
// It is not a validator. A full one needs a YAML parser and a JSON Schema
// implementation, and this project is not going to grow two dependencies to
// lint one file. What it does check is the mistake that actually happens when a
// spec is maintained by hand: a $ref pointing at a component that was renamed
// or never written, which silently produces an empty schema in every viewer.
//
//	go run ./internal/cmd/checkspec server/openapi.yaml
package main

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

var (
	// A component definition sits four spaces deep: two for "components", two
	// for the section, and the name is the next key.
	definitionRE = regexp.MustCompile(`^ {4}([A-Za-z][A-Za-z0-9_]*):\s*$`)
	sectionRE    = regexp.MustCompile(`^ {2}([a-z][A-Za-z]*):\s*$`)
	refRE        = regexp.MustCompile(`\$ref:\s*"#/components/([a-zA-Z]+)/([A-Za-z0-9_]+)"`)
	pathRE       = regexp.MustCompile(`^ {2}(/[^:]*):\s*$`)
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: checkspec <openapi.yaml>")
		os.Exit(2)
	}

	problems, err := check(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "checkspec:", err)
		os.Exit(1)
	}
	if len(problems) > 0 {
		for _, p := range problems {
			fmt.Fprintln(os.Stderr, "checkspec:", p)
		}
		os.Exit(1)
	}

	fmt.Println("checkspec: the document looks consistent")
}

func check(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	var (
		problems []string
		defined  = map[string]bool{} // "schemas/Item"
		used     = map[string]int{}  // "schemas/Item" -> first line it is used on
		paths    int
		version  string

		inComponents bool
		section      string
		line         int
	)

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line++
		text := scanner.Text()

		switch {
		case strings.HasPrefix(text, "openapi:"):
			version = strings.TrimSpace(strings.TrimPrefix(text, "openapi:"))
		case text == "components:":
			inComponents = true
			section = ""
		case len(text) > 0 && text[0] != ' ' && text[0] != '#':
			inComponents = false
		}

		if !inComponents {
			if m := pathRE.FindStringSubmatch(text); m != nil {
				paths++
			}
		} else {
			if m := sectionRE.FindStringSubmatch(text); m != nil {
				section = m[1]
			} else if m := definitionRE.FindStringSubmatch(text); m != nil && section != "" {
				defined[section+"/"+m[1]] = true
			}
		}

		for _, m := range refRE.FindAllStringSubmatch(text, -1) {
			key := m[1] + "/" + m[2]
			if _, seen := used[key]; !seen {
				used[key] = line
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	if !strings.HasPrefix(version, "3.") {
		problems = append(problems, fmt.Sprintf("openapi version is %q, want a 3.x document", version))
	}
	if paths == 0 {
		problems = append(problems, "the document describes no paths")
	}

	// Dangling references: the failure this tool exists for.
	for key, at := range used {
		if !defined[key] {
			problems = append(problems, fmt.Sprintf("line %d: $ref to #/components/%s, which is not defined", at, key))
		}
	}

	// Unused components are not an error, but they are almost always a rename
	// that was only half applied.
	for key := range defined {
		if _, ok := used[key]; !ok {
			problems = append(problems, fmt.Sprintf("component #/components/%s is defined but never referenced", key))
		}
	}

	sort.Strings(problems)
	return problems, nil
}
