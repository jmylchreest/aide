package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/memory"
)

// =============================================================================
// Helper
// =============================================================================

// setupShareTest creates a temp dir with a bolt DB and returns a Backend.
func setupShareTest(t *testing.T) (b *Backend, tmpDir string, cleanup func()) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "aide-share-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	// Mimic the real layout: <root>/.aide/memory/memory.db
	memDir := filepath.Join(tmpDir, ".aide", "memory")
	if err := os.MkdirAll(memDir, 0o755); err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to create memory dir: %v", err)
	}

	dbPath := filepath.Join(memDir, "memory.db")
	b, err = NewBackend(dbPath)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to create backend: %v", err)
	}

	cleanup = func() {
		b.Close()
		os.RemoveAll(tmpDir)
	}
	return b, tmpDir, cleanup
}

// =============================================================================
// Markdown Generation
// =============================================================================

func TestWriteDecisionMarkdown(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "aide-share-md-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	d := &memory.Decision{
		Topic:      "auth-strategy",
		Decision:   "JWT with refresh tokens",
		Rationale:  "Stateless, mobile-friendly",
		Details:    "Use RS256 signing",
		DecidedBy:  "agent-1",
		References: []string{"https://jwt.io"},
		CreatedAt:  time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
	}

	path := filepath.Join(tmpDir, "auth-strategy.md")
	if err := writeDecisionMarkdown(path, d); err != nil {
		t.Fatalf("writeDecisionMarkdown: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read: %v", err)
	}
	s := string(content)

	// Verify frontmatter fields
	if !strings.Contains(s, "topic: auth-strategy") {
		t.Error("missing topic in frontmatter")
	}
	if !strings.Contains(s, "decision: JWT with refresh tokens") {
		t.Error("missing decision in frontmatter")
	}
	if !strings.Contains(s, "decided_by: agent-1") {
		t.Error("missing decided_by")
	}
	if !strings.Contains(s, "date: 2026-01-15") {
		t.Error("missing date")
	}
	if !strings.Contains(s, "- https://jwt.io") {
		t.Error("missing reference")
	}

	// Verify body sections
	if !strings.Contains(s, "## Rationale") {
		t.Error("missing Rationale section")
	}
	if !strings.Contains(s, "Stateless, mobile-friendly") {
		t.Error("missing rationale content")
	}
	if !strings.Contains(s, "## Details") {
		t.Error("missing Details section")
	}
	if !strings.Contains(s, "Use RS256 signing") {
		t.Error("missing details content")
	}
}

