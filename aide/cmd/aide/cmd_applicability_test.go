package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/jmylchreest/aide/aide/pkg/blueprint"
	"github.com/jmylchreest/aide/aide/pkg/memory"
	"github.com/jmylchreest/aide/aide/pkg/store"
)

func TestBlueprintApplicabilityMigrationPreservesEdits(t *testing.T) {
	dbPath, root := newConfigProject(t)
	loadBlueprintConfig(t, root)
	b, err := NewBackend(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	bp, err := blueprint.LoadEmbedded("go")
	if err != nil {
		t.Fatal(err)
	}
	for i, d := range bp.Decisions[:2] {
		value := d.Decision
		if i == 1 {
			value = "User edited this rule"
		}
		if _, err := b.SetDecision(d.Topic, value, d.Rationale, d.Details, "blueprint:go@0.1.18", d.References, nil); err != nil {
			t.Fatal(err)
		}
	}
	b.Close()
	if err := blueprintMigrate(dbPath, []string{"--dry-run"}); err != nil {
		t.Fatal(err)
	}
	b, err = NewBackend(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	d, _ := b.GetDecision(bp.Decisions[0].Topic)
	if d.Applicability != nil {
		t.Fatal("dry run wrote metadata")
	}
	b.Close()
	if err := blueprintMigrate(dbPath, nil); err != nil {
		t.Fatal(err)
	}
	b, err = NewBackend(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	d, _ = b.GetDecision(bp.Decisions[0].Topic)
	if d.Applicability == nil || d.Applicability.AnyOf[0].Language != "go" {
		t.Fatal("missing migrated applicability")
	}
	edited, _ := b.GetDecision(bp.Decisions[1].Topic)
	if edited.Applicability != nil || edited.Decision != "User edited this rule" {
		t.Fatal("modified decision migrated")
	}
	updated, err := b.SetDecision(d.Topic, "Reworded", d.Rationale, d.Details, "user", d.References, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(updated.DecisionContext, d.DecisionContext) {
		t.Fatal("ordinary edit lost applicability")
	}
	cleared, err := b.SetDecision(d.Topic, "Reworded", d.Rationale, d.Details, "user", d.References, nil, &memory.DecisionContext{})
	if err != nil {
		t.Fatal(err)
	}
	if cleared.Applicability != nil {
		t.Fatal("explicit clear failed")
	}
}

func TestBlueprintImportConflictsArePreflighted(t *testing.T) {
	dbPath, root := newConfigProject(t)
	loadBlueprintConfig(t, root)
	paths := []string{}
	for _, name := range []string{"a", "b"} {
		bp := registryBlueprint(name, name)
		bp.Decisions[0].Topic = "same"
		data, _ := json.Marshal(bp)
		path := filepath.Join(root, name+".json")
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, path)
	}
	if err := blueprintImport(dbPath, paths); err == nil {
		t.Fatal("conflicting import succeeded")
	}
	b, err := NewBackend(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	if _, err := b.GetDecision("same"); err != store.ErrNotFound {
		t.Fatal("preflight partially wrote", err)
	}
}
