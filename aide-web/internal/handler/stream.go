package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

const sseHeartbeatInterval = 15 * time.Second

// StreamSSE bridges a typed stream into an SSE response. idOf is written as
// the SSE id: field so browsers can resume via Last-Event-ID on reconnect.
//
// Headers are written and flushed immediately on entry so EventSource fires
// onopen even before the first event arrives. A heartbeat comment is sent
// every sseHeartbeatInterval to keep idle connections alive through proxies.
func StreamSSE[T any](w http.ResponseWriter, r *http.Request, recv func() (T, error), idOf func(T) string) error {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return errors.New("streaming unsupported: ResponseWriter is not http.Flusher")
	}

	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ctx := r.Context()

	type result struct {
		msg T
		err error
	}
	results := make(chan result, 1)
	go func() {
		defer close(results)
		for {
			msg, err := recv()
			select {
			case results <- result{msg: msg, err: err}:
			case <-ctx.Done():
				return
			}
			if err != nil {
				return
			}
		}
	}()

	heartbeat := time.NewTicker(sseHeartbeatInterval)
	defer heartbeat.Stop()

	var writeMu sync.Mutex
	write := func(format string, args ...any) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		if _, err := fmt.Fprintf(w, format, args...); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-heartbeat.C:
			if err := write(": keepalive\n\n"); err != nil {
				return err
			}
		case r, ok := <-results:
			if !ok {
				return nil
			}
			if errors.Is(r.err, io.EOF) {
				return nil
			}
			if r.err != nil {
				if ctx.Err() != nil {
					return nil
				}
				return r.err
			}
			data, err := json.Marshal(r.msg)
			if err != nil {
				continue
			}
			if id := idOf(r.msg); id != "" {
				if err := write("id: %s\n", id); err != nil {
					return err
				}
			}
			if err := write("data: %s\n\n", data); err != nil {
				return err
			}
		}
	}
}
