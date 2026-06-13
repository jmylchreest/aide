package config

import "testing"

// schemaMap flattens Schema() into a key→kind lookup for the assertions below.
func schemaMap(t *testing.T) map[string]FieldKind {
	t.Helper()
	out := map[string]FieldKind{}
	for _, fi := range Schema() {
		if _, dup := out[fi.Key]; dup {
			t.Errorf("Schema() has duplicate key %q", fi.Key)
		}
		out[fi.Key] = fi.Kind
	}
	return out
}

func TestSchemaKinds(t *testing.T) {
	m := schemaMap(t)

	cases := []struct {
		key  string
		kind FieldKind
	}{
		// Top-level scalars.
		{"mode", KindString},
		{"force_init", KindBool},
		{"project_root", KindString},
		{"index_non_vcs", KindBool},
		{"index_workers", KindInt},

		// Nested scalars across several sections.
		{"code.watch", KindBool},
		{"code.watch_paths", KindString},
		{"pprof.enable", KindBool},
		{"grammar.url", KindString},
		{"memory.scoring_enabled", KindBool},
		{"cleanup.observe_max_age", KindString},
		{"cleanup.enabled", KindBool},

		// Reflect repetition nesting (struct within struct).
		{"reflect.enabled", KindBool},
		{"reflect.repetition.min_count", KindInt},
		{"reflect.repetition.window_minutes", KindInt},
		{"reflect.repetition.ignore_commands", KindStringSlice},

		// Share *bool gates and list filters.
		{"share.auto_import", KindBool},
		{"share.decisions.export", KindPtrBool},
		{"share.decisions.import", KindPtrBool},
		{"share.memories.export", KindPtrBool},
		{"share.memories.import", KindPtrBool},
		{"share.memories.export_filter.include", KindStringSlice},
		{"share.memories.export_filter.exclude", KindStringSlice},
		{"share.decisions.import_filter.exclude", KindStringSlice},
	}

	for _, c := range cases {
		got, ok := m[c.key]
		if !ok {
			t.Errorf("Schema() missing key %q", c.key)
			continue
		}
		if got != c.kind {
			t.Errorf("Schema()[%q] kind = %v, want %v", c.key, got, c.kind)
		}
	}
}

func TestLookup(t *testing.T) {
	fi, ok := Lookup("share.decisions.export")
	if !ok {
		t.Fatal("Lookup should find share.decisions.export")
	}
	if fi.Kind != KindPtrBool {
		t.Errorf("share.decisions.export kind = %v, want PtrBool", fi.Kind)
	}
	if _, ok := Lookup("does.not.exist"); ok {
		t.Error("Lookup should not find an unknown key")
	}
}

func TestResolveSharesBuildKoanf(t *testing.T) {
	// Resolve must layer the same sources as Load: an env override wins over the
	// defaults confmap, confirming both paths share buildKoanf.
	isolateHome(t)
	t.Setenv("AIDE_MODE", "autopilot")
	k, err := Resolve("")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got := k.String("mode"); got != "autopilot" {
		t.Errorf("Resolve mode = %q, want autopilot", got)
	}
	if !k.Bool("cleanup.enabled") {
		t.Error("Resolve should carry the cleanup.enabled=true default")
	}
}
