package tgju_test

import (
	"errors"
	"slices"
	"testing"

	tgju "github.com/amiranmanesh/tgju-api-go"
)

func TestMarkets(t *testing.T) {
	t.Parallel()

	got := tgju.Markets()
	want := []tgju.Market{tgju.Coin, tgju.Currency, tgju.Gold}
	if !slices.Equal(got, want) {
		t.Fatalf("Markets() = %v, want %v", got, want)
	}

	// The order must be stable: callers iterate it to build deterministic
	// responses.
	if !slices.Equal(tgju.Markets(), got) {
		t.Error("Markets() is not stable between calls")
	}
}

func TestParseMarket(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   string
		want tgju.Market
	}{
		{"currency", tgju.Currency},
		{"CURRENCY", tgju.Currency},
		{"  fx  ", tgju.Currency},
		{"currencies", tgju.Currency},
		{"ارز", tgju.Currency},
		{"gold", tgju.Gold},
		{"gold-chart", tgju.Gold},
		{"silver", tgju.Gold},
		{"طلا", tgju.Gold},
		{"coin", tgju.Coin},
		{"coins", tgju.Coin},
		{"سکه", tgju.Coin},
	}

	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()
			got, err := tgju.ParseMarket(tc.in)
			if err != nil {
				t.Fatalf("ParseMarket(%q): %v", tc.in, err)
			}
			if got != tc.want {
				t.Errorf("ParseMarket(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}

	for _, in := range []string{"", "crypto", "stock", "oil"} {
		if _, err := tgju.ParseMarket(in); !errors.Is(err, tgju.ErrUnknownMarket) {
			t.Errorf("ParseMarket(%q) err = %v, want ErrUnknownMarket", in, err)
		}
	}
}

func TestMarketMetadata(t *testing.T) {
	t.Parallel()

	for _, m := range tgju.Markets() {
		if !m.Valid() {
			t.Errorf("%q is listed but not valid", m)
		}
		if m.String() != string(m) {
			t.Errorf("String() = %q, want %q", m.String(), m)
		}
		if m.Label() == "" {
			t.Errorf("%q has no label", m)
		}
		if m.Path() == "" || m.Path()[0] != '/' {
			t.Errorf("%q has path %q, want one starting with a slash", m, m.Path())
		}
		if want := tgju.DefaultBaseURL + m.Path(); m.URL() != want {
			t.Errorf("%q URL() = %q, want %q", m, m.URL(), want)
		}
	}

	unknown := tgju.Market("crypto")
	if unknown.Valid() {
		t.Error("crypto must not be a valid market")
	}
	if unknown.URL() != "" || unknown.Path() != "" || unknown.Label() != "" {
		t.Error("an unknown market must expose no metadata")
	}
}
