package grpcapi

import (
	"context"
	"testing"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/memory"
	"github.com/jmylchreest/aide/aide/pkg/store"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type tokenLimitStore struct {
	store.Store
	requested int
	events    []*memory.TokenEvent
}

func (s *tokenLimitStore) ListTokenEvents(_ string, limit int, _, _ time.Time) ([]*memory.TokenEvent, error) {
	s.requested = limit
	return s.events, nil
}

func TestTokenEventsUnlimitedRequestFailsInsteadOfTruncating(t *testing.T) {
	st := &tokenLimitStore{events: make([]*memory.TokenEvent, MaxTokenEventListLimit+1)}
	svc := &tokenServiceImpl{store: st}
	for _, limit := range []int32{0, -1} {
		if _, err := svc.ListTokenEvents(context.Background(), &TokenEventListRequest{Limit: limit}); status.Code(err) != codes.ResourceExhausted {
			t.Fatalf("limit %d silently truncated: %v", limit, err)
		}
		if st.requested != MaxTokenEventListLimit+1 {
			t.Fatalf("missing overflow sentinel: %d", st.requested)
		}
	}
	st.events = []*memory.TokenEvent{{ID: "one", Timestamp: time.Now()}}
	for _, limit := range []int32{0, -1} {
		got, err := svc.ListTokenEvents(context.Background(), &TokenEventListRequest{Limit: limit})
		if err != nil || len(got.Events) != 1 || !got.TimeRangeApplied {
			t.Fatalf("safe complete request failed: %+v %v", got, err)
		}
	}
	st.requested = 0
	if _, err := svc.ListTokenEvents(context.Background(), &TokenEventListRequest{Limit: MaxTokenEventListLimit + 1}); status.Code(err) != codes.InvalidArgument || st.requested != 0 {
		t.Fatalf("oversized positive limit reached store: %d %v", st.requested, err)
	}
}
