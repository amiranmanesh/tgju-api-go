package server

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	tgju "github.com/amiranmanesh/tgju-api-go"
)

// APIError is the body of every failed response. One shape for every failure
// means a client can branch on Code without parsing prose.
type APIError struct {
	// Code is a stable, machine readable reason: "not_found",
	// "unknown_market", "upstream_unavailable", "upstream_changed",
	// "rate_limited", "timeout" or "internal".
	Code string `json:"code"`
	// Message is a human readable explanation, in English.
	Message string `json:"message"`
	// RequestID echoes the X-Request-Id of the request, so a report from a
	// user can be found in the logs.
	RequestID string `json:"request_id,omitempty"`
}

// Error implements the error interface so an APIError can be returned by a
// handler as well as serialised.
func (e APIError) Error() string { return e.Code + ": " + e.Message }

// The error codes this API emits.
const (
	CodeNotFound        = "not_found"
	CodeUnknownMarket   = "unknown_market"
	CodeUpstreamDown    = "upstream_unavailable"
	CodeUpstreamChanged = "upstream_changed"
	CodeRateLimited     = "rate_limited"
	CodeTimeout         = "timeout"
	CodeInternal        = "internal"
)

// writeJSON serialises v with the given status. Encoding failures are logged
// rather than surfaced: the status line has already been sent, so there is no
// honest way to change the answer.
func writeJSON(w http.ResponseWriter, r *http.Request, status int, v any) {
	body, err := json.Marshal(v)
	if err != nil {
		loggerFrom(r).ErrorContext(r.Context(), "server: could not encode response",
			slog.String("error", err.Error()))
		http.Error(w, `{"code":"internal","message":"response could not be encoded"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

// writeError translates an error from the library into a status and a body.
//
// The mapping is the contract of this API, so it lives in one place: a caller
// must be able to tell "you asked for something that does not exist" from
// "tgju.org is having a bad day" from "tgju.org changed its markup and this
// service needs a new release".
func writeError(w http.ResponseWriter, r *http.Request, err error) {
	status, code := classify(err)

	apiErr := APIError{Code: code, Message: message(err, code), RequestID: RequestIDFrom(r.Context())}

	log := loggerFrom(r)
	if status >= http.StatusInternalServerError {
		log.ErrorContext(r.Context(), "server: request failed",
			slog.String("code", code), slog.Int("status", status), slog.String("error", err.Error()))
	} else {
		log.InfoContext(r.Context(), "server: request rejected",
			slog.String("code", code), slog.Int("status", status))
	}

	writeJSON(w, r, status, apiErr)
}

func classify(err error) (int, string) {
	var apiErr APIError
	if errors.As(err, &apiErr) {
		return statusForCode(apiErr.Code), apiErr.Code
	}

	switch {
	case errors.Is(err, tgju.ErrUnknownMarket):
		return http.StatusNotFound, CodeUnknownMarket
	case errors.Is(err, tgju.ErrNotFound):
		return http.StatusNotFound, CodeNotFound
	case errors.Is(err, tgju.ErrParse), errors.Is(err, tgju.ErrEmpty):
		// The service is healthy; tgju.org is no longer what it was.
		return http.StatusBadGateway, CodeUpstreamChanged
	case errors.Is(err, context.DeadlineExceeded):
		return http.StatusGatewayTimeout, CodeTimeout
	case errors.Is(err, context.Canceled):
		return http.StatusRequestTimeout, CodeTimeout
	case errors.Is(err, tgju.ErrRequest), errors.Is(err, tgju.ErrUnexpectedStatus), errors.Is(err, tgju.ErrTooLarge):
		return http.StatusBadGateway, CodeUpstreamDown
	default:
		return http.StatusInternalServerError, CodeInternal
	}
}

func statusForCode(code string) int {
	switch code {
	case CodeNotFound, CodeUnknownMarket:
		return http.StatusNotFound
	case CodeRateLimited:
		return http.StatusTooManyRequests
	case CodeTimeout:
		return http.StatusGatewayTimeout
	case CodeUpstreamDown, CodeUpstreamChanged:
		return http.StatusBadGateway
	default:
		return http.StatusInternalServerError
	}
}

// message keeps the wording of a failure under this package's control. Handing
// a raw upstream error to a client leaks internal URLs and tells an attacker
// more than it tells a user, so only the errors this API defines speak for
// themselves.
func message(err error, code string) string {
	var apiErr APIError
	if errors.As(err, &apiErr) && apiErr.Message != "" {
		return apiErr.Message
	}

	switch code {
	case CodeUnknownMarket:
		return "no such market; see GET /v1/markets"
	case CodeNotFound:
		return "no such instrument on the requested market"
	case CodeUpstreamChanged:
		return "tgju.org served a page this service could not parse"
	case CodeUpstreamDown:
		return "tgju.org could not be reached"
	case CodeTimeout:
		return "the request took too long"
	case CodeRateLimited:
		return "too many requests"
	default:
		return "internal error"
	}
}

// notFound answers with the API's own JSON body instead of the standard library
// plain text, so a client never has to parse two error formats.
func notFound(w http.ResponseWriter, r *http.Request) {
	writeError(w, r, APIError{Code: CodeNotFound, Message: "no such endpoint; see GET /docs"})
}
