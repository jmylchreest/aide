package grammar

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadIndexRejectsEscapingMarkersAtomically(t *testing.T) {
	for _, file := range []string{"../outside", `..\outside`, "/outside", `C:\outside`, `\\server\share`, ""} {
		t.Run(file, func(t *testing.T) {
			reg, err := NewPackRegistry()
			if err != nil {
				t.Fatal(err)
			}
			before := len(reg.ProjectMarkers())
			data, err := json.Marshal(PackIndex{ProjectMarkers: []ProjectMarker{{File: "valid-new-marker", Check: MarkerCheckFile, Label: "test", Kind: "module"}, {File: file, Check: MarkerCheckFile, Label: "bad", Kind: "module"}}})
			if err != nil {
				t.Fatal(err)
			}
			if err := reg.LoadIndex(data); err == nil {
				t.Fatalf("accepted nonlocal marker %q", file)
			}
			if got := len(reg.ProjectMarkers()); got != before {
				t.Fatalf("invalid index partially applied: %d != %d", got, before)
			}
		})
	}
	reg, err := NewPackRegistry()
	if err != nil {
		t.Fatal(err)
	}
	data := []byte(`{"project_markers":[{"file":"go.work","check":"sibling","sibling_of":"../go.mod"}]}`)
	if err := reg.LoadIndex(data); err == nil {
		t.Fatal("accepted escaping sibling marker")
	}
}

func TestMarkerCannotFollowSymlinkOutsideProject(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, []byte("marker"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "marker")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if MarkerExistsInProject(root, ProjectMarker{File: "marker", Check: MarkerCheckFile}) {
		t.Fatal("followed external marker symlink")
	}
}

func TestLoadPackRejectsMalformedPatterns(t *testing.T) {
	for _, data := range []string{
		`{"name":"go","entrypoints":{"symbols":[{"exclude":"["}]}}`,
		`{"name":"go","entrypoints":{"refs":[{"qualifier":"["}]}}`,
		`{"name":"go","entrypoints":{"file_patterns":[{"file_match":"["}]}}`,
		`{"name":"go","entrypoints":{"file_patterns":[{"file_match":"*.go","content":"["}]}}`,
		`{"name":"go","deadcode":{"test_file_patterns":["["]}}`,
	} {
		reg, err := NewPackRegistry()
		if err != nil {
			t.Fatal(err)
		}
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "pack.json"), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		if err := reg.LoadFromDir(dir); err == nil {
			t.Errorf("accepted invalid pack %s", data)
		}
		if !reg.Get("go").HasParser() {
			t.Fatal("invalid pack replaced embedded pack")
		}
	}
}
