// Package numfmt normalises the numbers tgju.org renders for a Persian
// audience into values a Go program can compute with.
//
// The site mixes ASCII digits ("1,864,000"), Persian digits ("۱۱:۴۹:۴۵"),
// thousands separators, the Arabic decimal separator and the zero width
// non-joiner. Every exported function here is pure and allocation friendly, so
// the HTML scraper can call them once per cell without becoming the bottleneck.
package numfmt

import (
	"strconv"
	"strings"
	"unicode"
)

// zeroWidth lists the invisible runes tgju sprinkles into titles and numbers:
// the zero width space, the non-joiner and joiner, both directional marks and
// the byte order mark. They carry no meaning for us yet break both number
// parsing and map lookups, so they are dropped on sight.
const zeroWidth = "\u200B\u200C\u200D\u200E\u200F\uFEFF"

// Digits rewrites Persian (U+06F0..U+06F9) and Arabic-Indic (U+0660..U+0669)
// digits as ASCII and drops zero width runes. Every other rune is preserved, so
// "۱۱:۴۹:۴۵" becomes "11:49:45" and "۲۴ مرداد" becomes "24 مرداد".
func Digits(s string) string {
	if s == "" {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r >= '۰' && r <= '۹': // U+06F0..U+06F9, Persian
			b.WriteRune('0' + (r - '۰'))
		case r >= '٠' && r <= '٩': // U+0660..U+0669, Arabic-Indic
			b.WriteRune('0' + (r - '٠'))
		case r == '٫': // Arabic decimal separator
			b.WriteRune('.')
		case strings.ContainsRune(zeroWidth, r):
			// drop
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Clean trims a cell of a market table: digits are normalised, zero width runes
// removed and the surrounding whitespace collapsed to single spaces.
func Clean(s string) string {
	return strings.Join(strings.FieldsFunc(Digits(s), unicode.IsSpace), " ")
}

// Value parses a rendered amount such as "1,864,000" or "۳٬۷۶۵٬۹۰۰" into a
// float64. It reports false when the input carries no digits at all, which is
// how an empty or dash-only table cell arrives.
//
// Grouping separators (",", "٬", "'" and spaces) are ignored wherever they
// appear; a leading "-" or "+" is honoured.
func Value(s string) (float64, bool) {
	s = Digits(s)

	var b strings.Builder
	b.Grow(len(s))
	var digits, dot bool
	for i, r := range s {
		switch {
		case r >= '0' && r <= '9':
			digits = true
			b.WriteRune(r)
		case r == '.' && !dot:
			dot = true
			b.WriteRune(r)
		case (r == '-' || r == '+') && i == 0:
			b.WriteRune(r)
		default:
			// separators, currency marks and stray text are dropped
		}
	}
	if !digits {
		return 0, false
	}
	v, err := strconv.ParseFloat(strings.TrimSuffix(b.String(), "."), 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// Change splits the "change" cell of a market table, rendered as
// "(0.32%) 6,050", into its percentage and its absolute part. The absolute part
// is returned as text so the caller can keep the site's own formatting next to
// the parsed number.
//
// A cell without parentheses is treated as an absolute change with no
// percentage, and "(0%) 0" yields two zeroes.
func Change(s string) (percent float64, amount string) {
	s = Clean(s)
	if s == "" {
		return 0, ""
	}
	if open := strings.IndexByte(s, '('); open >= 0 {
		if shut := strings.IndexByte(s[open:], ')'); shut >= 0 {
			percent, _ = Value(s[open+1 : open+shut])
			amount = strings.TrimSpace(s[open+shut+1:])
			return percent, amount
		}
	}
	return 0, s
}