func TestWriteMemoriesMarkdown(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "aide-share-mems-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	mems := []*memory.Memory{
		{
			ID:        "MEM001",
			Category:  memory.CategoryLearning,
			Content:   "Auth middleware lives at src/auth.ts",
			Tags:      []string{"project:myapp", "scope:global"},
			CreatedAt: time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC),
		},
		{
			ID:        "MEM002",
			Category:  memory.CategoryLearning,
			Content:   "Use vitest for testing",
			Tags:      []string{"project:myapp"},
			CreatedAt: time.Date(2026, 1, 11, 0, 0, 0, 0, time.UTC),
		},
	}

	path := filepath.Join(tmpDir, "learning.md")
	if err := writeMemoriesMarkdown(path, memory.CategoryLearning, mems); err != nil {
		t.Fatalf("writeMemoriesMarkdown: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := string(content)

	if !strings.Contains(s, "category: learning") {
		t.Error("missing category in frontmatter")
	}
	if !strings.Contains(s, "count: 2") {
		t.Error("missing count in frontmatter")
	}
	if !strings.Contains(s, "aide:id=MEM001") {
		t.Error("missing MEM001 metadata")
	}
	if !strings.Contains(s, "aide:id=MEM002") {
		t.Error("missing MEM002 metadata")
	}
	if !strings.Contains(s, "tags=project:myapp,scope:global") {
		t.Error("missing tags for MEM001")
	}
	if !strings.Contains(s, "Auth middleware lives at src/auth.ts") {
		t.Error("missing MEM001 content")
	}
}

// =============================================================================
// Markdown Parsing
// =============================================================================

func TestParseDecisionMarkdownRoundTrip(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "aide-share-parse-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	original := &memory.Decision{
		Topic:      "testing",
		Decision:   "Use vitest for unit tests",
		Rationale:  "Fast and TypeScript-native",
		Details:    "Configured with coverage thresholds",
		DecidedBy:  "dev-1",
		References: []string{"https://vitest.dev", "https://example.com"},
		CreatedAt:  time.Date(2026, 2, 5, 0, 0, 0, 0, time.UTC),
	}

	path := filepath.Join(tmpDir, "testing.md")
	if err := writeDecisionMarkdown(path, original); err != nil {
		t.Fatalf("write: %v", err)
	}

	parsed, err := parseDecisionMarkdown(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if parsed.Topic != original.Topic {
		t.Errorf("topic: got %q, want %q", parsed.Topic, original.Topic)
	}
	if parsed.Decision != original.Decision {
		t.Errorf("decision: got %q, want %q", parsed.Decision, original.Decision)
	}
	if parsed.Rationale != original.Rationale {
		t.Errorf("rationale: got %q, want %q", parsed.Rationale, original.Rationale)
	}
	if parsed.Details != original.Details {
		t.Errorf("details: got %q, want %q", parsed.Details, original.Details)
	}
	if parsed.DecidedBy != original.DecidedBy {
		t.Errorf("decided_by: got %q, want %q", parsed.DecidedBy, original.DecidedBy)
	}
	if len(parsed.References) != len(original.References) {
		t.Errorf("references count: got %d, want %d", len(parsed.References), len(original.References))
	}
	for i, ref := range parsed.References {
		if ref != original.References[i] {
			t.Errorf("reference[%d]: got %q, want %q", i, ref, original.References[i])
		}
	}
}

func TestParseMemoriesMarkdownRoundTrip(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "aide-share-memparse-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	originals := []*memory.Memory{
		{
			ID:        "RT001",
			Category:  memory.CategoryLearning,
			Content:   "First memory content",
			Tags:      []string{"project:test", "scope:global"},
			CreatedAt: time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC),
		},
		{
			ID:        "RT002",
			Category:  memory.CategoryLearning,
			Content:   "Second memory content\nwith multiple lines",
			Tags:      []string{"project:test"},
			CreatedAt: time.Date(2026, 1, 11, 0, 0, 0, 0, time.UTC),
		},
	}

	path := filepath.Join(tmpDir, "learning.md")
	if err := writeMemoriesMarkdown(path, memory.CategoryLearning, originals); err != nil {
		t.Fatalf("write: %v", err)
	}

	parsed, err := parseMemoriesMarkdown(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if len(parsed) != len(originals) {
		t.Fatalf("count: got %d, want %d", len(parsed), len(originals))
	}

	for i, m := range parsed {
		if m.ID != originals[i].ID {
			t.Errorf("[%d] id: got %q, want %q", i, m.ID, originals[i].ID)
		}
		if m.Content != originals[i].Content {
			t.Errorf("[%d] content: got %q, want %q", i, m.Content, originals[i].Content)
		}
		if m.Category != originals[i].Category {
			t.Errorf("[%d] category: got %q, want %q", i, m.Category, originals[i].Category)
		}
		if len(m.Tags) != len(originals[i].Tags) {
			t.Errorf("[%d] tags count: got %d, want %d", i, len(m.Tags), len(originals[i].Tags))
		}
	}
}

// =============================================================================
// Stale File Cleanup
// =============================================================================

func TestRemoveStaleFiles(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "aide-share-stale-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create some files
	for _, name := range []string{"keep.md", "stale.md", "also-stale.md", "notmd.txt"} {
		os.WriteFile(filepath.Join(tmpDir, name), []byte("test"), 0644)
	}

	expected := map[string]bool{"keep.md": true}
	if err := removeStaleFiles(tmpDir, expected); err != nil {
		t.Fatalf("removeStaleFiles: %v", err)
	}

	entries, _ := os.ReadDir(tmpDir)
	names := map[string]bool{}
	for _, e := range entries {
		names[e.Name()] = true
	}

	if !names["keep.md"] {
		t.Error("keep.md should have been preserved")
	}
	if names["stale.md"] {
		t.Error("stale.md should have been removed")
	}
	if names["also-stale.md"] {
		t.Error("also-stale.md should have been removed")
	}
	if !names["notmd.txt"] {
		t.Error("notmd.txt (non-.md) should have been preserved")
	}
}

