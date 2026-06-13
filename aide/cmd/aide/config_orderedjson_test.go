package main

import (
	"strings"
	"testing"
)

// TestOrderedConfig_PatchPreservesForeignKeys is the core guarantee: editing
// one key must not reorder or disturb keys other tools wrote into the same file.
func TestOrderedConfig_PatchPreservesForeignKeys(t *testing.T) {
	// A file whose keys are NOT alphabetical and contain unrelated tools' data.
	original := `{
  "tiers": {
    "fast": "Cheapest/fastest model",
    "smart": "Most capable model"
  },
  "aliases": {
    "opus": "smart",
    "haiku": "fast"
  },
  "hud": {
    "enabled": true,
    "elements": [
      "mode",
      "model"
    ]
  }
}`

	m, err := decodeOrderedJSON([]byte(original))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	// Set a brand-new aide key in a new section.
	setNested(m, []string{"maintenance", "compact_on_exit"}, false)

	out, err := encodeOrderedJSON(m)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	got := string(out)

	// Every foreign key keeps its original order; the new section is appended.
	want := `{
  "tiers": {
    "fast": "Cheapest/fastest model",
    "smart": "Most capable model"
  },
  "aliases": {
    "opus": "smart",
    "haiku": "fast"
  },
  "hud": {
    "enabled": true,
    "elements": [
      "mode",
      "model"
    ]
  },
  "maintenance": {
    "compact_on_exit": false
  }
}`
	if got != want {
		t.Fatalf("patched output mismatch.\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

// TestOrderedConfig_UpdateInPlace verifies updating an existing key changes only
// its value and keeps its position (no reorder).
func TestOrderedConfig_UpdateInPlace(t *testing.T) {
	m, err := decodeOrderedJSON([]byte(`{
  "zebra": "first",
  "alpha": "second"
}`))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	setNested(m, []string{"zebra"}, "changed")
	out, _ := encodeOrderedJSON(m)
	want := `{
  "zebra": "changed",
  "alpha": "second"
}`
	if string(out) != want {
		t.Fatalf("got:\n%s\nwant:\n%s", out, want)
	}
}

// TestOrderedConfig_UnsetPrunesEmptyParents verifies removing the last key in a
// section drops the now-empty section, leaving the rest untouched.
func TestOrderedConfig_UnsetPrunesEmptyParents(t *testing.T) {
	m, err := decodeOrderedJSON([]byte(`{
  "keep": {
    "a": 1
  },
  "drop": {
    "only": true
  }
}`))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	deleteNested(m, []string{"drop", "only"})
	out, _ := encodeOrderedJSON(m)
	want := `{
  "keep": {
    "a": 1
  }
}`
	if string(out) != want {
		t.Fatalf("got:\n%s\nwant:\n%s", out, want)
	}
}

// TestOrderedConfig_RoundTripNoEdit proves a decode→encode with no change is a
// no-op for an already-ordered file (so re-running set on an unrelated key never
// churns the file just by touching it).
func TestOrderedConfig_RoundTripNoEdit(t *testing.T) {
	src := `{
  "b": 1,
  "a": [
    "x",
    "y"
  ]
}`
	m, err := decodeOrderedJSON([]byte(src))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	out, _ := encodeOrderedJSON(m)
	if string(out) != src {
		t.Fatalf("round-trip changed file.\ngot:\n%s\nwant:\n%s", out, src)
	}
}

// TestOrderedConfig_SliceValueExpands verifies a freshly-set string slice is
// rendered expanded (one element per line), matching how decoded arrays look.
func TestOrderedConfig_SliceValueExpands(t *testing.T) {
	m := newOrderedMap()
	setNested(m, []string{"share", "memories", "export_filter", "exclude"}, []string{"scope:global", "session:*"})
	out, _ := encodeOrderedJSON(m)
	if !strings.Contains(string(out), "\"scope:global\",\n") {
		t.Fatalf("expected expanded slice, got:\n%s", out)
	}
}
