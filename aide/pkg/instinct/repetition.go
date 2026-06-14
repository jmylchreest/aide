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
		// Shell utilities where high repetition is normal and conveys no
		// "the agent forgot the answer" signal: navigation, output, and
		// trivial test-scaffolding. Note `cat` is deliberately NOT here —
		// re-reading the same file repeatedly IS the canonical pattern
		// this detector should catch.
		IgnoreCommands: []string{
			"git status",
			"git add",
			"ls",
			"pwd",
			"cd",
			"echo",
			"printf",
			"true",
			"false",
		},
	}
}

// Repetition optionally carries a Config that overrides the package defaults.
// Zero-value (Repetition{}) keeps using DefaultRepetitionConfig() — existing
// callers don't need to change.
type Repetition struct {
	Config RepetitionConfig
}

func (Repetition) Name() string { return ShapeRepetition }
func (r Repetition) DefaultConfig() any {
	if r.Config.MinCount <= 0 {
		return DefaultRepetitionConfig()
	}
	return r.Config
}
func (Repetition) Capabilities() Capabilities { return Capabilities{RequiresLLM: false} }

// Detect counts how many times each normalised Bash command was issued in
// the session. Any command crossing MinCount within WindowMinutes becomes
// a candidate proposal.
func (Repetition) Detect(events []*observe.Event, cfgAny any, _ ParserContext) []Proposal {
	cfg, _ := cfgAny.(RepetitionConfig)
	if cfg.MinCount <= 0 {
		cfg = DefaultRepetitionConfig()
	}
	ignore := make([]string, 0, len(cfg.IgnoreCommands))
	for _, c := range cfg.IgnoreCommands {
		if n := normaliseBash(c); n != "" {
			ignore = append(ignore, n)
		}
	}

	type occ struct {
		cmd      string
		evidence []*observe.Event
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
		if isIgnored(sig, ignore) {
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

// normaliseBash reduces a Bash command to a stable signature for grouping
// repeated invocations. It is pipeline-aware and argument-preserving: every
// pipeline stage contributes its executable (path-stripped) plus its
// arguments, with leading env assignments and redirections removed and
// whitespace collapsed.
//
// Arguments are kept deliberately. Repetition means "the agent ran the SAME
// command again" (it forgot it already had the answer), NOT "the agent used
// the same tool on different inputs". So `sed -n 1,20p a.go` and
// `sed -n 90,110p b.go` must produce DIFFERENT signatures — they were two
// distinct lookups, not a repeat. Likewise a pipeline keeps all of its
// stages, so `cat x | jq .a` no longer collapses to a bare `cat`.
func normaliseBash(cmd string) string {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return ""
	}
	stages := strings.Split(cmd, "|")
	out := make([]string, 0, len(stages))
	for _, stage := range stages {
		if sig := normaliseStage(stage); sig != "" {
			out = append(out, sig)
		}
	}
	return strings.Join(out, " | ")
}

// normaliseStage normalises one pipeline stage: drop leading `KEY=value`
// environment assignments, strip the executable's path, drop redirections
// and their targets, and collapse whitespace. Returns "" for an empty or
// all-environment stage.
func normaliseStage(stage string) string {
	parts := strings.Fields(stage)
	i := 0
	for i < len(parts) && isEnvAssignment(parts[i]) {
		i++
	}
	if i >= len(parts) {
		return ""
	}
	exe := parts[i]
	if idx := strings.LastIndex(exe, "/"); idx >= 0 {
		exe = exe[idx+1:]
	}
	tokens := []string{exe}
	for j := i + 1; j < len(parts); j++ {
		tok := parts[j]
		switch tok {
		case ">", ">>", "<", "2>", "2>>", "&>", "&>>":
			j++ // standalone operator: also skip its target token
			continue
		}
		// Attached forms: 2>/dev/null, &>out, >out, >>file.
		if strings.HasPrefix(tok, "2>") || strings.HasPrefix(tok, "&>") ||
			(strings.HasPrefix(tok, ">") && len(tok) > 1) {
			continue
		}
		tokens = append(tokens, tok)
	}
	return strings.Join(tokens, " ")
}

// isEnvAssignment reports whether tok is a leading `KEY=value` shell
// assignment (not a flag like `--opt=val`).
func isEnvAssignment(tok string) bool {
	if strings.HasPrefix(tok, "-") {
		return false
	}
	return strings.IndexByte(tok, '=') > 0
}

// isIgnored reports whether sig matches an ignore entry exactly or as a
// command prefix, so `ls` ignores `ls -la /foo` and `git status` ignores
// `git status -s`. Entries are pre-normalised by the caller.
func isIgnored(sig string, ignore []string) bool {
	for _, ig := range ignore {
		if sig == ig || strings.HasPrefix(sig, ig+" ") {
			return true
		}
	}
	return false
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
