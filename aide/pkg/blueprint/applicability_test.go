package blueprint

import (
	"github.com/jmylchreest/aide/aide/pkg/memory"
	"testing"
)

func TestAllBlueprintsDeclareApplicability(t *testing.T) {
	bps, err := ListEmbedded()
	if err != nil {
		t.Fatal(err)
	}
	for _, bp := range bps {
		if bp.SchemaVersion != 2 {
			t.Errorf("%s: schema %d", bp.Name, bp.SchemaVersion)
		}
		for _, d := range bp.Decisions {
			if bp.EffectiveContext(d).Applicability == nil {
				t.Errorf("%s/%s missing context", bp.Name, d.Topic)
			}
		}
	}
	if err := ValidateBundle(bps); err != nil {
		t.Fatal(err)
	}
}

func TestApplicabilityDefaultsAndBundleConflicts(t *testing.T) {
	python := &memory.Applicability{Kind: "context", AnyOf: []memory.ApplicabilitySelector{{Language: "python"}}}
	project := &memory.Applicability{Kind: "project"}
	a := &Blueprint{Name: "a", DefaultApplicability: python, Decisions: []BlueprintDecision{{Topic: "shared", Decision: "Do it", Rationale: "why"}}}
	b := *a
	b.Name = "b"
	if err := ValidateBundle([]*Blueprint{a, &b}); err != nil {
		t.Fatalf("identical duplicates: %v", err)
	}
	b.DefaultApplicability = project
	if err := ValidateBundle([]*Blueprint{a, &b}); err == nil {
		t.Fatal("different applicability accepted for same topic")
	}
	d := a.Decisions[0]
	d.Applicability = project
	if a.EffectiveContext(d).Applicability.Kind != "project" {
		t.Fatal("override did not replace default")
	}
	chain, err := ResolveWithIncludes("python", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, bp := range chain {
		if bp.Name == "general" && bp.EffectiveContext(bp.Decisions[0]).Applicability.Kind != "project" {
			t.Fatal("include inherited caller default")
		}
	}
}

func TestSchemaCompatibilityAndMissingApplicability(t *testing.T) {
	b := &Blueprint{SchemaVersion: 1, Name: "x", DisplayName: "x", Description: "x", Version: "1", Decisions: []BlueprintDecision{{Topic: "x", Decision: "x", Rationale: "x"}}}
	if err := b.Validate(); err != nil {
		t.Fatal(err)
	}
	b.SchemaVersion = 2
	if err := b.Validate(); err == nil {
		t.Fatal("schema 2 accepted unspecified context")
	}
	b.DefaultApplicability = &memory.Applicability{Kind: "project"}
	if err := b.Validate(); err != nil {
		t.Fatal(err)
	}
	b.SchemaVersion = 1
	if err := b.Validate(); err == nil {
		t.Fatal("schema 1 silently accepted new metadata")
	}
}
