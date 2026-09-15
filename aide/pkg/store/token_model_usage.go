package store

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/jmylchreest/aide/aide/pkg/memory"
	"github.com/jmylchreest/aide/aide/pkg/observe"
)

const maxUsageTokens int64 = 1<<53 - 1

var usageCounterNames = []string{"input_tokens", "uncached_input_tokens", "cache_read_input_tokens", "cache_write_input_tokens", "output_tokens", "reasoning_output_tokens", "total_tokens", "reported_output_tokens"}

type usageRecord struct {
	event       *observe.Event
	fingerprint string
	counters    map[string]int64
	valid       bool
	conflict    bool
	selected    []*observe.Event
}
type modelUsage struct{ records map[string]*usageRecord }

func newModelUsage() *modelUsage { return &modelUsage{records: map[string]*usageRecord{}} }
func isModelUsage(e *observe.Event) bool {
	return e.Kind == observe.KindSession && e.Name == "model_usage"
}
func usageText(s string) bool {
	return len(s) > 0 && len(s) <= 512 && strings.TrimSpace(s) == s && strings.IndexFunc(s, unicode.IsControl) < 0
}
func usageIdentity(e *observe.Event) string {
	if !usageText(e.SessionID) || !usageText(e.Attrs["host"]) || !usageText(e.Attrs["usage_id"]) {
		return ""
	}
	b, _ := json.Marshal([]string{e.SessionID, e.Attrs["host"], e.Attrs["usage_id"]})
	return string(b)
}

// Source rows can repeat a response with different timestamps (e.g. tool fanout).
// Equality uses counters and source identity; the earliest retained time wins.
func usageFingerprint(e *observe.Event) string {
	fields := []string{e.Attrs["model_usage_version"], e.Attrs["usage_source"], e.Attrs["usage_time_basis"], e.Attrs["model"], e.Attrs["provider"], e.Attrs["usage_invalid"]}
	if e.Attrs["usage_time_basis"] == "source" {
		at, err := time.Parse(time.RFC3339Nano, e.Attrs["usage_source_time"])
		fields = append(fields, strconv.FormatBool(err == nil && !at.IsZero()))
	}
	for _, k := range append([]string{"model", "provider"}, usageCounterNames...) {
		v, ok := e.Attrs[k]
		fields = append(fields, strconv.FormatBool(ok), v)
	}
	b, _ := json.Marshal(fields)
	return string(b)
}
func usageOrigin(e *observe.Event) string {
	if !isModelUsage(e) {
		return ""
	}
	id := usageIdentity(e)
	if id == "" {
		return ""
	}
	b, _ := json.Marshal([]string{"model_usage", id, usageFingerprint(e)})
	return string(b)
}
func parseUsage(e *observe.Event) (map[string]int64, bool) {
	a := e.Attrs
	sources := map[string]string{"codex": "codex.token_usage_record.v1", "claude-code": "claude.assistant_usage.v1", "opencode": "opencode.step_finish.v1"}
	if a["usage_invalid"] != "" || a["model_usage_version"] != "1" || usageIdentity(e) == "" || sources[a["host"]] == "" || sources[a["host"]] != a["usage_source"] || e.Timestamp.IsZero() || (a["usage_time_basis"] != "source" && a["usage_time_basis"] != "observed") {
		return nil, false
	}
	if a["usage_time_basis"] == "source" {
		at, err := time.Parse(time.RFC3339Nano, a["usage_source_time"])
		if err != nil || at.IsZero() {
			return nil, false
		}
	}
	for _, k := range []string{"model", "provider"} {
		if v, ok := a[k]; ok && !usageText(v) {
			return nil, false
		}
	}
	counters, ok := parseUsageCounters(a)
	if !ok || !validUsageTotals(counters) {
		return nil, false
	}
	return counters, true
}

func parseUsageCounters(a map[string]string) (map[string]int64, bool) {
	counters := map[string]int64{}
	for _, k := range usageCounterNames {
		if v, ok := a[k]; ok {
			if v == "" || strings.IndexFunc(v, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
				return nil, false
			}
			n, err := strconv.ParseInt(v, 10, 64)
			if err != nil || n > maxUsageTokens {
				return nil, false
			}
			counters[k] = n
		}
	}
	if len(counters) == 0 {
		return nil, false
	}
	return counters, true
}