func TestRemoveStaleFilesEmptyExpected(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "aide-share-stale-empty-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	os.WriteFile(filepath.Join(tmpDir, "old.md"), []byte("test"), 0644)

	// Empty expected set = remove all .md files
	if err := removeStaleFiles(tmpDir, map[string]bool{}); err != nil {
		t.Fatalf("removeStaleFiles: %v", err)
	}

	entries, _ := os.ReadDir(tmpDir)
	if len(entries) != 0 {
		t.Errorf("expected empty dir, got %d entries", len(entries))
	}
}

func TestRemoveStaleFilesNonexistentDir(t *testing.T) {
	// Should not error on non-existent directory
	err := removeStaleFiles("/tmp/does-not-exist-aide-test", map[string]bool{})
	if err != nil {
		t.Errorf("expected nil error for non-existent dir, got: %v", err)
	}
}

// =============================================================================
// Shareable Memory Filter
// =============================================================================

func TestIsShareableMemory(t *testing.T) {
	tests := []struct {
		name     string
		memory   *memory.Memory
		expected bool
	}{
		{
			name:     "scope:global tag",
			memory:   &memory.Memory{Category: memory.CategoryLearning, Tags: []string{"scope:global"}},
			expected: true,
		},
		{
			name:     "project tag",
			memory:   &memory.Memory{Category: memory.CategoryLearning, Tags: []string{"project:myapp"}},
			expected: true,
		},
		{
			name:     "gotcha category always shared",
			memory:   &memory.Memory{Category: "gotcha"},
			expected: true,
		},
		{
			name:     "pattern category always shared",
			memory:   &memory.Memory{Category: "pattern"},
			expected: true,
		},
		{
			name:     "decision category always shared",
			memory:   &memory.Memory{Category: "decision"},
			expected: true,
		},
		{
			name:     "session-only learning not shared",
			memory:   &memory.Memory{Category: memory.CategoryLearning, Tags: []string{"session:abc123"}},
			expected: false,
		},
		{
			name:     "no tags learning not shared",
			memory:   &memory.Memory{Category: memory.CategoryLearning},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isShareableMemory(tt.memory)
			if got != tt.expected {
				t.Errorf("isShareableMemory = %v, want %v", got, tt.expected)
			}
		})
	}
}

// =============================================================================
// Full Export/Import Round-Trip
// =============================================================================

func TestShareExportImportDecisionsRoundTrip(t *testing.T) {
	b, tmpDir, cleanup := setupShareTest(t)
	defer cleanup()

	// Add decisions
	_, err := b.SetDecision("auth", "JWT", "Stateless", "Use RS256", "agent-1", nil)
	if err != nil {
		t.Fatalf("SetDecision: %v", err)
	}
	_, err = b.SetDecision("db", "PostgreSQL", "ACID compliance", "", "", nil)
	if err != nil {
		t.Fatalf("SetDecision: %v", err)
	}

	sharedDir := filepath.Join(tmpDir, ".aide", "shared")

	// Export
	count, err := shareExportDecisions(b, sharedDir)
	if err != nil {
		t.Fatalf("shareExportDecisions: %v", err)
	}
	if count != 2 {
		t.Errorf("exported %d decisions, want 2", count)
	}

	// Verify files exist
	if _, err := os.Stat(filepath.Join(sharedDir, "decisions", "auth.md")); err != nil {
		t.Error("auth.md not created")
	}
	if _, err := os.Stat(filepath.Join(sharedDir, "decisions", "db.md")); err != nil {
		t.Error("db.md not created")
	}

	// Clear decisions from bolt and re-import
	b.ClearDecisions()

	imported, skipped, err := shareImportDecisions(b, sharedDir, false)
	if err != nil {
		t.Fatalf("shareImportDecisions: %v", err)
	}
	if imported != 2 {
		t.Errorf("imported %d, want 2", imported)
	}
	if skipped != 0 {
		t.Errorf("skipped %d, want 0", skipped)
	}

	// Verify decisions are back
	d, err := b.GetDecision("auth")
	if err != nil {
		t.Fatalf("GetDecision auth: %v", err)
	}
	if d.Decision != "JWT" {
		t.Errorf("auth decision: got %q, want %q", d.Decision, "JWT")
	}
}

