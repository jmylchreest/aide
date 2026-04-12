package blueprint

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadEmbedded(t *testing.T) {
	t.Parallel()

	names := []string{"general", "go", "github-actions", "go-github-actions", "c", "cpp", "rust", "rust-github-actions", "zig", "dart", "kotlin"}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			bp, err := LoadEmbedded(name)
			if err != nil {
				t.Fatalf("LoadEmbedded(%q): %v", name, err)
			}
			if bp.Name != name {
				t.Errorf("expected name %q, got %q", name, bp.Name)
			}
			if len(bp.Decisions) == 0 {
				t.Error("expected at least one decision")
			}
		})
	}
}

func TestLoadEmbeddedNotFound(t *testing.T) {
	t.Parallel()
	_, err := LoadEmbedded("nonexistent-blueprint")
	if err == nil {
		t.Fatal("expected error for nonexistent blueprint")
	}
}

func TestListEmbedded(t *testing.T) {
	t.Parallel()
	blueprints, err := ListEmbedded()
	if err != nil {
		t.Fatalf("ListEmbedded: %v", err)
	}
	if len(blueprints) < 4 {
		t.Errorf("expected at least 4 embedded blueprints, got %d", len(blueprints))
	}

	names := make(map[string]bool)
	for _, bp := range blueprints {
		names[bp.Name] = true
	}
	for _, want := range []string{"general", "go", "github-actions", "go-github-actions"} {
		if !names[want] {
			t.Errorf("expected embedded blueprint %q not found", want)
		}
	}
}

func TestValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		bp      Blueprint
		wantErr string
	}{
		{
			name: "valid minimal",
			bp: Blueprint{
				SchemaVersion: 1,
				Name:          "test",
				DisplayName:   "Test",
				Description:   "A test",
				Version:       "0.0.1",
				Decisions: []BlueprintDecision{
					{Topic: "t1", Decision: "d1", Rationale: "r1"},
				},
			},
		},
		{
			name: "bad schema version",
			bp: Blueprint{
				SchemaVersion: 99,
				Name:          "test",
				DisplayName:   "Test",
				Description:   "A test",
				Version:       "0.0.1",
			},
			wantErr: "unsupported schema_version",
		},
		{
			name: "missing name",
			bp: Blueprint{
				SchemaVersion: 1,
				DisplayName:   "Test",
				Description:   "A test",
				Version:       "0.0.1",
			},
			wantErr: "missing required field: name",
		},
		{
			name: "missing display_name",
			bp: Blueprint{
				SchemaVersion: 1,
				Name:          "test",
				Description:   "A test",
				Version:       "0.0.1",
			},
			wantErr: "missing required field: display_name",
		},
		{
			name: "missing description",
			bp: Blueprint{
				SchemaVersion: 1,
				Name:          "test",
				DisplayName:   "Test",
				Version:       "0.0.1",
			},
			wantErr: "missing required field: description",
		},
		{
			name: "missing version",
			bp: Blueprint{
				SchemaVersion: 1,
				Name:          "test",
				DisplayName:   "Test",
				Description:   "A test",
			},
			wantErr: "missing required field: version",
		},
		{
			name: "decision missing topic",
			bp: Blueprint{
				SchemaVersion: 1,
				Name:          "test",
				DisplayName:   "Test",
				Description:   "A test",
				Version:       "0.0.1",
				Decisions:     []BlueprintDecision{{Decision: "d", Rationale: "r"}},
			},
			wantErr: "missing required field: topic",
		},
		{
			name: "decision missing decision text",
			bp: Blueprint{
				SchemaVersion: 1,
				Name:          "test",
				DisplayName:   "Test",
				Description:   "A test",
				Version:       "0.0.1",
				Decisions:     []BlueprintDecision{{Topic: "t", Rationale: "r"}},
			},
			wantErr: "missing required field: decision",
		},
		{
			name: "decision missing rationale",
			bp: Blueprint{
				SchemaVersion: 1,
				Name:          "test",
				DisplayName:   "Test",
				Description:   "A test",
				Version:       "0.0.1",
				Decisions:     []BlueprintDecision{{Topic: "t", Decision: "d"}},
			},
			wantErr: "missing required field: rationale",
		},
		{
			name: "duplicate topic",
			bp: Blueprint{
				SchemaVersion: 1,
				Name:          "test",
				DisplayName:   "Test",
				Description:   "A test",
				Version:       "0.0.1",
				Decisions: []BlueprintDecision{
					{Topic: "same", Decision: "d1", Rationale: "r1"},
					{Topic: "same", Decision: "d2", Rationale: "r2"},
				},
			},
			wantErr: "duplicate topic",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.bp.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if got := err.Error(); !contains(got, tt.wantErr) {
				t.Errorf("error %q does not contain %q", got, tt.wantErr)
			}
		})
	}
}

