package instinct

import (
	"fmt"
	"sort"
	"strings"

	"github.com/jmylchreest/aide/aide/pkg/observe"
	"github.com/oklog/ulid/v2"
)

const ShapeFriction = "friction"

type FrictionConfig struct {
	MinCount      int // failures of the same target needed to fire (default 2)
	WindowMinutes int // run-length constraint (default 30)
}

func DefaultFrictionConfig() FrictionConfig {
	return FrictionConfig{MinCount: 2, WindowMinutes: 30}
}

// Friction detects the same tool failing repeatedly on the same target — a
// recurring obstacle the agent keeps hitting (a command that won't run, a file
// that won't apply, a check that keeps failing). The lesson worth keeping is
// usually the *fix*, which only becomes clear by reading the evidence — so this
// is an LLM-tier detector: it gathers the failures deterministically but only
// surfaces them during the reviewed reflect pass, never auto-fires in the Stop
// hook.
type Friction struct {
	Config FrictionConfig
}

func (Friction) Name() string { return ShapeFriction }
func (f Friction) DefaultConfig() any {
	if f.Config.MinCount <= 0 {
		return DefaultFrictionConfig()
	}
	return f.Config
}

// Capabilities marks friction as LLM-tier: deciding whether a cluster of
// failures is durable friction worth remembering (vs. a flaky one-off or a typo
// immediately corrected) needs the agent's judgement over the evidence, so it
// runs only in the skill pass.
func (Friction) Capabilities() Capabilities { return Capabilities{RequiresLLM: true} }

func (Friction) Detect(events []*observe.Event, cfgAny any, _ ParserContext) []Proposal {
	cfg, _ := cfgAny.(FrictionConfig)
	if cfg.MinCount <= 0 {
		cfg = DefaultFrictionConfig()
	}

	type occ struct {
		sig      string
		evidence []*observe.Event
	}
	bySig := make(map[string]*occ)
	for _, e := range events {
		if e.Kind != observe.KindToolCall || strings.TrimSpace(e.Error) == "" {
			continue
		}
		sig := frictionSignature(e)
		if sig == "" {
			continue
		}
		if v, ok := bySig[sig]; ok {
			v.evidence = append(v.evidence, e)
		} else {
			bySig[sig] = &occ{sig: sig, evidence: []*observe.Event{e}}
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
		window := densestWindow(o.evidence, cfg.MinCount, cfg.WindowMinutes)
		if window == nil {
			continue
		}

		ids := make([]string, 0, len(window))
		for _, ev := range window {
			ids = append(ids, ev.ID)
		}
		spanMin := window[len(window)-1].Timestamp.Sub(window[0].Timestamp).Minutes()
		firstErr := trimText(window[0].Error, 120)

		out = append(out, Proposal{
			ID:    ulid.Make().String(),
			Shape: ShapeFriction,
			Summary: fmt.Sprintf(
				"%s failed %d times in a %.0f-minute window (e.g. %q) — capture the fix so it isn't rediscovered.",
				sig, len(window), spanMin, firstErr,
			),
			Evidence: Evidence{
				ObserveEventIDs: ids,
				Snapshot:        snapshotEvents(window, 5),
			},
			ProposedInstinct: ProposedMemory{
				Category: "instinct",
				Content: fmt.Sprintf(
					"[DRAFT — rewrite with concrete context before accepting] "+
						"Repeated friction: %s failed %d× in a %.0f-minute window. "+
						"First error: %q. Capture the ROOT CAUSE and the fix that "+
						"resolved it (the missing flag, the prerequisite, the correct "+
						"path) so a future session avoids the loop.",
					sig, len(window), spanMin, firstErr,
				),
				Tags:     []string{"instinct", "shape:friction", "draft", "instinct_key:friction:" + sig},
				Priority: 0.7,
			},
		})
	}
	return out
}

// frictionSignature groups failures by the thing that failed. For Bash that's
// the normalised command (so the same failing command clusters but different
// commands don't); for file tools it's the tool + path; otherwise the tool name
// plus any file path.
func frictionSignature(e *observe.Event) string {
	if strings.EqualFold(e.Name, "Bash") {
		if cmd := normaliseBash(commandFromEvent(e)); cmd != "" {
			return "Bash `" + cmd + "`"
		}
		return ""
	}
	if e.FilePath != "" {
		return e.Name + " " + displayPath(e.FilePath)
	}
	return e.Name
}
