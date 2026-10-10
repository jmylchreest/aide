package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmylchreest/aide/aide/pkg/blueprint"
	"github.com/jmylchreest/aide/aide/pkg/config"
	"github.com/jmylchreest/aide/aide/pkg/store"
)

func registryBlueprint(name, decision string, includes ...string) *blueprint.Blueprint {
	return &blueprint.Blueprint{
		SchemaVersion: 1, Name: name, DisplayName: name, Description: "Registry fixture", Version: "1.0.0",
		Includes:  includes,
		Decisions: []blueprint.BlueprintDecision{{Topic: name, Decision: decision, Rationale: "Registry test"}},
	}
}

func blueprintRegistryServer(t *testing.T, entries ...*blueprint.Blueprint) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	for _, bp := range entries {
		mux.HandleFunc("/"+bp.Name+".json", func(w http.ResponseWriter, _ *http.Request) {
			if err := json.NewEncoder(w).Encode(bp); err != nil {
				t.Errorf("encode blueprint: %v", err)
			}
		})
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func writeBlueprintRegistries(t *testing.T, path string, registries []string) {
	t.Helper()
	data, err := json.Marshal(map[string]any{"blueprints": map[string]any{"registries": registries}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func loadBlueprintConfig(t *testing.T, root string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, ".aide", "memory"), 0o755); err != nil {
		t.Fatal(err)
	}
	prior := config.Get()
	t.Cleanup(func() { config.Set(prior) })
	if _, err := config.Load(root); err != nil {
		t.Fatal(err)
	}
}

func TestBlueprintConfiguredRegistryImportIncludesAndDryRun(t *testing.T) {
	dbPath, root := newConfigProject(t)
	srv := blueprintRegistryServer(t,
		registryBlueprint("team-writing", "Write clearly", "team-base"),
		registryBlueprint("team-base", "Use concrete examples"))
	writeBlueprintRegistries(t, config.GlobalFilePath(), []string{srv.URL})
	loadBlueprintConfig(t, root)

	if err := cmdBlueprint(dbPath, []string{"import", "team-writing", "--dry-run"}); err != nil {
		t.Fatal(err)
	}
	b, err := NewBackend(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = b.GetDecision("team-writing")
	_ = b.Close()
	if err != store.ErrNotFound {
		t.Fatalf("dry-run wrote a decision: %v", err)
	}
	if err := cmdBlueprint(dbPath, []string{"import", "team-writing"}); err != nil {
		t.Fatal(err)
	}
	b, err = NewBackend(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	for _, name := range []string{"team-writing", "team-base"} {
		d, err := b.GetDecision(name)
		if err != nil || d.DecidedBy != "blueprint:"+name+"@1.0.0" {
			t.Fatalf("missing imported blueprint %s: %+v %v", name, d, err)
		}
	}
}

func TestBlueprintRegistryShowUsesProjectOverride(t *testing.T) {
	dbPath, root := newConfigProject(t)
	global := blueprintRegistryServer(t, registryBlueprint("team-writing", "Global preference"))
	project := blueprintRegistryServer(t, registryBlueprint("team-writing", "Project preference"))
	writeBlueprintRegistries(t, config.GlobalFilePath(), []string{global.URL})
	writeBlueprintRegistries(t, config.FilePath(root), []string{project.URL})
	loadBlueprintConfig(t, root)
	out := captureStdout(t, func() {
		if err := cmdBlueprint(dbPath, []string{"show", "team-writing"}); err != nil {
			t.Error(err)
		}
	})
	if !strings.Contains(out, "Project preference") || strings.Contains(out, "Global preference") {
		t.Fatalf("wrong registry preview: %s", out)
	}
}

func TestBlueprintOneOffRegistryPrecedesConfiguredWithFallback(t *testing.T) {
	dbPath, root := newConfigProject(t)
	configured := blueprintRegistryServer(t,
		registryBlueprint("team-writing", "Configured preference"),
		registryBlueprint("team-base", "Configured include"))
	oneOff := blueprintRegistryServer(t, registryBlueprint("team-writing", "One-off preference", "team-base"))
	writeBlueprintRegistries(t, config.FilePath(root), []string{configured.URL})
	loadBlueprintConfig(t, root)
	if err := cmdBlueprint(dbPath, []string{"import", "--registry=" + oneOff.URL, "team-writing"}); err != nil {
		t.Fatal(err)
	}
	b, err := NewBackend(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	for name, want := range map[string]string{"team-writing": "One-off preference", "team-base": "Configured include"} {
		d, err := b.GetDecision(name)
		if err != nil || d.Decision != want {
			t.Fatalf("decision %s: %+v %v", name, d, err)
		}
	}
	// A one-off import must not change subsequent configured resolution.
	out := captureStdout(t, func() {
		if err := blueprintShow("team-writing", dbPath); err != nil {
			t.Error(err)
		}
	})
	if !strings.Contains(out, "Configured preference") {
		t.Fatalf("one-off registry mutated configuration: %s", out)
	}
}

func TestBlueprintRegistryConfigSet(t *testing.T) {
	dbPath, root := newConfigProject(t)
	if err := cmdConfigSet(dbPath, []string{"blueprints.registries", "https://example.com/blueprints"}); err != nil {
		t.Fatal(err)
	}
	loadBlueprintConfig(t, root)
	k, err := config.Resolve(root)
	if err != nil || strings.Join(k.Strings("blueprints.registries"), ",") != "https://example.com/blueprints" {
		t.Fatalf("registry config was not saved: %v", err)
	}
}
