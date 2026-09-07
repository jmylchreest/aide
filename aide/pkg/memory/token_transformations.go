package memory

import "time"

// TokenChange is a paired observation at one boundary. Positive delta means
// less text; negative means overhead. Neither boundary proves model delivery.
type TokenChange struct {
	BeforeBytes         int64 `json:"before_bytes"`
	AfterBytes          int64 `json:"after_bytes"`
	DeltaBytes          int64 `json:"delta_bytes"`
	EstimatedTokenDelta int64 `json:"estimated_token_delta"`
	Events              int   `json:"events"`
}

type TokenChangeWindow struct {
	Host      string      `json:"host"`
	SessionID string      `json:"session_id"`
	ActorID   string      `json:"actor_id"`
	Epoch     string      `json:"epoch"`
	Stage     string      `json:"stage"`
	First     time.Time   `json:"first"`
	Last      time.Time   `json:"last"`
	Change    TokenChange `json:"change"`
}

type TokenTransformations struct {
	ByStage          map[string]*TokenChange `json:"by_stage"`
	Windows          []*TokenChangeWindow    `json:"windows"`
	WindowsLimited   bool                    `json:"windows_limited"`
	UnwindowedEvents int                     `json:"unwindowed_events"`
	InvalidEvents    int                     `json:"invalid_events"`
}

func NewTokenTransformations() *TokenTransformations {
	return &TokenTransformations{ByStage: map[string]*TokenChange{}, Windows: []*TokenChangeWindow{}}
}

func (q *TokenChange) add(before, after int64) {
	q.BeforeBytes += before
	q.AfterBytes += after
	q.DeltaBytes += before - after
	q.EstimatedTokenDelta += EstimateTextTokens(before) - EstimateTextTokens(after)
	q.Events++
}

func (a *TokenTransformations) Add(e *TokenEvent) {
	stage := e.Attrs["observation_stage"]
	before, beforeOK := MeasuredBytes(e.Attrs, "before_bytes")
	after, afterOK := MeasuredBytes(e.Attrs, "after_bytes")
	if !beforeOK || !afterOK || (stage != "rewrite_candidate" && stage != "adapter_change") {
		a.InvalidEvents++
		return
	}
	q := a.ByStage[stage]
	if q == nil {
		q = &TokenChange{}
		a.ByStage[stage] = q
	}
	q.add(before, after)
	epoch, host, actor := e.Attrs["context_epoch"], e.Attrs["host"], e.Attrs["actor_id"]
	if epoch == "" || e.Attrs["context_status"] != "active" || !HasObservationIdentity(e.SessionID, e.Attrs) {
		a.UnwindowedEvents++
		return
	}
	for _, w := range a.Windows {
		if w.Host == host && w.SessionID == e.SessionID && w.ActorID == actor && w.Epoch == epoch && w.Stage == stage {
			w.Change.add(before, after)
			if e.Timestamp.Before(w.First) {
				w.First = e.Timestamp.UTC()
			}
			if e.Timestamp.After(w.Last) {
				w.Last = e.Timestamp.UTC()
			}
			return
		}
	}
	// The store visits newest records first. Retain the first 64 distinct
	// windows and keep scanning so their totals and all stage totals are complete.
	if len(a.Windows) == 64 {
		a.WindowsLimited = true
		return
	}
	w := &TokenChangeWindow{Host: host, SessionID: e.SessionID, ActorID: actor, Epoch: epoch, Stage: stage, First: e.Timestamp.UTC(), Last: e.Timestamp.UTC()}
	w.Change.add(before, after)
	a.Windows = append(a.Windows, w)
}
