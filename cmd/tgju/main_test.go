package main

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	tgju "github.com/amiranmanesh/tgju-api-go"
	"github.com/amiranmanesh/tgju-api-go/internal/fixture"
)

// exec runs the CLI against the saved tgju pages and returns its output.
func exec(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()

	upstream := fixture.Server(t)
	t.Setenv("TGJU_BASE_URL", upstream.URL)

	var out, errOut bytes.Buffer
	code = run(t.Context(), args, &out, &errOut)

	return code, out.String(), errOut.String()
}

func TestNoArgumentsPrintsUsage(t *testing.T) {
	code, _, stderr := exec(t)

	if code != exitUsage {
		t.Errorf("exit code = %d, want %d", code, exitUsage)
	}
	if !strings.Contains(stderr, "Usage:") {
		t.Errorf("stderr does not show the usage:\n%s", stderr)
	}
}

func TestUnknownCommand(t *testing.T) {
	code, _, stderr := exec(t, "fetch")

	if code != exitUsage {
		t.Errorf("exit code = %d, want %d", code, exitUsage)
	}
	if !strings.Contains(stderr, `unknown command "fetch"`) {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestHelp(t *testing.T) {
	code, stdout, _ := exec(t, "help")

	if code != exitOK {
		t.Errorf("exit code = %d, want 0", code)
	}
	for _, command := range []string{"serve", "get", "item", "watch", "markets", "version"} {
		if !strings.Contains(stdout, command) {
			t.Errorf("the usage does not mention %q", command)
		}
	}
}

func TestMarketsCommand(t *testing.T) {
	code, stdout, stderr := exec(t, "markets")

	if code != exitOK {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr)
	}
	for _, m := range tgju.Markets() {
		if !strings.Contains(stdout, string(m)) {
			t.Errorf("output does not list %q:\n%s", m, stdout)
		}
	}
}

func TestVersionCommand(t *testing.T) {
	code, stdout, stderr := exec(t, "version")

	if code != exitOK {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr)
	}
	if !strings.Contains(stdout, tgju.Version) {
		t.Errorf("output does not carry the version:\n%s", stdout)
	}
	if !strings.Contains(stdout, "platform") {
		t.Errorf("output does not carry the platform:\n%s", stdout)
	}
}

func TestGetTable(t *testing.T) {
	code, stdout, stderr := exec(t, "get", "gold")

	if code != exitOK {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr)
	}
	for _, want := range []string{"geram18", "طلای 18 عیار / 750", "قیمت نقره", "instruments"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("output does not contain %q:\n%s", want, stdout)
		}
	}
}

func TestGetJSON(t *testing.T) {
	code, stdout, stderr := exec(t, "get", "currency", "--format", "json")

	if code != exitOK {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr)
	}

	var snap tgju.Snapshot
	if err := json.Unmarshal([]byte(stdout), &snap); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, stdout)
	}
	if snap.Market != tgju.Currency || snap.IsEmpty() {
		t.Errorf("snapshot = %+v", snap)
	}
}

func TestGetCSV(t *testing.T) {
	code, stdout, stderr := exec(t, "get", "coin", "--format", "csv")

	if code != exitOK {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr)
	}

	records, err := csv.NewReader(strings.NewReader(stdout)).ReadAll()
	if err != nil {
		t.Fatalf("output is not valid CSV: %v", err)
	}
	if len(records) < 2 {
		t.Fatalf("got %d CSV rows, want a header and some data", len(records))
	}
	if records[0][0] != "market" || records[0][3] != "title" {
		t.Errorf("unexpected header: %v", records[0])
	}
}

func TestGetFilters(t *testing.T) {
	code, stdout, stderr := exec(t, "get", "currency", "--keys", "price_dollar_rl,price_eur", "--format", "csv")

	if code != exitOK {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr)
	}

	records, err := csv.NewReader(strings.NewReader(stdout)).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 3 { // header plus two rows
		t.Fatalf("got %d rows, want 3: %v", len(records), records)
	}
}

func TestGetRejectsFiltersThatMatchNothing(t *testing.T) {
	code, _, stderr := exec(t, "get", "gold", "--keys", "price_eur")

	if code != exitFailure {
		t.Errorf("exit code = %d, want %d", code, exitFailure)
	}
	if !strings.Contains(stderr, "matched the filters") {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestGetRejectsUnknownMarkets(t *testing.T) {
	code, _, stderr := exec(t, "get", "crypto")

	if code != exitFailure {
		t.Errorf("exit code = %d, want %d", code, exitFailure)
	}
	if !strings.Contains(stderr, "unknown market") {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestGetRejectsBadFlags(t *testing.T) {
	tests := [][]string{
		{"get"},                                   // no market
		{"get", "gold", "currency"},               // two markets
		{"get", "gold", "--format", "yaml"},       // unknown format
		{"get", "gold", "--unit", "dollars"},      // unknown unit
		{"item"},                                  // no key
		{"watch", "geram18", "--interval", "1ms"}, // too fast
	}

	for _, args := range tests {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			code, _, stderr := exec(t, args...)
			if code != exitUsage {
				t.Errorf("exit code = %d, want %d (stderr: %s)", code, exitUsage, stderr)
			}
		})
	}
}

func TestItemCommand(t *testing.T) {
	code, stdout, stderr := exec(t, "item", "geram18")

	if code != exitOK {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr)
	}
	for _, want := range []string{"geram18", "gold", "price"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("output does not contain %q:\n%s", want, stdout)
		}
	}
}

