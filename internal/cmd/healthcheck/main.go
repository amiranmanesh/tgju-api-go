// Command healthcheck probes a running tgju server and exits 0 when it is
// healthy.
//
// It exists because the runtime image is built from scratch: there is no shell
// and no curl for a HEALTHCHECK to call. A dozen lines of Go compiled next to
// the server keep the container honest without adding a distribution to the
// image.
//
// The address is read from TGJU_ADDR, the same variable the server listens on,
// so the two cannot drift apart.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

func main() {
	addr := os.Getenv("TGJU_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	if strings.HasPrefix(addr, ":") {
		addr = "127.0.0.1" + addr
	}

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+"/healthz", nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, "healthcheck:", err)
		os.Exit(1)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Fprintln(os.Stderr, "healthcheck:", err)
		os.Exit(1)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		fmt.Fprintln(os.Stderr, "healthcheck: status", resp.Status)
		os.Exit(1)
	}
}
