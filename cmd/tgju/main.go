// Command tgju reads the tgju.org price boards from a terminal, and serves them
// over HTTP.
//
// Usage:
//
//	tgju serve [flags]         start the HTTP API
//	tgju get <market> [flags]  print one board
//	tgju item <key> [flags]    print one instrument
//	tgju watch <key> [flags]   follow one instrument until interrupted
//	tgju markets               list the supported boards
//	tgju version               print the version
//
// Run "tgju <command> -h" for the flags of a command.
//
// Every flag can also be given as an environment variable, upper cased and
// prefixed with TGJU_: --cache-ttl is TGJU_CACHE_TTL. The flag wins when both
// are set, which is what makes the same image usable from a compose file and
// from a shell.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
)

// exit codes, so a script can tell the failures apart.
const (
	exitOK       = 0
	exitUsage    = 2
	exitFailure  = 1
	exitUpstream = 3
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	os.Exit(run(ctx, os.Args[1:], os.Stdout, os.Stderr))
}

// run is main without the process, so the tests can drive it.
func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		usage(stderr)
		return exitUsage
	}

	var err error
	switch command := args[0]; command {
	case "serve":
		err = serve(ctx, args[1:], stderr)
	case "get":
		err = get(ctx, args[1:], stdout)
	case "item":
		err = item(ctx, args[1:], stdout)
	case "watch":
		err = watch(ctx, args[1:], stdout)
	case "markets":
		err = markets(args[1:], stdout)
	case "version":
		err = version(args[1:], stdout)
	case "help", "-h", "--help":
		usage(stdout)
		return exitOK
	default:
		fmt.Fprintf(stderr, "tgju: unknown command %q\n\n", command)
		usage(stderr)
		return exitUsage
	}

	switch {
	case err == nil, errors.Is(err, errHelp):
		return exitOK
	case errors.Is(err, context.Canceled):
		// Ctrl-C is how "watch" and "serve" are meant to end.
		return exitOK
	case errors.Is(err, errUsage):
		fmt.Fprintf(stderr, "tgju: %v\n", err)
		return exitUsage
	}

	fmt.Fprintf(stderr, "tgju: %v\n", err)
	if isUpstream(err) {
		return exitUpstream
	}
	return exitFailure
}

// errUsage marks a failure the user can fix by typing something else.
var errUsage = errors.New("usage")

func usageErrorf(format string, args ...any) error {
	return fmt.Errorf("%w: %s", errUsage, fmt.Sprintf(format, args...))
}

func usage(w io.Writer) {
	fmt.Fprint(w, `tgju — live currency, gold and coin prices from tgju.org

Usage:
  tgju serve [flags]           start the HTTP API
  tgju get <market> [flags]    print one board (currency, gold, coin)
  tgju item <key> [flags]      print one instrument, e.g. price_dollar_rl
  tgju watch <key> [flags]     follow one instrument until interrupted
  tgju markets                 list the supported boards
  tgju version                 print the version

Examples:
  tgju serve --addr :8080
  tgju get gold
  tgju get currency --format json --keys price_dollar_rl,price_eur
  tgju item geram18 --unit toman
  tgju watch price_dollar_rl --interval 30s

Run "tgju <command> -h" for the flags of a command.
`)
}
