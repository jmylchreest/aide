package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/config"
	"github.com/jmylchreest/aide/aide/pkg/contextshare"
	"github.com/jmylchreest/aide/aide/pkg/memory"
)

// writeTree materialises a minimal per-record .aide/shared/ tree (manifest, one
// decision version, two memories) so the import preview has records to count.
func writeTree(t *testing.T, sharedDir string) {
	t.Helper()
	created := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)

	d := &memory.Decision{Topic: "auth", Decision: "JWT", CreatedAt: created}
	dPath := contextshare.DecisionPath(sharedDir, d)
	if err := os.MkdirAll(filepath.Dir(dPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dPath, contextshare.MarshalDecision(d), 0o644); err != nil {
		t.Fatal(err)
	}

	mems := []*memory.Memory{
		{ID: "01PROJTREEAAAAAAAAAAAAAAAA", Category: "pattern", Content: "tree project", Tags: []string{"project:foo"}, CreatedAt: created},
		{ID: "01GLOBALTREEAAAAAAAAAAAAAA", Category: "pattern", Content: "tree global", Tags: []string{"scope:global"}, CreatedAt: created},
	}
	for _, m := range mems {
		mPath := contextshare.MemoryPath(sharedDir, m.ID)
		if err := os.MkdirAll(filepath.Dir(mPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(mPath, contextshare.MarshalMemory(m), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if err := contextshare.WriteManifest(sharedDir, created); err != nil {
		t.Fatal(err)
	}
}

// seedShowFixture returns the canonical fixture for the show tests: 3 decisions
// and 5 memories (one scope:global, one session:x, three project:foo). Under
// the default policy the memory export filter excludes scope:global and
// session:*, so 3 of the 5 memories are publishable and 2 are excluded.
func seedShowFixture() ([]*memory.Decision, []*memory.Memory) {
	created := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)
	decisions := []*memory.Decision{
		{Topic: "auth", Decision: "JWT", CreatedAt: created},
		{Topic: "db", Decision: "postgres", CreatedAt: created},
		{Topic: "cache", Decision: "redis", CreatedAt: created},
	}
	memories := []*memory.Memory{
		{ID: "01PROJ1AAAAAAAAAAAAAAAAAAA", Category: "pattern", Content: "a", Tags: []string{"project:foo"}, CreatedAt: created},
		{ID: "01PROJ2AAAAAAAAAAAAAAAAAAA", Category: "pattern", Content: "b", Tags: []string{"project:foo"}, CreatedAt: created},
		{ID: "01PROJ3AAAAAAAAAAAAAAAAAAA", Category: "gotcha", Content: "c", Tags: []string{"project:foo"}, CreatedAt: created},
		{ID: "01GLOBALAAAAAAAAAAAAAAAAAA", Category: "pattern", Content: "g", Tags: []string{"scope:global"}, CreatedAt: created},
		{ID: "01SESSIONAAAAAAAAAAAAAAAAA", Category: "learning", Content: "s", Tags: []string{"session:x"}, CreatedAt: created},
	}
	return decisions, memories
}

func TestBuildShareViewDefaultPolicy(t *testing.T) {
	decisions, memories := seedShowFixture()
	// Default policy: no .aide/shared/ tree, so no import preview.
	view := buildShareView(config.ShareConfig{}, "/proj/.aide/shared", decisions, memories)

	// Decisions: export+import ON, unfiltered (include [*], no exclude).
	if !view.Decisions.ExportEnabled || !view.Decisions.ImportEnabled {
		t.Errorf("decisions should be ON/ON by default, got export=%v import=%v",
			view.Decisions.ExportEnabled, view.Decisions.ImportEnabled)
	}
	if got := view.Decisions.ExportFilter.Include; len(got) != 1 || got[0] != "*" {
		t.Errorf("decision export include = %v, want [*]", got)
	}
	if got := view.Decisions.ExportFilter.Exclude; len(got) != 0 {
		t.Errorf("decision export exclude = %v, want none", got)
	}
	if dp := view.Decisions.Preview; dp.Total != 3 || dp.Published != 3 || dp.Excluded != 0 {
		t.Errorf("decision preview = %+v, want total=3 published=3 excluded=0", dp)
	}

	// Memories: export+import OFF, default exclude scope:global, session:*.
	if view.Memories.ExportEnabled || view.Memories.ImportEnabled {
		t.Errorf("memories should be OFF/OFF by default, got export=%v import=%v",
			view.Memories.ExportEnabled, view.Memories.ImportEnabled)
	}
	wantExcl := []string{"scope:global", "session:*"}
	if got := view.Memories.ExportFilter.Exclude; len(got) != 2 || got[0] != wantExcl[0] || got[1] != wantExcl[1] {
		t.Errorf("memory export exclude = %v, want %v", got, wantExcl)
	}

	// The export preview is computed even though export is OFF ("if enabled").
	mp := view.Memories.Preview
	if mp.Total != 5 || mp.Published != 3 || mp.Excluded != 2 {
		t.Errorf("memory preview = %+v, want total=5 published=3 excluded=2", mp)
	}

	byPattern := map[string]int{}
	for _, pt := range mp.ByPattern {
		byPattern[pt.Pattern] = pt.Count
	}
	if byPattern["scope:global"] != 1 {
		t.Errorf("expected 1 memory excluded by scope:global, got %d", byPattern["scope:global"])
	}
	if byPattern["session:*"] != 1 {
		t.Errorf("expected 1 memory excluded by session:*, got %d", byPattern["session:*"])
	}

	// No tree on disk → no import preview.
	if view.Import != nil {
		t.Errorf("import preview should be nil without a .aide/shared/ tree, got %+v", view.Import)
	}
}

// captureStdout runs fn and returns whatever it wrote to os.Stdout.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		data, _ := io.ReadAll(r)
		done <- string(data)
	}()
	fn()
	w.Close()
	os.Stdout = old
	return <-done
}

func TestCmdShareShowJSON(t *testing.T) {
	config.Set(&config.Config{}) // explicit defaults
	t.Cleanup(func() { config.Set(&config.Config{}) })

	dbPath, _ := newShareProject(t)
	decisions, memories := seedShowFixture()
	withBackend(t, dbPath, func(b *Backend) {
		for _, d := range decisions {
			if err := b.Store().SetDecision(d); err != nil {
				t.Fatalf("SetDecision: %v", err)
			}
		}
		for _, m := range memories {
			if err := b.Store().AddMemory(m); err != nil {
				t.Fatalf("AddMemory: %v", err)
			}
		}
	})

	out := captureStdout(t, func() {
		if err := cmdShareShow(dbPath, []string{"--json"}); err != nil {
			t.Fatalf("cmdShareShow: %v", err)
		}
	})

	var got shareView
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, out)
	}

	if !got.Decisions.ExportEnabled || !got.Decisions.ImportEnabled {
		t.Errorf("decisions ON/ON expected, got export=%v import=%v",
			got.Decisions.ExportEnabled, got.Decisions.ImportEnabled)
	}
	if dp := got.Decisions.Preview; dp.Total != 3 || dp.Published != 3 {
		t.Errorf("decision preview = %+v, want total=3 published=3", dp)
	}
	if got.Memories.ExportEnabled || got.Memories.ImportEnabled {
		t.Errorf("memories OFF/OFF expected, got export=%v import=%v",
			got.Memories.ExportEnabled, got.Memories.ImportEnabled)
	}
	mp := got.Memories.Preview
	if mp.Total != 5 || mp.Published != 3 || mp.Excluded != 2 {
		t.Errorf("memory preview = %+v, want total=5 published=3 excluded=2", mp)
	}
	byPattern := map[string]int{}
	for _, pt := range mp.ByPattern {
		byPattern[pt.Pattern] = pt.Count
	}
	if byPattern["scope:global"] != 1 || byPattern["session:*"] != 1 {
		t.Errorf("by_pattern = %v, want scope:global=1 session:*=1", byPattern)
	}
}