func validUsageTotals(counters map[string]int64) bool {
	input, hasInput := counters["input_tokens"]
	output, hasOutput := counters["output_tokens"]
	if hasInput {
		sum := int64(0)
		all := true
		for _, k := range []string{"uncached_input_tokens", "cache_read_input_tokens", "cache_write_input_tokens"} {
			n, ok := counters[k]
			all = all && ok
			if n > input || sum > maxUsageTokens-n {
				return false
			}
			sum += n
		}
		if sum > input || (all && sum != input) {
			return false
		}
	}
	if n, ok := counters["reasoning_output_tokens"]; ok && hasOutput && n > output {
		return false
	}
	if total, ok := counters["total_tokens"]; ok {
		for k, n := range counters {
			if k != "reported_output_tokens" && n > total {
				return false
			}
		}
	}
	if n, ok := counters["total_tokens"]; ok && hasInput && hasOutput && (input > maxUsageTokens-output || n != input+output) {
		return false
	}
	return true
}
func (m *modelUsage) observe(e *observe.Event) {
	if !isModelUsage(e) {
		return
	}
	eventCopy := *e
	e = &eventCopy
	if e.Attrs["usage_time_basis"] == "source" {
		if at, err := time.Parse(time.RFC3339Nano, e.Attrs["usage_source_time"]); err == nil && !at.IsZero() {
			e.Timestamp = at
		}
	}
	identity := usageIdentity(e)
	if identity == "" {
		identity = "invalid:" + e.ID
	}
	fp := usageFingerprint(e)
	if old := m.records[identity]; old != nil {
		if old.fingerprint != fp {
			old.conflict = true
			old.selected = append(old.selected, e)
		}
		if e.Timestamp.Before(old.event.Timestamp) && old.fingerprint == fp {
			old.event = e
			old.selected[0] = e
		}
		return
	}
	c, valid := parseUsage(e)
	m.records[identity] = &usageRecord{event: e, fingerprint: fp, counters: c, valid: valid, selected: []*observe.Event{e}}
}
func usageSelected(e *observe.Event, session string, since, until time.Time) bool {
	return (session == "" || e.SessionID == session) && (since.IsZero() || !e.Timestamp.Before(since)) && (until.IsZero() || !e.Timestamp.After(until))
}
func (m *modelUsage) result(session string, since, until time.Time) *memory.TokenModelUsage {
	usage, _ := m.resultWithSessions(session, since, until)
	return usage
}
func (m *modelUsage) resultWithSessions(session string, since, until time.Time) (*memory.TokenModelUsage, map[string]bool) {
	sessions := map[string]bool{}
	out := &memory.TokenModelUsage{Version: 1, BySource: []*memory.ModelUsageSource{}}
	groups := map[string]*memory.ModelUsageSource{}
	ids := make([]string, 0, len(m.records))
	for id := range m.records {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		r := m.records[id]
		if r.conflict {
			for _, e := range r.selected {
				if usageSelected(e, session, since, until) {
					out.Conflicts++
					break
				}
			}
			continue
		}
		e := r.event
		if !usageSelected(e, session, since, until) {
			continue
		}
		if !r.valid {
			out.Invalid++
			continue
		}
		a := e.Attrs
		keyBytes, _ := json.Marshal([]string{a["host"], a["usage_source"], a["model"], a["provider"]})
		key := string(keyBytes)
		g := groups[key]
		if g == nil {
			g = &memory.ModelUsageSource{Host: a["host"], Source: a["usage_source"], Model: a["model"], Provider: a["provider"], Counters: map[string]*memory.ModelUsageCounter{}}
		}
		overflow := false
		for k, n := range r.counters {
			if c := g.Counters[k]; c != nil && c.Tokens > maxUsageTokens-n {
				overflow = true
			}
		}
		if overflow {
			out.Invalid++
			continue
		}
		groups[key] = g
		sessions[e.SessionID] = true
		g.Observations++
		out.Observations++
		if a["usage_time_basis"] == "source" {
			g.SourceTimed++
		} else {
			g.ObservedTimed++
		}
		for k, n := range r.counters {
			c := g.Counters[k]
			if c == nil {
				c = &memory.ModelUsageCounter{}
				g.Counters[k] = c
			}
			c.Tokens += n
			c.Observations++
		}
	}
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		out.BySource = append(out.BySource, groups[k])
	}
	return out, sessions
}
