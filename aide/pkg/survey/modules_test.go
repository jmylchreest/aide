package survey

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// fakeModulesSource serves a hand-built code index.
type fakeModulesSource struct {
	files    []ModuleFile
	refs     map[string][]ReferenceHit
	definers map[string][]string
}

func (f *fakeModulesSource) ListSourceFiles() ([]ModuleFile, error) { return f.files, nil }
func (f *fakeModulesSource) FileReferences(p string) ([]ReferenceHit, error) {
	return f.refs[p], nil
}
func (f *fakeModulesSource) DefiningFiles(sym string) ([]string, error) {
	return f.definers[sym], nil
}

// modulesFixture lays a two-package Go project on disk (the resolver probes
// real files) and mirrors it in a fake code index.
func modulesFixture(t *testing.T) (string, *fakeModulesSource) {
	t.Helper()
	root := t.TempDir()
	disk := map[string]string{
		"go.mod":          "module example.com/m\n",
		"a/one.go":        "package a\n",
		"a/two.go":        "package a\n",
		"b/three.go":      "package b\n",
		"b/three_test.go": "package b\n",
		"b/four.go":       "package b\n",
	}
	for rel, content := range disk {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}

	src := &fakeModulesSource{
		files: []ModuleFile{
			{Path: "a/one.go", Language: "go"},
			{Path: "a/two.go", Language: "go"},
			{Path: "b/three.go", Language: "go"},
			{Path: "b/three_test.go", Language: "go"},
			{Path: "b/four.go", Language: "go"},
		},
		refs: map[string][]ReferenceHit{
			// a/one.go imports package b and calls ThreeFunc — attribution
			// should edge it to b/three.go only, not the whole package. The
			// intra-module ties are stronger than the single cross edge, as
			// in a real codebase.
			"a/one.go": {
				{Kind: "import", Symbol: `"example.com/m/b"`},
				{Kind: "call", Symbol: "ThreeFunc"},
				{Kind: "call", Symbol: "TwoHelper"},
				{Kind: "call", Symbol: "TwoParse"},
				{Kind: "type_ref", Symbol: "TwoConfig"},
			},
			"a/two.go": {
				{Kind: "call", Symbol: "OneThing"},
				{Kind: "call", Symbol: "OneOther"},
				{Kind: "type_ref", Symbol: "OneState"},
			},
			"b/three.go": {
				{Kind: "call", Symbol: "FourUtil"},
				{Kind: "call", Symbol: "FourFmt"},
				{Kind: "type_ref", Symbol: "FourOpts"},
			},
			"b/three_test.go": {
				{Kind: "call", Symbol: "ThreeFunc"},
			},
			"b/four.go": {
				{Kind: "call", Symbol: "ThreeFunc"},
				{Kind: "call", Symbol: "ThreeRun"},
				{Kind: "type_ref", Symbol: "ThreeCfg"},
			},
		},
		definers: map[string][]string{
			"ThreeFunc": {"b/three.go"},
			"ThreeRun":  {"b/three.go"},
			"ThreeCfg":  {"b/three.go"},
			"FourUtil":  {"b/four.go"},
			"FourFmt":   {"b/four.go"},
			"FourOpts":  {"b/four.go"},
			"TwoHelper": {"a/two.go"},
			"TwoParse":  {"a/two.go"},
			"TwoConfig": {"a/two.go"},
			"OneThing":  {"a/one.go"},
			"OneOther":  {"a/one.go"},
			"OneState":  {"a/one.go"},
		},
	}
	return root, src
}

func TestRunModules(t *testing.T) {
	root, src := modulesFixture(t)

	result, err := RunModules(ModulesConfig{RootDir: root, Source: src})
	if err != nil {
		t.Fatalf("RunModules: %v", err)
	}

	if result.Files != 5 {
		t.Errorf("Files = %d, want 5", result.Files)
	}
	if result.ImportsTotal != 1 || result.ImportsResolved != 1 {
		t.Errorf("imports = %d/%d, want 1/1", result.ImportsResolved, result.ImportsTotal)
	}
	if len(result.Entries) != 2 {
		t.Fatalf("expected 2 module entries, got %d: %+v", len(result.Entries), result.Entries)
	}

	byName := map[string]*Entry{}
	for _, e := range result.Entries {
		if e.Analyzer != AnalyzerModules || e.Kind != KindModule {
			t.Errorf("entry %q has analyzer/kind %s/%s", e.Name, e.Analyzer, e.Kind)
		}
		byName[e.Name] = e
	}
	if byName["a"] == nil || byName["b"] == nil {
		t.Fatalf("expected modules 'a' and 'b', got %v", byName)
	}
	// The test file must be paired into its subject, not appear as a member.
	if members := byName["b"].Metadata["members"]; members == "" ||
		containsStr(members, "three_test.go") {
		t.Errorf("test file leaked into members: %s", members)
	}
}

