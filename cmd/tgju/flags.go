package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	tgju "github.com/amiranmanesh/tgju-api-go"
)

// clientFlags are the settings every command shares, because every command ends
// up building a [tgju.Client].
type clientFlags struct {
	baseURL  string
	timeout  time.Duration
	cacheTTL time.Duration
	retries  int
	logLevel string
	logJSON  bool
}

func (f *clientFlags) register(fs *flag.FlagSet) {
	fs.StringVar(&f.baseURL, "base-url", envString("BASE_URL", tgju.DefaultBaseURL),
		"site to read from; override for a mirror or a test server")
	fs.DurationVar(&f.timeout, "timeout", envDuration("TIMEOUT", tgju.DefaultTimeout),
		"deadline for one page fetch, retries included")
	fs.DurationVar(&f.cacheTTL, "cache-ttl", envDuration("CACHE_TTL", tgju.DefaultCacheTTL),
		"how long a snapshot is reused; 0 disables the cache")
	fs.IntVar(&f.retries, "retries", envInt("RETRIES", tgju.DefaultRetry.MaxAttempts),
		"total attempts per fetch, including the first")
	fs.StringVar(&f.logLevel, "log-level", envString("LOG_LEVEL", "info"),
		"debug, info, warn or error")
	fs.BoolVar(&f.logJSON, "log-json", envBool("LOG_JSON", false),
		"emit logs as JSON instead of text")
}

func (f *clientFlags) client(extra ...tgju.Option) *tgju.Client {
	opts := []tgju.Option{
		tgju.WithBaseURL(f.baseURL),
		tgju.WithTimeout(f.timeout),
		tgju.WithCacheTTL(f.cacheTTL),
		tgju.WithRetry(tgju.RetryPolicy{
			MaxAttempts: f.retries,
			Backoff:     tgju.DefaultRetry.Backoff,
			MaxBackoff:  tgju.DefaultRetry.MaxBackoff,
		}),
	}
	return tgju.New(append(opts, extra...)...)
}

// logger builds the structured logger the command writes to. Logs go to stderr
// so that piping stdout into jq keeps working.
func (f *clientFlags) logger(w io.Writer) *slog.Logger {
	opts := &slog.HandlerOptions{Level: parseLevel(f.logLevel)}
	if f.logJSON {
		return slog.New(slog.NewJSONHandler(w, opts))
	}
	return slog.New(slog.NewTextHandler(w, opts))
}

func parseLevel(s string) slog.Level {
	var level slog.Level
	if err := level.UnmarshalText([]byte(s)); err != nil {
		return slog.LevelInfo
	}
	return level
}

// newFlagSet returns a flag set that prints its usage to the given writer and
// reports errors instead of exiting, so run() stays in charge of the exit code.
func newFlagSet(name string, out io.Writer, summary string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(out)
	fs.Usage = func() {
		fmt.Fprintf(out, "%s\n\nUsage: tgju %s\n\nFlags:\n", summary, name)
		fs.PrintDefaults()
		fmt.Fprintf(out, "\nEvery flag can also be set as TGJU_<FLAG>, e.g. TGJU_CACHE_TTL=1m.\n")
	}
	return fs
}

// parse runs the flag set and returns the positional arguments.
//
// The standard flag package stops at the first non-flag argument, which would
// make "tgju get gold --format json" silently ignore the format. Parsing in a
// loop, taking one positional at a time, lets flags and arguments be written in
// whichever order reads best.
func parse(fs *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	for {
		if err := fs.Parse(args); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return nil, errHelp
			}
			return nil, fmt.Errorf("%w: %w", errUsage, err)
		}
		rest := fs.Args()
		if len(rest) == 0 {
			return positional, nil
		}
		positional = append(positional, rest[0])
		args = rest[1:]
	}
}

// errHelp reports that the user asked for the usage text, which is a success.
var errHelp = errors.New("help requested")

// isUpstream reports whether a failure came from tgju.org rather than from the
// user or this program, so main can pick a distinct exit code for it.
func isUpstream(err error) bool {
	var tgjuErr *tgju.Error
	return errors.As(err, &tgjuErr) && tgjuErr.Op != "lookup"
}

// The environment fallbacks. Every flag reads TGJU_<NAME>, which is how the
// container image is configured.

func envKey(name string) string {
	return "TGJU_" + strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
}

func envString(name, fallback string) string {
	if v, ok := os.LookupEnv(envKey(name)); ok && v != "" {
		return v
	}
	return fallback
}

func envDuration(name string, fallback time.Duration) time.Duration {
	if v, ok := os.LookupEnv(envKey(name)); ok && v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}

func envInt(name string, fallback int) int {
	if v, ok := os.LookupEnv(envKey(name)); ok && v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func envBool(name string, fallback bool) bool {
	if v, ok := os.LookupEnv(envKey(name)); ok && v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return fallback
}

func envFloat(name string, fallback float64) float64 {
	if v, ok := os.LookupEnv(envKey(name)); ok && v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return fallback
}