func TestShareImportDecisionsIdempotent(t *testing.T) {
	b, tmpDir, cleanup := setupShareTest(t)
	defer cleanup()

	_, _ = b.SetDecision("test", "value", "", "", "", nil)

	sharedDir := filepath.Join(tmpDir, ".aide", "shared")
	shareExportDecisions(b, sharedDir)

	// Import when data already exists — should skip
	imported, skipped, err := shareImportDecisions(b, sharedDir, false)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if imported != 0 {
		t.Errorf("imported %d, want 0 (idempotent)", imported)
	}
	if skipped != 1 {
		t.Errorf("skipped %d, want 1", skipped)
	}
}

func TestShareExportCleansStaleDecisions(t *testing.T) {
	b, tmpDir, cleanup := setupShareTest(t)
	defer cleanup()

	sharedDir := filepath.Join(tmpDir, ".aide", "shared")

	// Export two decisions
	b.SetDecision("keep", "yes", "", "", "", nil)
	b.SetDecision("remove", "later", "", "", "", nil)
	shareExportDecisions(b, sharedDir)

	// Verify both files exist
	if _, err := os.Stat(filepath.Join(sharedDir, "decisions", "remove.md")); err != nil {
		t.Fatal("remove.md should exist after first export")
	}

	// Delete one decision from bolt
	b.DeleteDecision("remove")

	// Re-export — stale file should be removed
	count, err := shareExportDecisions(b, sharedDir)
	if err != nil {
		t.Fatalf("second export: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 decision exported, got %d", count)
	}

	if _, err := os.Stat(filepath.Join(sharedDir, "decisions", "remove.md")); !os.IsNotExist(err) {
		t.Error("remove.md should have been cleaned up")
	}
	if _, err := os.Stat(filepath.Join(sharedDir, "decisions", "keep.md")); err != nil {
		t.Error("keep.md should still exist")
	}
}

