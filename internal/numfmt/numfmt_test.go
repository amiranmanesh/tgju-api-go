package numfmt_test

import (
	"math"
	"testing"

	"github.com/amiranmanesh/tgju-api-go/internal/numfmt"
)

func TestDigits(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"ascii is untouched", "1,864,000", "1,864,000"},
		{"persian clock", "۱۱:۴۹:۴۵", "11:49:45"},
		{"persian date keeps its month", "۲۴ مرداد", "24 مرداد"},
		{"arabic indic digits", "٣٧٦٥٩٠٠", "3765900"},
		{"arabic decimal separator", "۳٫۵", "3.5"},
		{"zero width non joiner is dropped", "سایر سکه\u200cها", "سایر سکهها"},
		{"byte order mark is dropped", "\ufeff42", "42"},
		{"mixed scripts", "قیمت ۱۸ عیار", "قیمت 18 عیار"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := numfmt.Digits(tc.in); got != tc.want {
				t.Errorf("Digits(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestClean(t *testing.T) {
	t.Parallel()

	tests := []struct{ in, want string }{
		{"  دلار  ", "دلار"},
		{"\n\t (0.32%)   6,050 \n", "(0.32%) 6,050"},
		{"", ""},
		{"   ", ""},
		{"۱۱:۴۹:۴۵", "11:49:45"},
	}

	for _, tc := range tests {
		if got := numfmt.Clean(tc.in); got != tc.want {
			t.Errorf("Clean(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want float64
		ok   bool
	}{
		{"grouped", "1,864,000", 1_864_000, true},
		{"persian grouped", "۱٬۸۶۴٬۰۰۰", 1_864_000, true},
		{"decimal", "0.32", 0.32, true},
		{"persian decimal", "۳٫۵", 3.5, true},
		{"percent sign is ignored", "0.32%", 0.32, true},
		{"negative", "-500", -500, true},
		{"spaces inside", "1 864 000", 1_864_000, true},
		{"zero", "0", 0, true},
		{"empty", "", 0, false},
		{"dash only", "-", 0, false},
		{"letters only", "ندارد", 0, false},
		{"trailing dot", "12.", 12, true},
		{"second dot is dropped", "1.2.3", 1.23, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := numfmt.Value(tc.in)
			if ok != tc.ok {
				t.Fatalf("Value(%q) ok = %v, want %v", tc.in, ok, tc.ok)
			}
			if ok && math.Abs(got-tc.want) > 1e-9 {
				t.Errorf("Value(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestChange(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		in          string
		wantPercent float64
		wantAmount  string
	}{
		{"typical", "(0.32%) 6,050", 0.32, "6,050"},
		{"no move", "(0%) 0", 0, "0"},
		{"persian digits", "(۰٫۱۳%) ۲۵۲,۰۰۰", 0.13, "252,000"},
		{"double digit percent", "(18.93%) 5,770,000", 18.93, "5,770,000"},
		{"no parentheses", "6,050", 0, "6,050"},
		{"empty", "", 0, ""},
		{"unterminated parenthesis", "(0.5% 100", 0, "(0.5% 100"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			percent, amount := numfmt.Change(tc.in)
			if math.Abs(percent-tc.wantPercent) > 1e-9 {
				t.Errorf("Change(%q) percent = %v, want %v", tc.in, percent, tc.wantPercent)
			}
			if amount != tc.wantAmount {
				t.Errorf("Change(%q) amount = %q, want %q", tc.in, amount, tc.wantAmount)
			}
		})
	}
}

// FuzzValue asserts the invariant that matters for a scraper: whatever tgju
// puts in a cell, parsing it must not panic, and a reported success must be a
// finite number.
func FuzzValue(f *testing.F) {
	for _, seed := range []string{"1,864,000", "۱٬۸۶۴٬۰۰۰", "(0.32%) 6,050", "", "-", "1.2.3.4", "--5", "+.", "۰"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, in string) {
		v, ok := numfmt.Value(in)
		if ok && (math.IsNaN(v) || math.IsInf(v, 0)) {
			t.Fatalf("Value(%q) reported success with a non finite value %v", in, v)
		}
	})
}

// FuzzChange checks that splitting a change cell never panics and never invents
// a percentage out of an input without digits.
func FuzzChange(f *testing.F) {
	for _, seed := range []string{"(0.32%) 6,050", "()", "(", ")", "(%)", "((1%)) 2"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, in string) {
		percent, _ := numfmt.Change(in)
		if math.IsNaN(percent) || math.IsInf(percent, 0) {
			t.Fatalf("Change(%q) produced a non finite percentage %v", in, percent)
		}
	})
}

func BenchmarkValue(b *testing.B) {
	for b.Loop() {
		numfmt.Value("1,864,000")
	}
}

func BenchmarkDigitsPersian(b *testing.B) {
	for b.Loop() {
		numfmt.Digits("۱۱:۴۹:۴۵")
	}
}