func TestLoadFromDir(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	data := `{
		"schema_version": 1,
		"name": "custom",
		"display_name": "Custom",
		"description": "A custom blueprint",
		"version": "1.0.0",
		"decisions": [
			{"topic": "custom-rule", "decision": "Do it", "rationale": "Because"}
		]
	}`
	if err := os.WriteFile(filepath.Join(dir, "custom.json"), []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}

	bp, err := LoadFromDir(dir, "custom")
	if err != nil {
		t.Fatalf("LoadFromDir: %v", err)
	}
	if bp.Name != "custom" {
		t.Errorf("expected name 'custom', got %q", bp.Name)
	}
	if len(bp.Decisions) != 1 {
		t.Errorf("expected 1 decision, got %d", len(bp.Decisions))
	}
}

func TestLoadFromDirNotFound(t *testing.T) {
	t.Parallel()
	_, err := LoadFromDir(t.TempDir(), "nonexistent")
	if !os.IsNotExist(err) {
		t.Errorf("expected os.ErrNotExist, got %v", err)
	}
}

func TestLoadFromFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "my-practices.json")
	data := `{
		"schema_version": 1,
		"name": "my-practices",
		"display_name": "My Practices",
		"description": "Custom practices",
		"version": "1.0.0",
		"decisions": [
			{"topic": "my-rule", "decision": "Always", "rationale": "Because I said so"}
		]
	}`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}

	bp, err := LoadFromFile(path)
	if err != nil {
		t.Fatalf("LoadFromFile: %v", err)
	}
	if bp.Name != "my-practices" {
		t.Errorf("expected name 'my-practices', got %q", bp.Name)
	}
}

func TestResolve(t *testing.T) {
	t.Parallel()

	t.Run("embedded", func(t *testing.T) {
		t.Parallel()
		bp, source, err := Resolve("go", "", nil)
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		if source != "embedded" {
			t.Errorf("expected source 'embedded', got %q", source)
		}
		if bp.Name != "go" {
			t.Errorf("expected name 'go', got %q", bp.Name)
		}
	})

	t.Run("local override wins", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		data := `{
			"schema_version": 1,
			"name": "go",
			"display_name": "Custom Go",
			"description": "Overridden",
			"version": "99.0.0",
			"decisions": [
				{"topic": "custom", "decision": "d", "rationale": "r"}
			]
		}`
		if err := os.WriteFile(filepath.Join(dir, "go.json"), []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}

		bp, source, err := Resolve("go", dir, nil)
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		if !contains(source, "local") {
			t.Errorf("expected local source, got %q", source)
		}
		if bp.Version != "99.0.0" {
			t.Errorf("expected overridden version '99.0.0', got %q", bp.Version)
		}
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()
		_, _, err := Resolve("does-not-exist", "", nil)
		if err == nil {
			t.Fatal("expected error for nonexistent blueprint")
		}
	})
}

