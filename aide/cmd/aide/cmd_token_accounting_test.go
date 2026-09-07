package main

import (
	"testing"
	"time"
)

func TestTokenTimeRange(t *testing.T) {
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	since, until, err := tokenTimeRange([]string{"--since=24h", "--until=2026-09-07T11:00:00Z"}, now)
	if err != nil || !since.Equal(now.Add(-24*time.Hour)) || !until.Equal(now.Add(-time.Hour)) {
		t.Fatalf("range: %v %v %v", since, until, err)
	}
	for _, args := range [][]string{{"--since=bad"}, {"--since=-1h"}, {"--until=bad"}, {"--since=2026-09-08T00:00:00Z", "--until=2026-09-07T00:00:00Z"}} {
		if _, _, err := tokenTimeRange(args, now); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
}
