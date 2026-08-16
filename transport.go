package tgju

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// get retrieves url and returns its body, retrying transport failures and
// server errors according to the configured [RetryPolicy].
//
// Every failure is wrapped in an [Error] carrying the market, the URL, the
// upstream status and the number of attempts, so the caller can log one value
// instead of reconstructing the context.
func (c *Client) get(ctx context.Context, m Market, url string) ([]byte, error) {
	attempts := max(c.cfg.retry.MaxAttempts, 1)
	backoff := c.cfg.retry.Backoff

	var (
		lastErr    error
		lastStatus int
	)
	for attempt := 1; attempt <= attempts; attempt++ {
		if attempt > 1 {
			if err := sleep(ctx, backoff); err != nil {
				return nil, &Error{Op: "fetch", Market: m, URL: url, Attempts: attempt - 1,
					Err: errors.Join(ErrRequest, err)}
			}
			backoff *= 2
			if c.cfg.retry.MaxBackoff > 0 {
				backoff = min(backoff, c.cfg.retry.MaxBackoff)
			}
		}

		body, status, err := c.try(ctx, url)
		switch {
		case err == nil:
			c.cfg.logger.DebugContext(ctx, "tgju: fetched page",
				slog.String("market", string(m)), slog.String("url", url),
				slog.Int("bytes", len(body)), slog.Int("attempt", attempt))
			return body, nil

		case !retryable(err, status):
			return nil, &Error{Op: "fetch", Market: m, URL: url, StatusCode: status, Attempts: attempt, Err: err}
		}

		lastErr, lastStatus = err, status
		c.cfg.logger.WarnContext(ctx, "tgju: page fetch failed, retrying",
			slog.String("market", string(m)), slog.String("url", url),
			slog.Int("attempt", attempt), slog.Int("status", status),
			slog.String("error", err.Error()))

		// A context that is already done will not survive another attempt.
		if ctx.Err() != nil {
			return nil, &Error{Op: "fetch", Market: m, URL: url, StatusCode: status, Attempts: attempt,
				Err: errors.Join(ErrRequest, ctx.Err())}
		}
	}

	return nil, &Error{Op: "fetch", Market: m, URL: url, StatusCode: lastStatus, Attempts: attempts, Err: lastErr}
}

// try performs one request. It returns the body, the HTTP status when there was
// a response, and the error.
func (c *Client) try(ctx context.Context, url string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("%w: %w", ErrRequest, err)
	}

	req.Header.Set("User-Agent", c.cfg.userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	req.Header.Set("Accept-Language", "fa-IR,fa;q=0.9,en;q=0.8")
	for name, values := range c.cfg.header {
		req.Header[http.CanonicalHeaderKey(name)] = values
	}

	resp, err := c.cfg.http.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("%w: %w", ErrRequest, err)
	}
	defer func() {
		// Draining before closing lets the connection go back to the pool.
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode, fmt.Errorf("%w: %s", ErrUnexpectedStatus, resp.Status)
	}

	// One byte over the limit is read on purpose: it is how an oversized body
	// is told apart from one that happens to end exactly at the cap.
	body, err := io.ReadAll(io.LimitReader(resp.Body, c.cfg.maxBodySize+1))
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("%w: %w", ErrRequest, err)
	}
	if int64(len(body)) > c.cfg.maxBodySize {
		return nil, resp.StatusCode, fmt.Errorf("%w: over %d bytes", ErrTooLarge, c.cfg.maxBodySize)
	}
	return body, resp.StatusCode, nil
}

// retryable reports whether another attempt could plausibly succeed. A body
// that was too large or a status the site will keep returning is permanent; a
// dropped connection, a rate limit and a server error are not.
func retryable(err error, status int) bool {
	switch {
	case errors.Is(err, ErrTooLarge):
		return false
	case status == 0:
		return true // no response at all: transport level failure
	case status == http.StatusTooManyRequests, status == http.StatusRequestTimeout:
		return true
	default:
		return status >= 500
	}
}

// sleep pauses for d unless the context ends first.
func sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
