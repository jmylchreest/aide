package instinct

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/observe"
	"github.com/oklog/ulid/v2"
)

const ShapeRepetition = "repetition"

type RepetitionConfig struct {
	MinCount       int      // events needed in the session to fire (default 4)
	WindowMinutes  int      // run-length constraint (default 30)
	IgnoreCommands []string // normalised commands to skip entirely
}

func DefaultRepetitionConfig() RepetitionConfig {
	return RepetitionConfig{
		MinCount:      4,
		WindowMinutes: 30,
		IgnoreCommands: []string{
			"git status",
			"ls",
			"pwd",
		},
	}
}

type Repetition struct{}

func (Repetition) Name() string             { return ShapeRepetition }
func (Repetition) DefaultConfig() any        { return DefaultRepetitionConfig() }
func (Repetition) Capabilities() Capabilities { return Capabilities{RequiresLLM: false} }

// Detect counts how many times each normalised Bash command was issued in
// the session. Any command crossing MinCount within WindowMinutes becomes
// a candidate proposal.
func (Repetition) Detect(events []*observe.Event, cfgAny any, _ ParserContext) []Proposal {
	cfg, _ := cfgAny.(RepetitionConfig)
	if cfg.MinCount <= 0 {
		cfg = DefaultRepetitionConfig()
	}
	ignore := make(map[string]struct{}, len(cfg.IgnoreCommands))
	for _, c := range cfg.IgnoreCommands {
		ignore[normaliseBash(c)] = struct{}{}
	}

	type occ struct {
		cmd       string
		evidence  []*observe.Event
	}
	bySig := make(map[string]*occ)
	for _, e := range events {
		if e.Kind != observe.KindToolCall || !strings.EqualFold(e.Name, "Bash") {
			continue
		}
		sig := normaliseBash(commandFromEvent(e))
		if sig == "" {
			continue
		}
		if _, skip := ignore[sig]; skip {
			continue
		}
		if v, ok := bySig[sig]; ok {
			v.evidence = append(v.evidence, e)
		} else {
			bySig[sig] = &occ{cmd: sig, evidence: []*observe.Event{e}}
		}
	}

	var out []Proposal
	for sig, o := range bySig {
		if len(o.evidence) < cfg.MinCount {
			continue
		}
		sort.Slice(o.evidence, func(i, j int) bool {
			return o.evidence[i].Timestamp.Before(o.evidence[j].Timestamp)
		})

		// Sliding window: find any run of MinCount consecutive events whose
		// span ≤ WindowMinutes. Previously the check required the WHOLE
		// run to fit the window, which silently skipped any sparse pattern
		// (4 calls in 1 min + 1 more 35 min later → never fired).
		windowEvents := densestWindow(o.evidence, cfg.MinCount, cfg.WindowMinutes)
		if windowEvents == nil {
			continue
		}

		ids := make([]string, 0, len(windowEvents))
		for _, ev := range windowEvents {
			ids = append(ids, ev.ID)
		}
		snapshot := snapshotEvents(windowEvents, 5)

		spanMin := windowEvents[len(windowEvents)-1].Timestamp.Sub(windowEvents[0].Timestamp).Minutes()
		out = append(out, Proposal{
			ID:    ulid.Make().String(),
			Shape: ShapeRepetition,
			Summary: fmt.Sprintf(
				"`%s` was run %d times in a %.0f-minute window — consider caching or remembering its result.",
				sig, len(windowEvents), spanMin,
			),
			Evidence: Evidence{
				ObserveEventIDs: ids,
				Snapshot:        snapshot,
			},
			ProposedInstinct: ProposedMemory{
				Category: "instinct",
				// DRAFT content: the agent reviewing this proposal is expected
				// to rewrite it via `aide reflect accept <id> --content=…`
				// before promotion. The template captures the structural
				// signal; the agent supplies the actual lesson (why it
				// recurred, what to do instead, project context).
				Content: fmt.Sprintf(
					"[DRAFT — rewrite with concrete context before accepting] "+
						"Repetition of `%s` (%d× in a %.0f-minute window). "+
						"Capture WHY it kept recurring and the canonical alternative "+
						"(single lookup, cache, memoise, or a project-specific path).",
					sig, len(windowEvents), spanMin,
				),
				Tags:     []string{"instinct", "shape:repetition", "draft", "instinct_key:" + sig},
				Priority: 0.55,
			},
		})
	}
	return out
}

// normaliseBash strips arguments, paths, env prefixes, and redirects to
// produce a stable signature for grouping. Conservative: just first token.
func normaliseBash(cmd string) string {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return ""
	}
	// Drop env vars: KEY=value KEY2=value cmd ...
	parts := strings.Fields(cmd)
	first := 0
	for first < len(parts) && strings.Contains(parts[first], "=") && !strings.HasPrefix(parts[first], "-") {
		first++
	}
	if first >= len(parts) {
		return ""
	}
	exe := parts[first]
	// Strip path components: /usr/bin/git → git
	if idx := strings.LastIndex(exe, "/"); idx >= 0 {
		exe = exe[idx+1:]
	}
	// For very common multi-word commands, keep the subcommand too.
	if first+1 < len(parts) && knownMultiWord[exe] {
		return exe + " " + parts[first+1]
	}
	return exe
}

// knownMultiWord lists commands whose first arg is a subcommand we want to
// keep in the signature so `git status` and `git log` aren't lumped.
var knownMultiWord = map[string]bool{
	"git":  true,
	"docker": true,
	"kubectl": true,
	"npm":  true,
	"bun":  true,
	"go":   true,
	"make": true,
	"cargo": true,
}

// commandFromEvent extracts the raw command from a Bash tool_call event.
// Bash events store the command in Name, but recorder variants may put it
// in attrs.command. Try both.
func commandFromEvent(e *observe.Event) string {
	if e.Attrs != nil {
		if c, ok := e.Attrs["command"]; ok && c != "" {
			return c
		}
	}
	return e.Name
}

// densestWindow returns the largest contiguous run of time-sorted events
// whose first-to-last span fits within windowMinutes, provided that run
// has at least minCount entries. Returns nil if no such window exists.
//
// Algorithm: standard two-pointer sliding window. O(n). Picks the longest
// qualifying run so the proposal summary reflects the worst burst, not
// the minimum threshold.
func densestWindow(events []*observe.Event, minCount, windowMinutes int) []*observe.Event {
	if len(events) < minCount {
		return nil
	}
	if windowMinutes <= 0 {
		return events
	}
	maxSpan := time.Duration(windowMinutes) * time.Minute
	bestStart, bestLen := -1, 0
	left := 0
	for right := range len(events) {
		for left < right && events[right].Timestamp.Sub(events[left].Timestamp) > maxSpan {
			left++
		}
		runLen := right - left + 1
		if runLen >= minCount && runLen > bestLen {
			bestStart = left
			bestLen = runLen
		}
	}
	if bestStart < 0 {
		return nil
	}
	return events[bestStart : bestStart+bestLen]
}

// snapshotEvents copies up to limit events for embedding in a proposal so
// the evidence survives observe-event retention.
func snapshotEvents(events []*observe.Event, limit int) []*observe.Event {
	if len(events) <= limit {
		out := make([]*observe.Event, len(events))
		copy(out, events)
		return out
	}
	out := make([]*observe.Event, limit)
	copy(out, events[:limit])
	return out
}
