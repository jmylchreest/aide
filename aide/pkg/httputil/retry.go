// Package httputil provides resilient HTTP helpers for aide.
package httputil

import (
	"context"
	"fmt"
	"math/rand/v2"
	"net/http"
	"time"
)

// Default retry configuration.
const (
	DefaultMaxRetries  = 3
	DefaultBaseDelay   = 500 * time.Millisecond
	DefaultMaxDelay    = 10 * time.Second
	DefaultHTTPTimeout = 30 * time.Second
)

// retryableStatusCodes are HTTP status codes worth retrying.
var retryableStatusCodes = map[int]bool{
	http.StatusTooManyRequests:     true, // 429
	http.StatusInternalServerError: true, // 500
	http.StatusBadGateway:          true, // 502
	http.StatusServiceUnavailable:  true, // 503
	http.StatusGatewayTimeout:      true, // 504
}

// RetryOption configures a Client.
type RetryOption func(*Client)

// WithMaxRetries sets the maximum number of retry attempts (not counting the
// initial request). Zero means no retries.
func WithMaxRetries(n int) RetryOption {
	return func(c *Client) { c.maxRetries = n }
}

// WithBaseDelay sets the initial backoff delay before jitter.
func WithBaseDelay(d time.Duration) RetryOption {
	return func(c *Client) { c.baseDelay = d }
}

// WithMaxDelay caps the backoff delay.
func WithMaxDelay(d time.Duration) RetryOption {
	return func(c *Client) { c.maxDelay = d }
}

// WithHTTPTimeout sets the per-request timeout on the underlying http.Client.
func WithHTTPTimeout(d time.Duration) RetryOption {
	return func(c *Client) { c.httpClient.Timeout = d }
}

// WithHTTPClient replaces the underlying http.Client entirely.
// The caller is responsible for configuring timeouts on the provided client.
func WithHTTPClient(hc *http.Client) RetryOption {
	return func(c *Client) { c.httpClient = hc }
}

// Client wraps http.Client with retry-on-transient-failure behaviour.
type Client struct {
	httpClient *http.Client
	maxRetries int
	baseDelay  time.Duration
	maxDelay   time.Duration
}

// NewClient creates a Client with sensible defaults.
func NewClient(opts ...RetryOption) *Client {
	c := &Client{
		httpClient: &http.Client{Timeout: DefaultHTTPTimeout},
		maxRetries: DefaultMaxRetries,
		baseDelay:  DefaultBaseDelay,
		maxDelay:   DefaultMaxDelay,
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Do executes an HTTP request with retries on transient failures.
//
// The request's context controls overall cancellation. Retries are attempted
// for connection errors and retryable HTTP status codes (429, 500, 502, 503,
// 504). The response body of failed attempts is closed automatically.
//
// On success (or non-retryable failure), the caller owns the response.
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	var lastErr error

	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			delay := c.backoff(attempt)
			select {
			case <-req.Context().Done():
				return nil, req.Context().Err()
			case <-time.After(delay):
			}
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			// Connection-level error (DNS, TCP reset, TLS, timeout) — retryable.
			lastErr = err
			continue
		}

		if !retryableStatusCodes[resp.StatusCode] {
			// Success or non-retryable error (4xx except 429) — return as-is.
			return resp, nil
		}

		// Retryable HTTP status — close the body and retry.
		lastErr = fmt.Errorf("HTTP %d from %s", resp.StatusCode, req.URL.Host)
		resp.Body.Close()
	}

	// All retries exhausted.
	return nil, fmt.Errorf("%w (after %d retries)", lastErr, c.maxRetries)
}

// Get is a convenience wrapper around Do for simple GET requests.
func (c *Client) Get(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return c.Do(req)
}

// backoff returns the delay for the given attempt (1-indexed) using
// exponential backoff with full jitter, capped at maxDelay.
func (c *Client) backoff(attempt int) time.Duration {
	// 2^(attempt-1) * baseDelay, capped.
	delay := c.baseDelay
	for i := 1; i < attempt; i++ {
		delay *= 2
		if delay > c.maxDelay {
			delay = c.maxDelay
			break
		}
	}
	// Full jitter: uniform random in [0, delay].
	if delay > 0 {
		delay = time.Duration(rand.Int64N(int64(delay)))
	}
	return delay
}