// =============================================================================
// Helpers
// =============================================================================

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"auth-strategy", "auth-strategy"},
		{"simple", "simple"},
		{"has spaces", "has-spaces"},
		{"has/slashes", "has-slashes"},
		{"special!@#chars", "special-chars"},
		{"", "unnamed"},
		{"---", "unnamed"},
	}

	for _, tt := range tests {
		got := sanitizeFilename(tt.input)
		if got != tt.want {
			t.Errorf("sanitizeFilename(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestYamlEscapeUnescape(t *testing.T) {
	tests := []struct {
		input string
	}{
		{"simple value"},
		{"value with: colon"},
		{`value with "quotes"`},
		{"value with #hash"},
	}

	for _, tt := range tests {
		escaped := yamlEscape(tt.input)
		unescaped := yamlUnescape(escaped)
		// For values that don't need escaping, yamlUnescape is a no-op
		if strings.ContainsAny(tt.input, ":#{}[]|>&*!%@`'\"\\,\n") {
			if unescaped != tt.input {
				t.Errorf("roundtrip failed: %q -> %q -> %q", tt.input, escaped, unescaped)
			}
		}
	}
}

func TestFirstLine(t *testing.T) {
	if got := firstLine("one\ntwo\nthree"); got != "one" {
		t.Errorf("firstLine multiline: got %q", got)
	}
	if got := firstLine("single line"); got != "single line" {
		t.Errorf("firstLine single: got %q", got)
	}
	if got := firstLine(""); got != "" {
		t.Errorf("firstLine empty: got %q", got)
	}
}

func TestProjectRootFromDB(t *testing.T) {
	got := projectRoot("/home/user/myproject/.aide/memory/memory.db")
	if got != "/home/user/myproject" {
		t.Errorf("projectRoot: got %q, want /home/user/myproject", got)
	}
}

// =============================================================================
// Memory Import Conflict Resolution
// =============================================================================

// writeSharedMemoriesFile writes a single-entry memories file for import tests.
func writeSharedMemoriesFile(t *testing.T, tmpDir string, m *memory.Memory) string {
	t.Helper()
	dir := filepath.Join(tmpDir, ".aide", "shared", "memories")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, string(m.Category)+".md")
	if err := writeMemoriesMarkdown(path, m.Category, []*memory.Memory{m}); err != nil {
		t.Fatalf("writeMemoriesMarkdown: %v", err)
	}
	return filepath.Join(tmpDir, ".aide", "shared")
}

func TestShareImportMemoriesUpdateOnNewer(t *testing.T) {
	b, tmpDir, cleanup := setupShareTest(t)
	defer cleanup()

	t0 := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)
	local := &memory.Memory{
		ID:        "MEMUPD01",
		Category:  memory.CategoryLearning,
		Content:   "Original content",
		Tags:      []string{"project:test"},
		CreatedAt: t0,
		UpdatedAt: t0,
	}
	if err := b.Store().AddMemory(local); err != nil {
		t.Fatalf("seed: %v", err)
	}

	incoming := &memory.Memory{
		ID:        "MEMUPD01",
		Category:  memory.CategoryLearning,
		Content:   "Updated content from teammate",
		Tags:      []string{"project:test", "scope:global"},
		CreatedAt: t0,
		UpdatedAt: t0.Add(time.Hour),
	}
	sharedDir := writeSharedMemoriesFile(t, tmpDir, incoming)

	imported, skipped, err := shareImportMemories(b, sharedDir, false)
	if err != nil {
		t.Fatalf("shareImportMemories: %v", err)
	}
	if imported != 1 {
		t.Errorf("imported=%d, want 1 (newer incoming should update)", imported)
	}
	if skipped != 0 {
		t.Errorf("skipped=%d, want 0", skipped)
	}

	got, err := b.GetMemory("MEMUPD01")
	if err != nil {
		t.Fatalf("GetMemory: %v", err)
	}
	if got.Content != "Updated content from teammate" {
		t.Errorf("content not updated: got %q", got.Content)
	}
}

func TestShareImportMemoriesSkipsOlderOrEqual(t *testing.T) {
	b, tmpDir, cleanup := setupShareTest(t)
	defer cleanup()

	tLocal := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)
	local := &memory.Memory{
		ID:        "MEMUPD02",
		Category:  memory.CategoryLearning,
		Content:   "Local wins",
		Tags:      []string{"project:test"},
		CreatedAt: tLocal.Add(-time.Hour),
		UpdatedAt: tLocal,
	}
	if err := b.Store().AddMemory(local); err != nil {
		t.Fatalf("seed: %v", err)
	}

	incoming := &memory.Memory{
		ID:        "MEMUPD02",
		Category:  memory.CategoryLearning,
		Content:   "Should not replace",
		Tags:      []string{"project:test"},
		CreatedAt: tLocal.Add(-time.Hour),
		UpdatedAt: tLocal.Add(-30 * time.Minute),
	}
	sharedDir := writeSharedMemoriesFile(t, tmpDir, incoming)

	imported, skipped, err := shareImportMemories(b, sharedDir, false)
	if err != nil {
		t.Fatalf("shareImportMemories: %v", err)
	}
	if imported != 0 {
		t.Errorf("imported=%d, want 0 (older incoming must not overwrite)", imported)
	}
	if skipped != 1 {
		t.Errorf("skipped=%d, want 1", skipped)
	}

	got, err := b.GetMemory("MEMUPD02")
	if err != nil {
		t.Fatalf("GetMemory: %v", err)
	}
	if got.Content != "Local wins" {
		t.Errorf("content overwritten: got %q", got.Content)
	}
}

