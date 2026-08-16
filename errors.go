package tgju

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
)

// Sentinel errors returned by this package. Compare them with [errors.Is]; the
// detail of a particular failure is carried by [Error], which wraps one of
// them.
var (
	// ErrUnknownMarket is returned for a market name the library does not
	// serve.
	ErrUnknownMarket = errors.New("tgju: unknown market")
	// ErrRequest is returned when the request to tgju.org could not be made or
	// completed: DNS, TLS, a dropped connection, a cancelled context.
	ErrRequest = errors.New("tgju: request to tgju.org failed")
	// ErrUnexpectedStatus is returned when tgju.org answered with a status
	// other than 200. Read [Error.StatusCode] for the code itself.
	ErrUnexpectedStatus = errors.New("tgju: unexpected status from tgju.org")
	// ErrParse is returned when the page could be fetched but not understood,
	// which almost always means tgju changed its markup.
	ErrParse = errors.New("tgju: could not parse the tgju.org page")
	// ErrEmpty is returned when the page parsed cleanly but held no rows.
	ErrEmpty = errors.New("tgju: the page carried no prices")
	// ErrNotFound is returned by lookups for an instrument key that the board
	// does not publish.
	ErrNotFound = errors.New("tgju: no such instrument")
	// ErrTooLarge is returned when a response exceeds the configured body
	// limit, which protects a long lived service from a hostile or broken
	// upstream.
	ErrTooLarge = errors.New("tgju: response body is too large")
)

// Error is the rich error every fetch returns. It keeps the market, the URL and
// the upstream status so a caller can log them, while still unwrapping to one
// of the sentinels above.
type Error struct {
	// Op is the operation that failed: "fetch", "parse" or "lookup".
	Op string
	// Market is the board being read, when the failure is tied to one.
	Market Market
	// URL is the address that was requested.
	URL string
	// StatusCode is the HTTP status tgju answered with, or zero when the
	// request never produced a response.
	StatusCode int
	// Attempts is how many times the request was tried before giving up.
	Attempts int
	// Err is the wrapped sentinel or transport error.
	Err error
}

// Error implements the error interface.
func (e *Error) Error() string {
	msg := "tgju"
	if e.Market != "" {
		msg += ": " + string(e.Market)
	}
	if e.Op != "" {
		msg += ": " + e.Op
	}
	if e.StatusCode != 0 {
		msg += " (status " + strconv.Itoa(e.StatusCode)
		if text := http.StatusText(e.StatusCode); text != "" {
			msg += " " + text
		}
		msg += ")"
	}
	if e.Attempts > 1 {
		msg += fmt.Sprintf(" after %d attempts", e.Attempts)
	}
	if e.Err != nil {
		msg += ": " + e.Err.Error()
	}
	return msg
}

// Unwrap exposes the wrapped error to [errors.Is] and [errors.As].
func (e *Error) Unwrap() error { return e.Err }

// Temporary reports whether retrying the same call later could plausibly
// succeed. Parse failures and unknown markets are permanent; transport errors,
// rate limits and server errors are not.
func (e *Error) Temporary() bool {
	switch {
	case errors.Is(e.Err, ErrParse), errors.Is(e.Err, ErrUnknownMarket):
		return false
	case e.StatusCode == http.StatusTooManyRequests:
		return true
	case e.StatusCode >= 500:
		return true
	case e.StatusCode != 0:
		return false
	default:
		return true
	}
}