// With memory export ON the default filter still excludes scope:global and
// session:*, so the published count is unchanged but the policy reads ON.
func TestBuildShareViewMemoryExportOn(t *testing.T) {
	yes := true
	share := config.ShareConfig{Memories: config.ShareTypePolicy{Export: &yes}}
	decisions, memories := seedShowFixture()
	view := buildShareView(share, "/proj/.aide/shared", decisions, memories)

	if !view.Memories.ExportEnabled {
		t.Error("memory export should be ON")
	}
	if mp := view.Memories.Preview; mp.Published != 3 || mp.Excluded != 2 {
		t.Errorf("memory preview = %+v, want published=3 excluded=2", mp)
	}
}

// A custom exclude glob attributes excluded memories to that pattern, exercising
// the per-pattern tally for a non-default filter.
func TestBuildShareViewCustomExcludeTally(t *testing.T) {
	share := config.ShareConfig{
		Memories: config.ShareTypePolicy{
			ExportFilter: config.ShareFilter{Exclude: []string{"category:learning", "project:*"}},
		},
	}
	_, memories := seedShowFixture()
	view := buildShareView(share, "/proj/.aide/shared", memories2decisions(), memories)

	mp := view.Memories.Preview
	// 3 project:foo + 1 session:x (session:x also has no project tag) excluded by
	// project:* / category:learning; the scope:global memory passes (no project,
	// not learning). Attribution is to the FIRST matching exclude in list order.
	byPattern := map[string]int{}
	for _, pt := range mp.ByPattern {
		byPattern[pt.Pattern] = pt.Count
	}
	// 3 project:foo memories → project:* (category:learning listed first but they
	// are pattern/gotcha, not learning). The session:x learning memory → matches
	// category:learning first.
	if byPattern["project:*"] != 3 {
		t.Errorf("expected 3 excluded by project:*, got %d (tally %v)", byPattern["project:*"], byPattern)
	}
	if byPattern["category:learning"] != 1 {
		t.Errorf("expected 1 excluded by category:learning, got %d (tally %v)", byPattern["category:learning"], byPattern)
	}
}