// Legacy shared files (exported before the `updated` field existed) must still
// be treated as no-ops when the memory already exists — existing IDs skip,
// new IDs are appended. This preserves backwards compatibility.
func TestShareImportMemoriesLegacyWithoutUpdatedField(t *testing.T) {
	b, tmpDir, cleanup := setupShareTest(t)
	defer cleanup()

	tLocal := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	local := &memory.Memory{
		ID:        "MEMUPD03",
		Category:  memory.CategoryLearning,
		Content:   "Local keeps",
		Tags:      []string{"project:test"},
		CreatedAt: tLocal.Add(-time.Hour),
		UpdatedAt: tLocal,
	}
	if err := b.Store().AddMemory(local); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Simulate a legacy file: UpdatedAt zero and CreatedAt equals UpdatedAt
	// so writeMemoriesMarkdown will not emit an `updated=` field.
	incoming := &memory.Memory{
		ID:        "MEMUPD03",
		Category:  memory.CategoryLearning,
		Content:   "Legacy payload",
		Tags:      []string{"project:test"},
		CreatedAt: tLocal.Add(-time.Hour),
	}
	sharedDir := writeSharedMemoriesFile(t, tmpDir, incoming)

	imported, skipped, err := shareImportMemories(b, sharedDir, false)
	if err != nil {
		t.Fatalf("shareImportMemories: %v", err)
	}
	if imported != 0 {
		t.Errorf("imported=%d, want 0", imported)
	}
	if skipped != 1 {
		t.Errorf("skipped=%d, want 1", skipped)
	}

	got, err := b.GetMemory("MEMUPD03")
	if err != nil {
		t.Fatalf("GetMemory: %v", err)
	}
	if got.Content != "Local keeps" {
		t.Errorf("content overwritten by legacy file: got %q", got.Content)
	}
}

// When the local database has no record with the incoming ULID, share import
// must preserve the incoming ULID and CreatedAt so that a later edit on the
// publishing side (same ULID, newer UpdatedAt) lands as an update on every
// clone instead of creating an orphan duplicate.
func TestShareImportMemoriesPreservesULIDOnNewAdd(t *testing.T) {
	b, tmpDir, cleanup := setupShareTest(t)
	defer cleanup()

	incoming := &memory.Memory{
		ID:        "MEMADD01",
		Category:  memory.CategoryLearning,
		Content:   "New memory from teammate",
		Tags:      []string{"project:test", "scope:global"},
		CreatedAt: time.Date(2026, 4, 1, 9, 0, 0, 0, time.UTC),
	}
	sharedDir := writeSharedMemoriesFile(t, tmpDir, incoming)

	imported, skipped, err := shareImportMemories(b, sharedDir, false)
	if err != nil {
		t.Fatalf("shareImportMemories: %v", err)
	}
	if imported != 1 || skipped != 0 {
		t.Fatalf("counts: imported=%d skipped=%d, want 1/0", imported, skipped)
	}

	got, err := b.GetMemory("MEMADD01")
	if err != nil {
		t.Fatalf("GetMemory: %v (ULID was not preserved)", err)
	}
	if got.ID != "MEMADD01" {
		t.Errorf("id: got %q, want MEMADD01", got.ID)
	}
	if !got.CreatedAt.Equal(incoming.CreatedAt) {
		t.Errorf("CreatedAt: got %s, want %s", got.CreatedAt, incoming.CreatedAt)
	}
}