func containsStr(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (func() bool {
		for i := 0; i+len(needle) <= len(haystack); i++ {
			if haystack[i:i+len(needle)] == needle {
				return true
			}
		}
		return false
	})()
}

func TestRunModulesDeterminismAndRemap(t *testing.T) {
	root, src := modulesFixture(t)

	first, err := RunModules(ModulesConfig{RootDir: root, Source: src})
	if err != nil {
		t.Fatalf("RunModules: %v", err)
	}
	again, err := RunModules(ModulesConfig{RootDir: root, Source: src})
	if err != nil {
		t.Fatalf("RunModules again: %v", err)
	}
	if !reflect.DeepEqual(first.Entries, again.Entries) {
		t.Errorf("entries differ across identical runs:\n%+v\nvs\n%+v", first.Entries, again.Entries)
	}

	// Swap the previous IDs and re-run: community IDs must follow.
	previous := map[string]int{}
	for _, e := range first.Entries {
		swapped := map[string]string{"0": "1", "1": "0"}[e.Metadata["community_id"]]
		fakePrev := *e
		fakePrev.Metadata = map[string]string{
			"community_id": swapped,
			"members":      e.Metadata["members"],
		}
		for m, id := range PreviousAssignmentFromEntries([]*Entry{&fakePrev}) {
			previous[m] = id
		}
	}
	remapped, err := RunModules(ModulesConfig{RootDir: root, Source: src, Previous: previous})
	if err != nil {
		t.Fatalf("RunModules remap: %v", err)
	}
	for _, e := range remapped.Entries {
		var origID string
		for _, fe := range first.Entries {
			if fe.Name == e.Name {
				origID = fe.Metadata["community_id"]
			}
		}
		want := map[string]string{"0": "1", "1": "0"}[origID]
		if e.Metadata["community_id"] != want {
			t.Errorf("module %q id = %s, want %s (following previous assignment)",
				e.Name, e.Metadata["community_id"], want)
		}
	}
}

func TestDiffModules(t *testing.T) {
	mk := func(id, label string, members ...string) *Entry {
		m, _ := json.Marshal(members)
		return &Entry{
			Analyzer: AnalyzerModules,
			Kind:     KindModule,
			Name:     label,
			Metadata: map[string]string{"community_id": id, "members": string(m)},
		}
	}

	prev := []*Entry{
		mk("0", "auth", "auth/a.go", "auth/b.go"),
		mk("1", "store", "store/s.go", "store/t.go"),
		mk("2", "web", "web/w.go"),
	}
	next := []*Entry{
		mk("0", "auth", "auth/a.go", "auth/b.go", "store/t.go"), // t.go moved in
		mk("1", "storage", "store/s.go"),                        // renamed
		mk("3", "cli", "cli/c.go"),                              // new; id 2 (web) dissolved
	}

	d := DiffModules(prev, next)
	if len(d.New) != 1 || d.New[0] != "cli" {
		t.Errorf("New = %v, want [cli]", d.New)
	}
	if len(d.Dissolved) != 1 || d.Dissolved[0] != "web" {
		t.Errorf("Dissolved = %v, want [web]", d.Dissolved)
	}
	if len(d.Renamed) != 1 || d.Renamed[0] != [2]string{"store", "storage"} {
		t.Errorf("Renamed = %v", d.Renamed)
	}
	if d.Moved != 1 {
		t.Errorf("Moved = %d, want 1 (store/t.go)", d.Moved)
	}
	if s := d.Summary(); s == "" {
		t.Error("Summary should be non-empty for a changed map")
	}

	if s := DiffModules(prev, prev).Summary(); s != "" {
		t.Errorf("identical runs should produce empty summary, got %q", s)
	}
}

func TestRunModulesRequiresSource(t *testing.T) {
	if _, err := RunModules(ModulesConfig{RootDir: t.TempDir()}); err == nil {
		t.Error("expected error without a code index source")
	}
}
