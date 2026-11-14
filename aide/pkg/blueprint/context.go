package blueprint

import (
	"fmt"
	"reflect"
	"slices"

	"github.com/jmylchreest/aide/aide/pkg/memory"
)

func (bp *Blueprint) DecisionRecord(d BlueprintDecision) *memory.Decision {
	return &memory.Decision{Topic: d.Topic, Decision: d.Decision, Rationale: d.Rationale, Details: d.Details, References: d.References, Precedence: bp.EffectivePrecedence(d), DecisionContext: bp.EffectiveContext(d)}
}

// SameContent excludes provenance and revision timestamps, which differ when
// two bundles intentionally include an identical decision.
func SameContent(a, b *memory.Decision) bool {
	return a.Topic == b.Topic && a.Decision == b.Decision && a.Rationale == b.Rationale && a.Details == b.Details && a.Precedence == b.Precedence && slices.Equal(a.References, b.References) && reflect.DeepEqual(a.DecisionContext, b.DecisionContext)
}

// ValidateBundle checks the complete import before any records are written.
func ValidateBundle(bps []*Blueprint) error {
	seen := map[string]*memory.Decision{}
	owners := map[string]string{}
	contexts := map[string]memory.DecisionContext{}
	for _, bp := range bps {
		for _, d := range bp.Decisions {
			record := bp.DecisionRecord(d)
			if previous, ok := seen[d.Topic]; ok && !SameContent(previous, record) {
				return fmt.Errorf("conflicting topic %q in blueprints %s and %s; use distinct topics and explicit supersedes", d.Topic, owners[d.Topic], bp.Name)
			}
			seen[d.Topic], owners[d.Topic], contexts[d.Topic] = record, bp.Name, record.DecisionContext
		}
	}
	return memory.ValidateSupersession(contexts)
}