// The export must write a static DECISIONS.md explainer when the folder has
// content, and the importer must skip it rather than trying to parse it as a
// decision. If all decisions are deleted and the folder is re-exported, the
// explainer disappears too.
func TestShareExportWritesDecisionsReadme(t *testing.T) {
	b, tmpDir, cleanup := setupShareTest(t)
	defer cleanup()

	if _, err := b.SetDecision("topic-a", "value-a", "", "", "", nil); err != nil {
		t.Fatalf("SetDecision: %v", err)
	}

	sharedDir := filepath.Join(tmpDir, ".aide", "shared")
	if _, err := shareExportDecisions(b, sharedDir); err != nil {
		t.Fatalf("export: %v", err)
	}

	readmePath := filepath.Join(sharedDir, "decisions", "DECISIONS.md")
	data, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("DECISIONS.md not written: %v", err)
	}
	if !strings.Contains(string(data), "Team Decisions") {
		t.Errorf("DECISIONS.md missing expected header, got:\n%s", string(data))
	}

	imported, skipped, err := shareImportDecisions(b, sharedDir, false)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if imported != 0 || skipped != 1 {
		t.Errorf("import counts: imported=%d skipped=%d, want 0/1 (topic-a skipped as unchanged, DECISIONS.md ignored)", imported, skipped)
	}

	if _, err := b.ClearDecisions(); err != nil {
		t.Fatalf("ClearDecisions: %v", err)
	}
	if _, err := shareExportDecisions(b, sharedDir); err != nil {
		t.Fatalf("re-export: %v", err)
	}
	if _, err := os.Stat(readmePath); !os.IsNotExist(err) {
		t.Errorf("DECISIONS.md should be removed when folder is empty, got err=%v", err)
	}
}

func TestShareExportWritesMemoriesReadme(t *testing.T) {
	b, tmpDir, cleanup := setupShareTest(t)
	defer cleanup()

	// Seed a shareable memory (pattern category always exports).
	m := &memory.Memory{
		ID:        "MEMRDM01",
		Category:  "pattern",
		Content:   "Use Vitest",
		Tags:      []string{"project:test"},
		CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	if err := b.Store().AddMemory(m); err != nil {
		t.Fatalf("seed: %v", err)
	}

	sharedDir := filepath.Join(tmpDir, ".aide", "shared")
	if _, err := shareExportMemories(b, sharedDir, false); err != nil {
		t.Fatalf("export: %v", err)
	}

	readmePath := filepath.Join(sharedDir, "memories", "MEMORIES.md")
	data, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("MEMORIES.md not written: %v", err)
	}
	if !strings.Contains(string(data), "Team Memories") {
		t.Errorf("MEMORIES.md missing expected header, got:\n%s", string(data))
	}

	imported, skipped, err := shareImportMemories(b, sharedDir, false)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	// Existing memory round-trips (skipped), and MEMORIES.md is not counted.
	if imported != 0 || skipped != 1 {
		t.Errorf("import counts: imported=%d skipped=%d, want 0/1", imported, skipped)
	}

	if err := b.Store().DeleteMemory("MEMRDM01"); err != nil {
		t.Fatalf("DeleteMemory: %v", err)
	}
	if _, err := shareExportMemories(b, sharedDir, false); err != nil {
		t.Fatalf("re-export: %v", err)
	}
	if _, err := os.Stat(readmePath); !os.IsNotExist(err) {
		t.Errorf("MEMORIES.md should be removed when folder is empty, got err=%v", err)
	}
}

// TestParseMemoriesMarkdownUpdatedAt verifies round-trip of the UpdatedAt
// field via the `updated=` metadata token.
func TestParseMemoriesMarkdownUpdatedAt(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "aide-share-updated-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	created := time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)
	updated := time.Date(2026, 1, 12, 9, 30, 0, 0, time.UTC)

	originals := []*memory.Memory{
		{
			ID:        "UPD001",
			Category:  memory.CategoryLearning,
			Content:   "Edited memory",
			Tags:      []string{"project:test"},
			CreatedAt: created,
			UpdatedAt: updated,
		},
	}

	path := filepath.Join(tmpDir, "learning.md")
	if err := writeMemoriesMarkdown(path, memory.CategoryLearning, originals); err != nil {
		t.Fatalf("write: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "updated=2026-01-12T09:30:00Z") {
		t.Errorf("expected `updated=` in file, got:\n%s", string(content))
	}

	parsed, err := parseMemoriesMarkdown(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(parsed) != 1 {
		t.Fatalf("got %d entries, want 1", len(parsed))
	}
	if !parsed[0].UpdatedAt.Equal(updated) {
		t.Errorf("UpdatedAt: got %s, want %s", parsed[0].UpdatedAt, updated)
	}
}