// memories2decisions is a tiny helper so the custom-exclude test can reuse the
// fixture decisions without re-seeding.
func memories2decisions() []*memory.Decision {
	d, _ := seedShowFixture()
	return d
}

// When a per-record tree exists, show includes an import preview gated by the
// import filter. Default memory import excludes scope:global and session:*.
func TestCmdShareShowImportPreview(t *testing.T) {
	config.Set(&config.Config{})
	t.Cleanup(func() { config.Set(&config.Config{}) })

	dbPath, tmp := newShareProject(t)
	sharedDir := filepath.Join(tmp, ".aide", "shared")

	// Build a minimal per-record tree directly with the contextshare writers so
	// the import preview has something to enumerate. cmdShareShow itself never
	// writes here — this is fixture setup.
	writeTree(t, sharedDir)

	out := captureStdout(t, func() {
		if err := cmdShareShow(dbPath, []string{"--json"}); err != nil {
			t.Fatalf("cmdShareShow: %v", err)
		}
	})

	var got shareView
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, out)
	}
	if got.Import == nil {
		t.Fatal("expected import preview when a per-record tree exists")
	}
	// One decision version, accepted (decisions unfiltered).
	if d := got.Import.Decisions; d.Total != 1 || d.Accepted != 1 {
		t.Errorf("import decisions = %+v, want total=1 accepted=1", d)
	}
	// Two memory files: one project:foo (accepted), one scope:global (excluded).
	if m := got.Import.Memories; m.Total != 2 || m.Accepted != 1 || m.Excluded != 1 {
		t.Errorf("import memories = %+v, want total=2 accepted=1 excluded=1", m)
	}
}
