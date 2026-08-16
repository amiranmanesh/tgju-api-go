package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	tgju "github.com/amiranmanesh/tgju-api-go"
)

// format is how a command renders its result.
type format string

const (
	// formatTable is the default: aligned columns for a human.
	formatTable format = "table"
	// formatJSON is the full domain model, for a pipe into jq.
	formatJSON format = "json"
	// formatCSV is one row per instrument, for a spreadsheet.
	formatCSV format = "csv"
)

func parseFormat(s string) (format, error) {
	switch f := format(strings.ToLower(strings.TrimSpace(s))); f {
	case formatTable, formatJSON, formatCSV:
		return f, nil
	default:
		return "", usageErrorf("unknown --format %q; want table, json or csv", s)
	}
}

// unit selects how prices are printed. tgju quotes in rial; most Iranians think
// in toman, which is ten rials.
type unit string

const (
	unitRial  unit = "rial"
	unitToman unit = "toman"
	// unitRaw keeps tgju's own rendering, separators and all.
	unitRaw unit = "raw"
)

func parseUnit(s string) (unit, error) {
	switch u := unit(strings.ToLower(strings.TrimSpace(s))); u {
	case unitRial, unitToman, unitRaw:
		return u, nil
	default:
		return "", usageErrorf("unknown --unit %q; want rial, toman or raw", s)
	}
}

// render prints an amount according to the selected unit.
func (u unit) render(a tgju.Amount) string {
	switch {
	case a.IsZero():
		return "—"
	case u == unitRaw:
		return a.Text
	case u == unitToman:
		return group(a.Toman())
	default:
		return group(a.Value)
	}
}

// group formats a number with thousands separators, dropping a zero fraction.
func group(v float64) string {
	s := strconv.FormatFloat(v, 'f', -1, 64)

	whole, frac, hasFrac := strings.Cut(s, ".")
	neg := strings.HasPrefix(whole, "-")
	whole = strings.TrimPrefix(whole, "-")

	var b strings.Builder
	for i, digit := range whole {
		if i > 0 && (len(whole)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(digit)
	}

	out := b.String()
	if neg {
		out = "-" + out
	}
	if hasFrac {
		out += "." + frac
	}
	return out
}

// arrow is the one glyph that survives a screenshot: which way the price moved.
func arrow(status tgju.Status) string {
	switch status {
	case tgju.StatusHigh:
		return "▲"
	case tgju.StatusLow:
		return "▼"
	default:
		return " "
	}
}

// writeSnapshot renders a whole board.
func writeSnapshot(w io.Writer, snap tgju.Snapshot, f format, u unit) error {
	switch f {
	case formatJSON:
		return writeJSON(w, snap)
	case formatCSV:
		return writeCSV(w, snap.Items(), u)
	default:
		return writeTable(w, snap, u)
	}
}

// writeItem renders a single instrument.
func writeItem(w io.Writer, item tgju.Item, f format, u unit) error {
	switch f {
	case formatJSON:
		return writeJSON(w, item)
	case formatCSV:
		return writeCSV(w, []tgju.Item{item}, u)
	default:
		return writeItemLines(w, item, u)
	}
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

func writeCSV(w io.Writer, items []tgju.Item, u unit) error {
	out := csv.NewWriter(w)
	defer out.Flush()

	header := []string{"market", "category", "key", "title", "price", "low", "high", "status", "change_percent", "change", "time"}
	if err := out.Write(header); err != nil {
		return err
	}

	for _, item := range items {
		record := []string{
			string(item.Market),
			item.Category,
			item.Key,
			item.Title,
			u.render(item.Price),
			u.render(item.Low),
			u.render(item.High),
			string(item.Change.Status),
			strconv.FormatFloat(item.Change.Percent, 'f', -1, 64),
			u.render(item.Change.Amount),
			item.Time,
		}
		if err := out.Write(record); err != nil {
			return err
		}
	}

	out.Flush()
	return out.Error()
}

func writeTable(w io.Writer, snap tgju.Snapshot, u unit) error {
	for i, category := range snap.Categories {
		if i > 0 {
			fmt.Fprintln(w)
		}
		if category.Title != "" {
			fmt.Fprintf(w, "%s\n", category.Title)
		}

		rows := make([][]string, 0, len(category.Items)+1)
		rows = append(rows, []string{"KEY", "TITLE", "PRICE", "LOW", "HIGH", "CHANGE", "TIME"})
		for _, item := range category.Items {
			rows = append(rows, []string{
				item.Key,
				item.Title,
				u.render(item.Price),
				u.render(item.Low),
				u.render(item.High),
				fmt.Sprintf("%s %s%%", arrow(item.Change.Status), strconv.FormatFloat(item.Change.Percent, 'f', -1, 64)),
				item.Time,
			})
		}
		if err := writeAligned(w, rows); err != nil {
			return err
		}
	}

	fmt.Fprintf(w, "\n%d instruments · %s · fetched %s\n",
		snap.Len(), snap.Source, snap.FetchedAt.Format("2006-01-02 15:04:05 MST"))
	return nil
}

func writeItemLines(w io.Writer, item tgju.Item, u unit) error {
	rows := [][]string{
		{"key", item.Key},
		{"title", item.Title},
		{"market", string(item.Market)},
		{"category", item.Category},
		{"price", u.render(item.Price)},
		{"low", u.render(item.Low)},
		{"high", u.render(item.High)},
		{"change", fmt.Sprintf("%s %s%% (%s)", arrow(item.Change.Status),
			strconv.FormatFloat(item.Change.Percent, 'f', -1, 64), u.render(item.Change.Amount))},
		{"time", item.Time},
	}
	if item.ProfileURL != "" {
		rows = append(rows, []string{"profile", item.ProfileURL})
	}
	return writeAligned(w, rows)
}

// writeAligned prints rows in left aligned columns.
//
// Widths are counted in runes rather than bytes, which is the difference
// between a readable table and a ragged one as soon as a title is Persian.
func writeAligned(w io.Writer, rows [][]string) error {
	if len(rows) == 0 {
		return nil
	}

	widths := make([]int, len(rows[0]))
	for _, row := range rows {
		for i, cell := range row {
			if i < len(widths) {
				widths[i] = max(widths[i], utf8.RuneCountInString(cell))
			}
		}
	}

	var b strings.Builder
	for _, row := range rows {
		for i, cell := range row {
			b.WriteString(cell)
			if i < len(row)-1 {
				b.WriteString(strings.Repeat(" ", widths[i]-utf8.RuneCountInString(cell)+2))
			}
		}
		b.WriteByte('\n')
	}

	_, err := io.WriteString(w, b.String())
	return err
}
