package instinct

import (
	"fmt"
	"strings"

	"github.com/jmylchreest/aide/aide/pkg/observe"
	"github.com/oklog/ulid/v2"
)

const ShapeConvergence = "convergence"

type ConvergenceConfig struct {
	CorrectiveMarkers []string
	PositiveMarkers   []string
	// MaxTurnsBetween bounds how many events can sit between A → user
	// correction → B before the sequence is no longer considered linked.
	MaxTurnsBetween int
}

func DefaultConvergenceConfig() ConvergenceConfig {
	return ConvergenceConfig{
		CorrectiveMarkers: []string{
			"no", "instead", "actually", "wrong",
			"revert", "undo", "stop", "don't", "dont",
		},
		PositiveMarkers: []string{
			"perfect", "ship it", "lgtm", "yes", "exactly", "great",
		},
		MaxTurnsBetween: 6,
	}
}

type Convergence struct{}

func (Convergence) Name() string             { return ShapeConvergence }
func (Convergence) DefaultConfig() any        { return DefaultConvergenceConfig() }
func (Convergence) Capabilities() Capabilities {
	// Has a marker-based fallback, so works without an LLM. RequiresLLM=false
	// so the Stop-hook deterministic mode still runs it. When ParserContext
	// provides Classifications, Detect prefers those over markers — see below.
	return Capabilities{RequiresLLM: false}
}

// Detect scans for the sequence: Edit/Write (A) → user prompt containing a
// corrective marker → Edit/Write (B) on the same file → optional positive
// signal. Emits one proposal per sequence found.
//
// When ctx.Classifications contains an entry for the user-prompt event ID,
// the LLM-provided intent label is used instead of marker matching:
// intent="corrective" qualifies; intent="positive" is treated as a positive
// signal; anything else (or absence) falls back to the marker heuristic.
func (Convergence) Detect(events []*observe.Event, cfgAny any, ctx ParserContext) []Proposal {
	cfg, _ := cfgAny.(ConvergenceConfig)
	if len(cfg.CorrectiveMarkers) == 0 {
		cfg = DefaultConvergenceConfig()
	}
	correctives := buildMarkerSet(cfg.CorrectiveMarkers)
	positives := buildMarkerSet(cfg.PositiveMarkers)

	var out []Proposal
	for i, e := range events {
		if !isMutationEvent(e) {
			continue
		}
		// Look ahead for a user-prompt corrective marker within the window.
		correctionIdx, correctionText := findCorrectionAhead(events, i, cfg.MaxTurnsBetween, correctives, ctx.Classifications)
		if correctionIdx < 0 {
			continue
		}
		// Then look for a subsequent mutation on the same file.
		nextEditIdx := findMutationAhead(events, correctionIdx, cfg.MaxTurnsBetween, e.FilePath)
		if nextEditIdx < 0 {
			continue
		}
		// Optional: check whether anything positive followed.
		positiveSignal := findPositiveAhead(events, nextEditIdx, cfg.MaxTurnsBetween, positives, ctx.Classifications)

		ids := []string{e.ID, events[correctionIdx].ID, events[nextEditIdx].ID}
		snap := []*observe.Event{e, events[correctionIdx], events[nextEditIdx]}
		summary := fmt.Sprintf(
			"On %s, an edit was reverted toward a different approach after the user noted: %q.",
			displayPath(e.FilePath), trimText(correctionText, 80),
		)
		if positiveSignal {
			summary += " Followed by a positive signal."
		}

		out = append(out, Proposal{
			ID:      ulid.Make().String(),
			Shape:   ShapeConvergence,
			Summary: summary,
			Evidence: Evidence{
				ObserveEventIDs: ids,
				Snapshot:        snap,
			},
			ProposedInstinct: ProposedMemory{
				Category: "instinct",
				// DRAFT content: the agent reviewing this proposal is expected
				// to rewrite via `aide reflect accept <id> --content=…` before
				// promotion. The template carries the structural signal +
				// quoted user correction; the agent extracts the actual lesson
				// (which approach won, why it's preferred, project-specific
				// reasoning) by reading the evidence.
				Content: fmt.Sprintf(
					"[DRAFT — rewrite with concrete context before accepting] "+
						"Convergence on %s: an edit was reverted toward a different "+
						"approach after the user said %q. Capture WHY the second "+
						"approach is preferred and any project-specific reasoning.",
					displayPath(e.FilePath), trimText(correctionText, 200),
				),
				Tags:     []string{"instinct", "shape:convergence", "draft", "instinct_key:" + e.FilePath},
				Priority: 0.65,
			},
		})
	}
	return out
}

