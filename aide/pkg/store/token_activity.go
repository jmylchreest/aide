package store

import (
	"sort"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/memory"
)

// Keep activity bounded while scanning the same filtered events as the totals.
// Sparse buckets are intentional: absent telemetry is not measured zero.
type tokenActivity struct {
	interval int64
	buckets  map[int64]*memory.TokenActivityBucket
}

func newTokenActivity(since, until time.Time) *tokenActivity {
	interval := int64(60)
	if !since.IsZero() {
		if until.IsZero() {
			until = time.Now()
		}
		span := int64(until.Sub(since).Seconds())
		for _, seconds := range []int64{60, 300, 900, 3600, 14400, 86400, 604800, 2592000} {
			interval = seconds
			if span/seconds <= 48 {
				break
			}
		}
	}
	return &tokenActivity{interval: interval, buckets: map[int64]*memory.TokenActivityBucket{}}
}

func (a *tokenActivity) add(e *memory.TokenEvent) {
	key := e.Timestamp.Unix() / a.interval * a.interval
	b := a.buckets[key]
	if b == nil {
		b = &memory.TokenActivityBucket{Start: time.Unix(key, 0).UTC(), ByStage: map[string]*memory.TokenQuantity{}}
		a.buckets[key] = b
	}
	stage := e.Attrs["observation_stage"]
	if n, ok := memory.MeasuredBytes(e.Attrs, "payload_bytes"); ok && (stage == "host_result" || stage == "server_result") {
		q := b.ByStage[stage]
		if q == nil {
			q = &memory.TokenQuantity{}
			b.ByStage[stage] = q
		}
		q.Bytes += n
		q.EstimatedTokens += memory.EstimateTextTokens(n)
		q.Events++
	} else {
		b.Unmeasured++
	}
	for len(a.buckets) > 64 {
		a.coarsen()
	}
}

func (a *tokenActivity) coarsen() {
	a.interval *= 2
	merged := map[int64]*memory.TokenActivityBucket{}
	for start, b := range a.buckets {
		key := start / a.interval * a.interval
		target := merged[key]
		if target == nil {
			target = &memory.TokenActivityBucket{Start: time.Unix(key, 0).UTC(), ByStage: map[string]*memory.TokenQuantity{}}
			merged[key] = target
		}
		target.Unmeasured += b.Unmeasured
		for stage, q := range b.ByStage {
			t := target.ByStage[stage]
			if t == nil {
				t = &memory.TokenQuantity{}
				target.ByStage[stage] = t
			}
			t.Bytes += q.Bytes
			t.EstimatedTokens += q.EstimatedTokens
			t.Events += q.Events
		}
	}
	a.buckets = merged
}

func (a *tokenActivity) result() *memory.TokenActivity {
	result := &memory.TokenActivity{IntervalSeconds: a.interval, Buckets: []*memory.TokenActivityBucket{}}
	for _, b := range a.buckets {
		result.Buckets = append(result.Buckets, b)
	}
	sort.Slice(result.Buckets, func(i, j int) bool { return result.Buckets[i].Start.Before(result.Buckets[j].Start) })
	return result
}