func TestItemRespectsTheMarketFlag(t *testing.T) {
	code, _, stderr := exec(t, "item", "geram18", "--market", "currency")

	if code != exitFailure {
		t.Errorf("exit code = %d, want %d", code, exitFailure)
	}
	if !strings.Contains(stderr, "no such instrument") {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestItemUnitConversion(t *testing.T) {
	// JSON always carries the domain model, so the unit flag must not touch it.
	var rial, toman tgju.Item
	_, rialJSON, _ := exec(t, "item", "price_dollar_rl", "--unit", "rial", "--format", "json")
	_, tomanJSON, _ := exec(t, "item", "price_dollar_rl", "--unit", "toman", "--format", "json")

	if err := json.Unmarshal([]byte(rialJSON), &rial); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(tomanJSON), &toman); err != nil {
		t.Fatal(err)
	}
	// Each exec spins up its own fixture server, so the absolute profile URL
	// legitimately differs between the two runs.
	rial.ProfileURL, toman.ProfileURL = "", ""
	if rial != toman {
		t.Errorf("--unit changed the JSON output:\n%+v\n%+v", rial, toman)
	}
	if rial.Price.Value != 1_864_000 {
		t.Errorf("JSON price = %v, want the rial value from the page", rial.Price.Value)
	}

	_, table, _ := exec(t, "item", "price_dollar_rl", "--unit", "toman")
	if !strings.Contains(table, "186,400") {
		t.Errorf("toman rendering is missing from:\n%s", table)
	}

	_, rialTable, _ := exec(t, "item", "price_dollar_rl", "--unit", "rial")
	if !strings.Contains(rialTable, "1,864,000") {
		t.Errorf("rial rendering is missing from:\n%s", rialTable)
	}
}

// TestUpstreamFailureExitCode proves a script can tell "tgju.org is down" from
// "you typed something wrong".
func TestUpstreamFailureExitCode(t *testing.T) {
	upstream := fixture.ServerFunc(t, func(w http.ResponseWriter, _ *http.Request) bool {
		w.WriteHeader(http.StatusBadGateway)
		return false
	})
	t.Setenv("TGJU_BASE_URL", upstream.URL)

	var out, errOut bytes.Buffer
	code := run(t.Context(), []string{"get", "gold", "--retries", "1"}, &out, &errOut)

	if code != exitUpstream {
		t.Errorf("exit code = %d, want %d", code, exitUpstream)
	}
	if !strings.Contains(errOut.String(), "unexpected status") {
		t.Errorf("stderr = %q", errOut.String())
	}
}

func TestWatchStopsWithTheContext(t *testing.T) {
	upstream := fixture.Server(t)
	t.Setenv("TGJU_BASE_URL", upstream.URL)

	// Cancelling models Ctrl-C, which is the only way "watch" is meant to end.
	ctx, cancel := context.WithCancel(t.Context())
	go func() {
		time.Sleep(300 * time.Millisecond)
		cancel()
	}()
	defer cancel()

	var out, errOut bytes.Buffer
	code := run(ctx, []string{"watch", "price_dollar_rl", "--interval", "1s"}, &out, &errOut)

	if code != exitOK {
		t.Errorf("exit code = %d, want 0 on interrupt (stderr: %s)", code, errOut.String())
	}
	if !strings.Contains(out.String(), "دلار") {
		t.Errorf("watch printed nothing useful:\n%s", out.String())
	}
}

func TestEnvironmentFallback(t *testing.T) {
	upstream := fixture.Server(t)
	t.Setenv("TGJU_BASE_URL", upstream.URL)
	t.Setenv("TGJU_FORMAT", "json")

	var out, errOut bytes.Buffer
	if code := run(t.Context(), []string{"get", "gold"}, &out, &errOut); code != exitOK {
		t.Fatalf("exit code = %d, stderr = %s", code, errOut.String())
	}

	var snap tgju.Snapshot
	if err := json.Unmarshal(out.Bytes(), &snap); err != nil {
		t.Fatalf("TGJU_FORMAT was ignored; output is not JSON: %v", err)
	}

	// An explicit flag must still win over the environment.
	out.Reset()
	if code := run(t.Context(), []string{"get", "gold", "--format", "table"}, &out, &errOut); code != exitOK {
		t.Fatalf("exit code = %d", code)
	}
	if json.Valid(out.Bytes()) {
		t.Error("the --format flag did not override TGJU_FORMAT")
	}
}

func TestGroup(t *testing.T) {
	tests := map[float64]string{
		0:         "0",
		12:        "12",
		1234:      "1,234",
		1864000:   "1,864,000",
		-1234:     "-1,234",
		1234.5:    "1,234.5",
		123:       "123",
		1000000.5: "1,000,000.5",
	}
	for in, want := range tests {
		if got := group(in); got != want {
			t.Errorf("group(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestSplitCSV(t *testing.T) {
	tests := []struct {
		in   string
		want int
	}{
		{"", 0},
		{"a", 1},
		{"a,b", 2},
		{" a , b ,", 2},
		{",,,", 0},
	}
	for _, tc := range tests {
		if got := splitCSV(tc.in); len(got) != tc.want {
			t.Errorf("splitCSV(%q) = %v, want %d entries", tc.in, got, tc.want)
		}
	}
}