func isMutationEvent(e *observe.Event) bool {
	if e.Kind != observe.KindToolCall {
		return false
	}
	switch e.Name {
	case "Edit", "Write", "NotebookEdit":
		return true
	}
	return false
}

// findCorrectionAhead walks forward from idx up to window events and returns
// the index of the first user-prompt event judged corrective. When
// classifications include an entry for the event ID, intent="corrective"
// qualifies and anything else disqualifies (LLM beats markers). Otherwise
// falls back to marker matching against the prompt text.
// Returns (-1, "") if none found.
func findCorrectionAhead(events []*observe.Event, from, window int, markers map[string]struct{}, classifications map[string]Classification) (int, string) {
	end := min(from+1+window, len(events))
	for j := from + 1; j < end; j++ {
		ev := events[j]
		if !isUserPromptEvent(ev) {
			continue
		}
		text := userPromptText(ev)
		if text == "" {
			continue
		}
		if c, ok := classifications[ev.ID]; ok {
			if c.Intent == "corrective" {
				return j, text
			}
			continue
		}
		if containsAnyMarker(text, markers) {
			return j, text
		}
	}
	return -1, ""
}

// findMutationAhead walks forward from idx up to window events and
// returns the index of the next mutation event on filePath (or any file
// if filePath is empty). Returns -1 if none found.
func findMutationAhead(events []*observe.Event, from, window int, filePath string) int {
	end := min(from+1+window, len(events))
	for j := from + 1; j < end; j++ {
		ev := events[j]
		if !isMutationEvent(ev) {
			continue
		}
		if filePath != "" && ev.FilePath != filePath {
			continue
		}
		return j
	}
	return -1
}

func findPositiveAhead(events []*observe.Event, from, window int, markers map[string]struct{}, classifications map[string]Classification) bool {
	end := min(from+1+window, len(events))
	for j := from + 1; j < end; j++ {
		ev := events[j]
		if !isUserPromptEvent(ev) {
			continue
		}
		if c, ok := classifications[ev.ID]; ok {
			if c.Intent == "positive" {
				return true
			}
			continue
		}
		if containsAnyMarker(userPromptText(ev), markers) {
			return true
		}
	}
	return false
}

// isUserPromptEvent identifies the synthetic hook event we emit on
// UserPromptSubmit. The hook records kind=hook, name=user_prompt,
// with the text in attrs.text or attrs.prompt.
func isUserPromptEvent(e *observe.Event) bool {
	if e.Kind == observe.KindHook && (e.Name == "user_prompt" || e.Name == "UserPromptSubmit") {
		return true
	}
	return false
}

func userPromptText(e *observe.Event) string {
	if e.Attrs == nil {
		return ""
	}
	for _, k := range []string{"text", "prompt", "content"} {
		if v, ok := e.Attrs[k]; ok && v != "" {
			return v
		}
	}
	return ""
}

func containsAnyMarker(text string, markers map[string]struct{}) bool {
	lower := strings.ToLower(text)
	for _, w := range tokenise(lower) {
		if _, ok := markers[w]; ok {
			return true
		}
	}
	return false
}

func tokenise(text string) []string {
	out := make([]string, 0, 8)
	var b strings.Builder
	for _, r := range text {
		if isWordRune(r) {
			b.WriteRune(r)
		} else if b.Len() > 0 {
			out = append(out, b.String())
			b.Reset()
		}
	}
	if b.Len() > 0 {
		out = append(out, b.String())
	}
	return out
}

func isWordRune(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z':
		return true
	case r >= 'A' && r <= 'Z':
		return true
	case r >= '0' && r <= '9':
		return true
	case r == '\'':
		return true
	}
	return false
}

func buildMarkerSet(markers []string) map[string]struct{} {
	out := make(map[string]struct{}, len(markers))
	for _, m := range markers {
		out[strings.ToLower(strings.TrimSpace(m))] = struct{}{}
	}
	return out
}

func displayPath(p string) string {
	if p == "" {
		return "an unknown file"
	}
	if idx := strings.LastIndex(p, "/"); idx >= 0 && idx < len(p)-1 {
		return ".../" + p[idx+1:]
	}
	return p
}

func trimText(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
