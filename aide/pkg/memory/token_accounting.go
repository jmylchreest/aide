package memory

import (
	"strconv"
	"time"
)

// TokenQuantity describes observed UTF-8 text at one boundary. It excludes
// opaque media, transport framing and provider usage. Tokens are estimates.
type TokenQuantity struct {
	Bytes           int64 `json:"bytes"`
	EstimatedTokens int64 `json:"estimated_tokens"`
	Events          int   `json:"events"`
}

// TokenActivityBucket contains recorded measurements for one UTC interval.
// Unmeasured observations are counted separately, never imputed as zero.
type TokenActivityBucket struct {
	Start      time.Time                 `json:"start"`
	ByStage    map[string]*TokenQuantity `json:"by_stage"`
	Unmeasured int                       `json:"unmeasured"`
}

// TokenActivity is a bounded, sparse time series of recorded result text.
type TokenActivity struct {
	IntervalSeconds int64                  `json:"interval_seconds"`
	Buckets         []*TokenActivityBucket `json:"buckets"`
}

// TokenAccounting is additive to the legacy statistics. Stages must not be
// summed: server and host observations can describe the same unjoined call.
type TokenAccounting struct {
	Transformations *TokenTransformations     `json:"transformations,omitempty"`
	Activity        *TokenActivity            `json:"activity,omitempty"`
	Version         int                       `json:"version"`
	Estimator       string                    `json:"estimator"`
	ByStage         map[string]*TokenQuantity `json:"by_stage"`
	Arguments       TokenQuantity             `json:"arguments"`
	LegacyEvents    int                       `json:"legacy_events"`
	MissingPayload  int                       `json:"missing_payload"`
	MissingIdentity int                       `json:"missing_identity"`
}

const TextEstimator = "utf8-bytes/3-v1"

// MeasuredBytes preserves known zero and rejects malformed or negative sizes.
func MeasuredBytes(attrs map[string]string, key string) (int64, bool) {
	if attrs["accounting_version"] != "1" {
		return 0, false
	}
	n, err := strconv.ParseInt(attrs[key], 10, 64)
	return n, err == nil && n >= 0 && n <= 1<<53-1
}

func EstimateTextTokens(bytes int64) int64 { return bytes/3 + (bytes%3+1)/3 }

func NewTokenAccounting() *TokenAccounting {
	return &TokenAccounting{Version: 1, Estimator: TextEstimator, ByStage: make(map[string]*TokenQuantity), Transformations: NewTokenTransformations()}
}

func HasObservationIdentity(session string, attrs map[string]string) bool {
	return session != "" && attrs["host"] != "" && attrs["actor_id"] != "" &&
		attrs["invocation_id"] != "" && attrs["observation_stage"] != ""
}

func (a *TokenAccounting) Add(e *TokenEvent) {
	if e.EventType == "transformation" {
		if a.Transformations == nil {
			a.Transformations = NewTokenTransformations()
		}
		a.Transformations.Add(e)
		return
	}
	if !HasObservationIdentity(e.SessionID, e.Attrs) {
		a.MissingIdentity++
	}
	if e.Attrs["accounting_version"] != "1" {
		a.LegacyEvents++
		return
	}
	stage := e.Attrs["observation_stage"]
	if n, ok := MeasuredBytes(e.Attrs, "payload_bytes"); ok && (stage == "host_result" || stage == "server_result") {
		q := a.ByStage[stage]
		if q == nil {
			q = &TokenQuantity{}
			a.ByStage[stage] = q
		}
		q.Bytes += n
		q.EstimatedTokens += EstimateTextTokens(n)
		q.Events++
	} else {
		a.MissingPayload++
	}
	if n, ok := MeasuredBytes(e.Attrs, "argument_bytes"); ok {
		a.Arguments.Bytes += n
		a.Arguments.EstimatedTokens += EstimateTextTokens(n)
		a.Arguments.Events++
	}
}