func TestResolveWithIncludes(t *testing.T) {
	t.Parallel()

	t.Run("go includes general", func(t *testing.T) {
		t.Parallel()
		blueprints, err := ResolveWithIncludes("go", "", nil)
		if err != nil {
			t.Fatalf("ResolveWithIncludes: %v", err)
		}
		if len(blueprints) != 2 {
			t.Fatalf("expected 2 blueprints (general + go), got %d", len(blueprints))
		}
		// Topological: includes first
		if blueprints[0].Name != "general" {
			t.Errorf("expected first blueprint to be 'general', got %q", blueprints[0].Name)
		}
		if blueprints[1].Name != "go" {
			t.Errorf("expected second blueprint to be 'go', got %q", blueprints[1].Name)
		}
	})

	t.Run("go-github-actions includes github-actions includes general", func(t *testing.T) {
		t.Parallel()
		blueprints, err := ResolveWithIncludes("go-github-actions", "", nil)
		if err != nil {
			t.Fatalf("ResolveWithIncludes: %v", err)
		}
		if len(blueprints) != 3 {
			t.Fatalf("expected 3 blueprints, got %d", len(blueprints))
		}
		if blueprints[0].Name != "general" {
			t.Errorf("expected first to be 'general', got %q", blueprints[0].Name)
		}
		if blueprints[1].Name != "github-actions" {
			t.Errorf("expected second to be 'github-actions', got %q", blueprints[1].Name)
		}
		if blueprints[2].Name != "go-github-actions" {
			t.Errorf("expected third to be 'go-github-actions', got %q", blueprints[2].Name)
		}
	})

	t.Run("deduplicates shared includes", func(t *testing.T) {
		t.Parallel()
		// Create two blueprints that both include "general"
		dir := t.TempDir()
		writeBlueprint(t, dir, "a", []string{"general"})
		writeBlueprint(t, dir, "parent", []string{"a", "general"})

		blueprints, err := ResolveWithIncludes("parent", dir, nil)
		if err != nil {
			t.Fatalf("ResolveWithIncludes: %v", err)
		}
		// general should appear exactly once
		count := 0
		for _, bp := range blueprints {
			if bp.Name == "general" {
				count++
			}
		}
		if count != 1 {
			t.Errorf("expected 'general' exactly once, got %d times", count)
		}
	})
}

func TestResolveWithIncludesCycleDetection(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// Create a → b → a cycle
	writeBlueprint(t, dir, "cycle-a", []string{"cycle-b"})
	writeBlueprint(t, dir, "cycle-b", []string{"cycle-a"})

	_, err := ResolveWithIncludes("cycle-a", dir, nil)
	if err == nil {
		t.Fatal("expected cycle detection error")
	}
	if !contains(err.Error(), "cycle") {
		t.Errorf("expected cycle error, got: %v", err)
	}
}

func TestAllEmbeddedBlueprintsValid(t *testing.T) {
	t.Parallel()
	blueprints, err := ListEmbedded()
	if err != nil {
		t.Fatalf("ListEmbedded: %v", err)
	}
	for _, bp := range blueprints {
		t.Run(bp.Name, func(t *testing.T) {
			t.Parallel()
			// Validate is already called by Load, but be explicit
			if err := bp.Validate(); err != nil {
				t.Errorf("validation failed: %v", err)
			}
		})
	}
}

func TestAllEmbeddedIncludesResolvable(t *testing.T) {
	t.Parallel()
	blueprints, err := ListEmbedded()
	if err != nil {
		t.Fatalf("ListEmbedded: %v", err)
	}
	for _, bp := range blueprints {
		t.Run(bp.Name, func(t *testing.T) {
			t.Parallel()
			_, err := ResolveWithIncludes(bp.Name, "", nil)
			if err != nil {
				t.Errorf("includes resolution failed: %v", err)
			}
		})
	}
}

func TestNoTopicCollisionsAcrossIncludes(t *testing.T) {
	t.Parallel()

	// For each blueprint, resolve with includes and check no topic appears twice
	blueprints, err := ListEmbedded()
	if err != nil {
		t.Fatalf("ListEmbedded: %v", err)
	}
	for _, bp := range blueprints {
		t.Run(bp.Name, func(t *testing.T) {
			t.Parallel()
			chain, err := ResolveWithIncludes(bp.Name, "", nil)
			if err != nil {
				t.Fatalf("resolve: %v", err)
			}
			seen := make(map[string]string) // topic → blueprint name
			for _, resolved := range chain {
				for _, d := range resolved.Decisions {
					if prev, ok := seen[d.Topic]; ok {
						t.Errorf("topic %q appears in both %q and %q", d.Topic, prev, resolved.Name)
					}
					seen[d.Topic] = resolved.Name
				}
			}
		})
	}
}

// helpers

func writeBlueprint(t *testing.T, dir, name string, includes []string) {
	t.Helper()
	bp := Blueprint{
		SchemaVersion: 1,
		Name:          name,
		DisplayName:   name,
		Description:   "test blueprint " + name,
		Version:       "0.0.1",
		Includes:      includes,
		Decisions: []BlueprintDecision{
			{Topic: name + "-topic", Decision: "d", Rationale: "r"},
		},
	}
	data, err := json.MarshalIndent(bp, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name+".json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

func searchSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
