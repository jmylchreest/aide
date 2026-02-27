package httputil

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestClient_SuccessOnFirstAttempt(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewClient(WithMaxRetries(3))
	resp, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("expected 1 call, got %d", got)
	}
}

func TestClient_RetriesOnServerError(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := calls.Add(1)
		if n <= 2 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewClient(
		WithMaxRetries(3),
		WithBaseDelay(1*time.Millisecond),
		WithMaxDelay(5*time.Millisecond),
	)
	resp, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if got := calls.Load(); got != 3 {
		t.Errorf("expected 3 calls (2 failures + 1 success), got %d", got)
	}
}

func TestClient_ExhaustsRetries(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	c := NewClient(
		WithMaxRetries(2),
		WithBaseDelay(1*time.Millisecond),
		WithMaxDelay(2*time.Millisecond),
	)
	_, err := c.Get(context.Background(), srv.URL) //nolint:bodyclose // error path; no body to close
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}

	// 1 initial + 2 retries = 3 total.
	if got := calls.Load(); got != 3 {
		t.Errorf("expected 3 calls, got %d", got)
	}
}

func TestClient_NoRetryOn4xx(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := NewClient(WithMaxRetries(3))
	resp, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("expected 1 call (no retries for 404), got %d", got)
	}
}

func TestClient_RetriesOn429(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := calls.Add(1)
		if n == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewClient(
		WithMaxRetries(2),
		WithBaseDelay(1*time.Millisecond),
	)
	resp, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("expected 2 calls, got %d", got)
	}
}

func TestClient_RespectsContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	c := NewClient(
		WithMaxRetries(5),
		WithBaseDelay(1*time.Second), // Would be slow without cancellation.
	)
	_, err := c.Get(ctx, srv.URL) //nolint:bodyclose // error path; no body to close
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

func TestClient_ZeroRetries(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	c := NewClient(
		WithMaxRetries(0),
	)
	_, err := c.Get(context.Background(), srv.URL) //nolint:bodyclose // error path; no body to close
	if err == nil {
		t.Fatal("expected error with 0 retries on 502")
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("expected 1 call with 0 retries, got %d", got)
	}
}

func TestClient_BackoffCapped(t *testing.T) {
	c := &Client{
		baseDelay: 100 * time.Millisecond,
		maxDelay:  200 * time.Millisecond,
	}

	// Even at high attempt numbers, backoff should never exceed maxDelay.
	for attempt := 1; attempt <= 20; attempt++ {
		delay := c.backoff(attempt)
		if delay > c.maxDelay {
			t.Errorf("attempt %d: delay %v exceeds maxDelay %v", attempt, delay, c.maxDelay)
		}
		if delay < 0 {
			t.Errorf("attempt %d: delay %v is negative", attempt, delay)
		}
	}
}

func TestClient_DefaultConstants(t *testing.T) {
	if DefaultMaxRetries <= 0 {
		t.Errorf("DefaultMaxRetries must be positive, got %d", DefaultMaxRetries)
	}
	if DefaultBaseDelay <= 0 {
		t.Errorf("DefaultBaseDelay must be positive, got %v", DefaultBaseDelay)
	}
	if DefaultMaxDelay <= DefaultBaseDelay {
		t.Errorf("DefaultMaxDelay (%v) should exceed DefaultBaseDelay (%v)", DefaultMaxDelay, DefaultBaseDelay)
	}
	if DefaultHTTPTimeout <= 0 {
		t.Errorf("DefaultHTTPTimeout must be positive, got %v", DefaultHTTPTimeout)
	}
}
