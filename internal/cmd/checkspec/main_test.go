package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// write drops a document into a temporary file and returns its path.
func write(t *testing.T, body string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "openapi.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

const good = `openapi: 3.1.0
info:
  title: Example
paths:
  /things:
    get:
      responses:
        "200":
          content:
            application/json:
              schema: { $ref: "#/components/schemas/Thing" }
        "404": { $ref: "#/components/responses/NotFound" }
components:
  responses:
    NotFound:
      description: no
  schemas:
    Thing:
      type: object
`

func TestAcceptsAConsistentDocument(t *testing.T) {
	t.Parallel()

	problems, err := check(write(t, good))
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if len(problems) != 0 {
		t.Errorf("reported problems in a healthy document: %v", problems)
	}
}

func TestRejectsADanglingReference(t *testing.T) {
	t.Parallel()

	body := strings.Replace(good, `#/components/schemas/Thing`, `#/components/schemas/Widget`, 1)

	problems, err := check(write(t, body))
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if len(problems) == 0 {
		t.Fatal("a dangling $ref was not reported")
	}
	if !strings.Contains(strings.Join(problems, "\n"), "schemas/Widget") {
		t.Errorf("problems do not name the missing component: %v", problems)
	}
}

func TestRejectsAnOrphanComponent(t *testing.T) {
	t.Parallel()

	body := good + `    Orphan:
      type: string
`

	problems, err := check(write(t, body))
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if !strings.Contains(strings.Join(problems, "\n"), "schemas/Orphan") {
		t.Errorf("an unreferenced component was not reported: %v", problems)
	}
}

func TestRejectsADocumentWithoutPaths(t *testing.T) {
	t.Parallel()

	const body = `openapi: 3.1.0
info:
  title: Empty
paths:
`

	problems, err := check(write(t, body))
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if !strings.Contains(strings.Join(problems, "\n"), "no paths") {
		t.Errorf("an empty document was accepted: %v", problems)
	}
}

func TestRejectsTheWrongVersion(t *testing.T) {
	t.Parallel()

	body := strings.Replace(good, "openapi: 3.1.0", "openapi: 2.0", 1)

	problems, err := check(write(t, body))
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if !strings.Contains(strings.Join(problems, "\n"), "want a 3.x document") {
		t.Errorf("a Swagger 2.0 document was accepted: %v", problems)
	}
}

func TestReportsAMissingFile(t *testing.T) {
	t.Parallel()

	if _, err := check(filepath.Join(t.TempDir(), "absent.yaml")); err == nil {
		t.Fatal("a missing file was not reported")
	}
}

// TestTheRealDocument is the check that keeps this tool honest: it must accept
// the document this repository actually ships.
func TestTheRealDocument(t *testing.T) {
	t.Parallel()

	problems, err := check(filepath.Join("..", "..", "..", "server", "openapi.yaml"))
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if len(problems) != 0 {
		t.Errorf("server/openapi.yaml has problems:\n%s", strings.Join(problems, "\n"))
	}
}
