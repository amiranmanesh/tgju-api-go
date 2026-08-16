package tgju_test

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	tgju "github.com/amiranmanesh/tgju-api-go"
)

func TestErrorMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		err   *tgju.Error
		parts []string
	}{
		{
			name:  "full",
			err:   &tgju.Error{Op: "fetch", Market: tgju.Gold, StatusCode: 503, Attempts: 3, Err: tgju.ErrUnexpectedStatus},
			parts: []string{"gold", "fetch", "status 503", "Service Unavailable", "after 3 attempts", "unexpected status"},
		},
		{
			name:  "parse failure",
			err:   &tgju.Error{Op: "parse", Market: tgju.Currency, Err: tgju.ErrParse},
			parts: []string{"currency", "parse", "could not parse"},
		},
		{
			name:  "lookup without a market",
			err:   &tgju.Error{Op: "lookup", Err: tgju.ErrNotFound},
			parts: []string{"lookup", "no such instrument"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			msg := tc.err.Error()
			for _, part := range tc.parts {
				if !strings.Contains(msg, part) {
					t.Errorf("message %q does not mention %q", msg, part)
				}
			}
		})
	}
}

func TestErrorUnwrap(t *testing.T) {
	t.Parallel()

	err := error(&tgju.Error{Op: "fetch", Market: tgju.Coin, Err: tgju.ErrRequest})
	if !errors.Is(err, tgju.ErrRequest) {
		t.Error("errors.Is did not see through the wrapper")
	}

	var target *tgju.Error
	if !errors.As(err, &target) || target.Market != tgju.Coin {
		t.Error("errors.As did not recover the typed error")
	}
}

func TestErrorTemporary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  *tgju.Error
		want bool
	}{
		{"transport failure", &tgju.Error{Err: tgju.ErrRequest}, true},
		{"rate limited", &tgju.Error{StatusCode: http.StatusTooManyRequests, Err: tgju.ErrUnexpectedStatus}, true},
		{"server error", &tgju.Error{StatusCode: http.StatusBadGateway, Err: tgju.ErrUnexpectedStatus}, true},
		{"forbidden", &tgju.Error{StatusCode: http.StatusForbidden, Err: tgju.ErrUnexpectedStatus}, false},
		{"not found", &tgju.Error{StatusCode: http.StatusNotFound, Err: tgju.ErrUnexpectedStatus}, false},
		{"parse failure", &tgju.Error{Err: tgju.ErrParse}, false},
		{"unknown market", &tgju.Error{Err: tgju.ErrUnknownMarket}, false},
		{"body too large", &tgju.Error{StatusCode: http.StatusOK, Err: tgju.ErrTooLarge}, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.err.Temporary(); got != tc.want {
				t.Errorf("Temporary() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestSentinelsAreDistinct guards against a copy paste that would make two
// sentinels the same value and silently break errors.Is at a call site.
func TestSentinelsAreDistinct(t *testing.T) {
	t.Parallel()

	sentinels := []error{
		tgju.ErrUnknownMarket, tgju.ErrRequest, tgju.ErrUnexpectedStatus,
		tgju.ErrParse, tgju.ErrEmpty, tgju.ErrNotFound, tgju.ErrTooLarge,
	}
	for i, a := range sentinels {
		for j, b := range sentinels {
			if i != j && errors.Is(a, b) {
				t.Errorf("sentinel %v is indistinguishable from %v", a, b)
			}
		}
	}
}
