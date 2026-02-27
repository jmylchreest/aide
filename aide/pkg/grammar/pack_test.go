package grammar

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// allExpectedLanguages lists every language that should have a pack.json.
var allExpectedLanguages = []string{
	// 9 embedded (built-in)
	"c", "cpp", "go", "java", "javascript", "python", "rust", "typescript", "zig",
	// 19 dynamic
	"bash", "csharp", "css", "elixir", "elm", "groovy", "hcl", "html",
	"kotlin", "lua", "ocaml", "php", "protobuf", "ruby", "scala", "sql",
	"swift", "toml", "yaml",
	// 2 meta-only (no grammar, just file detection)
	"dockerfile", "json",
}

func TestNewPackRegistry_LoadsAllPacks(t *testing.T) {
	reg, err := NewPackRegistry()
	if err != nil {
		t.Fatalf("NewPackRegistry() error: %v", err)
	}

	got := reg.All()
	sort.Strings(got)

	want := make([]string, len(allExpectedLanguages))
	copy(want, allExpectedLanguages)
	sort.Strings(want)

	if len(got) != len(want) {
		t.Fatalf("expected %d languages, got %d\ngot:      %v\nexpected: %v",
			len(want), len(got), got, want)
	}

	for i, name := range want {
		if got[i] != name {
			t.Errorf("position %d: expected %q, got %q", i, name, got[i])
		}
	}
}

func TestNewPackRegistry_AllPacksHaveSchemaVersion(t *testing.T) {
	reg, err := NewPackRegistry()
	if err != nil {
		t.Fatalf("NewPackRegistry() error: %v", err)
	}
	for _, name := range allExpectedLanguages {
		p := reg.Get(name)
		if p == nil {
			t.Errorf("pack %q not found", name)
			continue
		}
		if p.SchemaVersion != 1 {
			t.Errorf("pack %q: schema_version = %d, want 1", name, p.SchemaVersion)
		}
	}
}

func TestNewPackRegistry_AllPacksHaveExtensions(t *testing.T) {
	reg, err := NewPackRegistry()
	if err != nil {
		t.Fatalf("NewPackRegistry() error: %v", err)
	}
	for _, name := range allExpectedLanguages {
		p := reg.Get(name)
		if p == nil {
			t.Errorf("pack %q not found", name)
			continue
		}
		// Meta-only packs may use filenames instead of extensions.
		if len(p.Meta.Extensions) == 0 && len(p.Meta.Filenames) == 0 {
			t.Errorf("pack %q has no extensions or filenames", name)
		}
	}
}

func TestNewPackRegistry_AllPacksHaveCSymbol(t *testing.T) {
	reg, err := NewPackRegistry()
	if err != nil {
		t.Fatalf("NewPackRegistry() error: %v", err)
	}
	// Meta-only packs that have no grammar binary (e.g., json, dockerfile).
	metaOnly := map[string]bool{"json": true, "dockerfile": true}
	for _, name := range allExpectedLanguages {
		p := reg.Get(name)
		if p == nil {
			t.Errorf("pack %q not found", name)
			continue
		}
		if metaOnly[name] {
			continue
		}
		if p.CSymbol == "" {
			t.Errorf("pack %q has no c_symbol", name)
		}
	}
}

// TestLangForExtension_SpotChecks verifies a representative sample of extension lookups.
func TestLangForExtension_SpotChecks(t *testing.T) {
	reg, err := NewPackRegistry()
	if err != nil {
		t.Fatalf("NewPackRegistry() error: %v", err)
	}

	tests := map[string]string{
		// Built-in
		".go":   "go",
		".ts":   "typescript",
		".tsx":  "typescript",
		".js":   "javascript",
		".jsx":  "javascript",
		".py":   "python",
		".rs":   "rust",
		".java": "java",
		".c":    "c",
		".h":    "c",
		".cpp":  "cpp",
		".hpp":  "cpp",
		".cc":   "cpp",
		".zig":  "zig",
		// Dynamic
		".cs":     "csharp",
		".kt":     "kotlin",
		".kts":    "kotlin",
		".scala":  "scala",
		".sc":     "scala",
		".groovy": "groovy",
		".gradle": "groovy",
		".rb":     "ruby",
		".rake":   "ruby",
		".php":    "php",
		".lua":    "lua",
		".ex":     "elixir",
		".exs":    "elixir",
		".sh":     "bash",
		".bash":   "bash",
		".zsh":    "bash",
		".swift":  "swift",
		".ml":     "ocaml",
		".mli":    "ocaml",
		".elm":    "elm",
		".sql":    "sql",
		".yaml":   "yaml",
		".yml":    "yaml",
		".toml":   "toml",
		".hcl":    "hcl",
		".tf":     "hcl",
		".proto":  "protobuf",
		".html":   "html",
		".htm":    "html",
		".css":    "css",
	}

	for ext, want := range tests {
		got, ok := reg.LangForExtension(ext)
		if !ok {
			t.Errorf("LangForExtension(%q): not found, want %q", ext, want)
			continue
		}
		if got != want {
			t.Errorf("LangForExtension(%q) = %q, want %q", ext, got, want)
		}
	}
}

// TestLangForFilename_SpotChecks verifies filename-to-language lookups.
func TestLangForFilename_SpotChecks(t *testing.T) {
	reg, err := NewPackRegistry()
	if err != nil {
		t.Fatalf("NewPackRegistry() error: %v", err)
	}

	tests := map[string]string{
		"Jenkinsfile": "groovy",
		"Vagrantfile": "ruby",
		"Rakefile":    "ruby",
		"Gemfile":     "ruby",
	}

	for fn, want := range tests {
		got, ok := reg.LangForFilename(fn)
		if !ok {
			t.Errorf("LangForFilename(%q): not found, want %q", fn, want)
			continue
		}
		if got != want {
			t.Errorf("LangForFilename(%q) = %q, want %q", fn, got, want)
		}
	}
}

// TestLangForShebang_SpotChecks verifies shebang interpreter lookups.
func TestLangForShebang_SpotChecks(t *testing.T) {
	reg, err := NewPackRegistry()
	if err != nil {
		t.Fatalf("NewPackRegistry() error: %v", err)
	}

	tests := map[string]string{
		"python":  "python",
		"python3": "python",
		"node":    "javascript",
		"ruby":    "ruby",
		"bash":    "bash",
		"sh":      "bash",
		"zsh":     "bash",
		"php":     "php",
		"lua":     "lua",
		"elixir":  "elixir",
		"swift":   "swift",
		"kotlin":  "kotlin",
		"scala":   "scala",
		"groovy":  "groovy",
	}

	for interp, want := range tests {
		got, ok := reg.LangForShebang(interp)
		if !ok {
			t.Errorf("LangForShebang(%q): not found, want %q", interp, want)
			continue
		}
		if got != want {
			t.Errorf("LangForShebang(%q) = %q, want %q", interp, got, want)
		}
	}
}

// TestNormaliseLang_Aliases verifies alias-to-canonical mappings.
func TestNormaliseLang_Aliases(t *testing.T) {
	reg, err := NewPackRegistry()
	if err != nil {
		t.Fatalf("NewPackRegistry() error: %v", err)
	}

	tests := map[string]string{
		// Canonical names return themselves
		"go":         "go",
		"python":     "python",
		"typescript": "typescript",
		"javascript": "javascript",
		"rust":       "rust",
		"java":       "java",
		"csharp":     "csharp",
		"ruby":       "ruby",
		"bash":       "bash",
		// Aliases
		"golang":    "go",
		"py":        "python",
		"ts":        "typescript",
		"js":        "javascript",
		"c#":        "csharp",
		"cs":        "csharp",
		"kt":        "kotlin",
		"rb":        "ruby",
		"sh":        "bash",
		"shell":     "bash",
		"ml":        "ocaml",
		"yml":       "yaml",
		"tf":        "hcl",
		"terraform": "hcl",
		"proto":     "protobuf",
		"ex":        "elixir",
		"exs":       "elixir",
		// Unknown returns unchanged
		"foobar": "foobar",
	}

	for input, want := range tests {
		got := reg.NormaliseLang(input)
		if got != want {
			t.Errorf("NormaliseLang(%q) = %q, want %q", input, got, want)
		}
	}
}

// TestGetByAlias verifies alias lookups return the correct pack.
func TestGetByAlias(t *testing.T) {
	reg, err := NewPackRegistry()
	if err != nil {
		t.Fatalf("NewPackRegistry() error: %v", err)
	}

	tests := map[string]string{
		"golang": "go",
		"py":     "python",
		"ts":     "typescript",
		"c#":     "csharp",
		"rb":     "ruby",
		"kt":     "kotlin",
		"sh":     "bash",
		"ml":     "ocaml",
		"yml":    "yaml",
		"tf":     "hcl",
		"proto":  "protobuf",
	}

	for alias, wantName := range tests {
		p := reg.GetByAlias(alias)
		if p == nil {
			t.Errorf("GetByAlias(%q) returned nil, want pack %q", alias, wantName)
			continue
		}
		if p.Name != wantName {
			t.Errorf("GetByAlias(%q).Name = %q, want %q", alias, p.Name, wantName)
		}
	}

	// Unknown alias should return nil.
	if p := reg.GetByAlias("nonexistent"); p != nil {
		t.Errorf("GetByAlias(\"nonexistent\") = %v, want nil", p.Name)
	}
}

// TestGet_ReturnsNilForUnknown verifies Get returns nil for unknown languages.
func TestGet_ReturnsNilForUnknown(t *testing.T) {
	reg, err := NewPackRegistry()
	if err != nil {
		t.Fatalf("NewPackRegistry() error: %v", err)
	}
	if p := reg.Get("nonexistent_language"); p != nil {
		t.Errorf("Get(\"nonexistent_language\") = %v, want nil", p.Name)
	}
}

// TestPacksWithTagQueries verifies languages that should have tag queries do.
func TestPacksWithTagQueries(t *testing.T) {
	reg, err := NewPackRegistry()
	if err != nil {
		t.Fatalf("NewPackRegistry() error: %v", err)
	}

	// Languages that have tag queries in their pack.json.
	langsWithTagQueries := []string{
		"go", "typescript", "javascript", "python", "rust", "java", "c", "cpp",
		"csharp", "kotlin", "scala", "ruby", "php", "lua", "elixir", "bash",
		"swift", "sql", "hcl", "protobuf",
	}
	for _, name := range langsWithTagQueries {
		p := reg.Get(name)
		if p == nil {
			t.Errorf("pack %q not found", name)
			continue
		}
		if p.Queries.Tags == "" {
			t.Errorf("pack %q: expected non-empty tag query", name)
		}
	}
}

// TestPacksWithRefQueries verifies languages that should have ref queries do.
func TestPacksWithRefQueries(t *testing.T) {
	reg, err := NewPackRegistry()
	if err != nil {
		t.Fatalf("NewPackRegistry() error: %v", err)
	}

	// Languages that have ref queries in their pack.json.
	langsWithRefQueries := []string{
		"go", "typescript", "javascript", "python", "rust", "java", "c", "cpp",
		"ruby", "php",
	}
	for _, name := range langsWithRefQueries {
		p := reg.Get(name)
		if p == nil {
			t.Errorf("pack %q not found", name)
			continue
		}
		if p.Queries.Refs == "" {
			t.Errorf("pack %q: expected non-empty ref query", name)
		}
	}

	// Languages without ref queries should have empty refs.
	langsWithoutRefQueries := []string{
		"zig", "csharp", "kotlin", "scala", "lua", "elixir", "bash", "swift",
		"sql", "hcl", "protobuf",
	}
	for _, name := range langsWithoutRefQueries {
		p := reg.Get(name)
		if p == nil {
			t.Errorf("pack %q not found", name)
			continue
		}
		if p.Queries.Refs != "" {
			t.Errorf("pack %q: expected empty ref query, got %q", name, p.Queries.Refs)
		}
	}
}

// TestPacksWithComplexity verifies languages that should have complexity data do.
func TestPacksWithComplexity(t *testing.T) {
	reg, err := NewPackRegistry()
	if err != nil {
		t.Fatalf("NewPackRegistry() error: %v", err)
	}

	// Languages that have complexity configurations.
	langsWithComplexity := []string{
		"go", "typescript", "javascript", "python", "rust", "java", "c", "cpp", "zig",
		"csharp", "kotlin", "scala", "ruby", "php", "lua", "bash", "swift",
		"elixir", "ocaml", "groovy", "elm",
	}
	for _, name := range langsWithComplexity {
		p := reg.Get(name)
		if p == nil {
			t.Errorf("pack %q not found", name)
			continue
		}
		if p.Complexity == nil {
			t.Errorf("pack %q: expected complexity data, got nil", name)
			continue
		}
		if len(p.Complexity.FuncNodeTypes) == 0 {
			t.Errorf("pack %q: complexity has empty func_node_types", name)
		}
		if len(p.Complexity.BranchTypes) == 0 {
			t.Errorf("pack %q: complexity has empty branch_types", name)
		}
		if p.Complexity.NameField == "" {
			t.Errorf("pack %q: complexity has empty name_field", name)
		}
	}

	// Languages without complexity should have nil.
	langsWithoutComplexity := []string{
		"sql", "yaml", "toml", "hcl",
		"protobuf", "html", "css",
	}
	for _, name := range langsWithoutComplexity {
		p := reg.Get(name)
		if p == nil {
			t.Errorf("pack %q not found", name)
			continue
		}
		if p.Complexity != nil {
			t.Errorf("pack %q: expected nil complexity, got %+v", name, p.Complexity)
		}
	}
}

// TestPacksWithImports verifies languages that should have import patterns do.
func TestPacksWithImports(t *testing.T) {
	reg, err := NewPackRegistry()
	if err != nil {
		t.Fatalf("NewPackRegistry() error: %v", err)
	}

	// Languages that have import patterns in pack.json.
	langsWithImports := []string{
		"go", "python", "typescript", "javascript", "java", "rust",
		"csharp", "kotlin", "scala", "ruby", "php", "lua", "elixir", "swift", "ocaml",
	}
	for _, name := range langsWithImports {
		p := reg.Get(name)
		if p == nil {
			t.Errorf("pack %q not found", name)
			continue
		}
		if p.Imports == nil {
			t.Errorf("pack %q: expected import patterns, got nil", name)
			continue
		}
		if len(p.Imports.Patterns) == 0 {
			t.Errorf("pack %q: import patterns is empty", name)
		}
	}
}

// TestLanguages_ReturnsAllPacks verifies Languages() returns a map of all packs.
func TestLanguages_ReturnsAllPacks(t *testing.T) {
	reg, err := NewPackRegistry()
	if err != nil {
		t.Fatalf("NewPackRegistry() error: %v", err)
	}

	langs := reg.Languages()
	if len(langs) != len(allExpectedLanguages) {
		t.Errorf("Languages() returned %d entries, want %d", len(langs), len(allExpectedLanguages))
	}

	for _, name := range allExpectedLanguages {
		if _, ok := langs[name]; !ok {
			t.Errorf("Languages() missing %q", name)
		}
	}
}

// TestLoadFromDir_OverridesEmbedded verifies that LoadFromDir can override an
// embedded pack (simulating a downloaded pack taking precedence).
func TestLoadFromDir_OverridesEmbedded(t *testing.T) {
	reg, err := NewPackRegistry()
	if err != nil {
		t.Fatalf("NewPackRegistry() error: %v", err)
	}

	// Verify the embedded go pack has extensions.
	goPack := reg.Get("go")
	if goPack == nil {
		t.Fatal("embedded go pack not found")
	}
	if goPack.Version == "override-test" {
		t.Fatal("embedded go pack already has override version")
	}

	// Create a temporary directory with an override pack.json.
	tmpDir := t.TempDir()
	overridePack := []byte(`{
  "schema_version": 1,
  "name": "go",
  "version": "override-test",
  "c_symbol": "tree_sitter_go",
  "meta": {
    "extensions": [".go", ".go2"],
    "aliases": ["golang"]
  }
}`)
	if err := os.WriteFile(filepath.Join(tmpDir, "pack.json"), overridePack, 0644); err != nil {
		t.Fatalf("writing override pack: %v", err)
	}

	// Load the override.
	if err := reg.LoadFromDir(tmpDir); err != nil {
		t.Fatalf("LoadFromDir() error: %v", err)
	}

	// The pack should now have the override version.
	goPack = reg.Get("go")
	if goPack == nil {
		t.Fatal("go pack not found after override")
	}
	if goPack.Version != "override-test" {
		t.Errorf("go pack version = %q, want %q", goPack.Version, "override-test")
	}

	// Extension lookup should now include .go2.
	if lang, ok := reg.LangForExtension(".go2"); !ok || lang != "go" {
		t.Errorf("LangForExtension(\".go2\") = (%q, %v), want (\"go\", true)", lang, ok)
	}
}

// TestLoadFromDir_ErrorOnMissingFile verifies LoadFromDir returns an error for
// a directory without pack.json.
func TestLoadFromDir_ErrorOnMissingFile(t *testing.T) {
	tmpDir := t.TempDir()
	reg, err := NewPackRegistry()
	if err != nil {
		t.Fatalf("NewPackRegistry() error: %v", err)
	}
	if err := reg.LoadFromDir(tmpDir); err == nil {
		t.Error("LoadFromDir() on empty dir: expected error, got nil")
	}
}

// TestLoadFromDir_ErrorOnInvalidJSON verifies LoadFromDir returns an error for
// invalid JSON.
func TestLoadFromDir_ErrorOnInvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "pack.json"), []byte("{invalid"), 0644); err != nil {
		t.Fatal(err)
	}
	reg, err := NewPackRegistry()
	if err != nil {
		t.Fatalf("NewPackRegistry() error: %v", err)
	}
	if err := reg.LoadFromDir(tmpDir); err == nil {
		t.Error("LoadFromDir() on invalid JSON: expected error, got nil")
	}
}

// TestCSymbol_MatchesDynamicPacks verifies that each dynamic language pack
// has a non-empty c_symbol and source_repo in its pack.json.
func TestCSymbol_MatchesDynamicPacks(t *testing.T) {
	reg, err := NewPackRegistry()
	if err != nil {
		t.Fatalf("NewPackRegistry() error: %v", err)
	}

	for name, pack := range reg.DynamicPacks() {
		if pack.CSymbol == "" {
			t.Errorf("pack %q: c_symbol is empty", name)
		}
		if pack.SourceRepo == "" {
			t.Errorf("pack %q: source_repo is empty", name)
		}
	}
}

// TestGoComplexity_MatchesHardcoded is a spot-check that verifies the Go pack's
// complexity data matches what's hardcoded in complexity_go.go.
func TestGoComplexity_MatchesHardcoded(t *testing.T) {
	reg, err := NewPackRegistry()
	if err != nil {
		t.Fatalf("NewPackRegistry() error: %v", err)
	}

	p := reg.Get("go")
	if p == nil {
		t.Fatal("go pack not found")
	}
	if p.Complexity == nil {
		t.Fatal("go pack has no complexity data")
	}

	// From complexity_go.go: funcNodeTypes = function_declaration, method_declaration, func_literal
	expectedFuncs := []string{"function_declaration", "method_declaration", "func_literal"}
	if !stringSlicesEqual(p.Complexity.FuncNodeTypes, expectedFuncs) {
		t.Errorf("go complexity func_node_types = %v, want %v",
			p.Complexity.FuncNodeTypes, expectedFuncs)
	}

	if p.Complexity.NameField != "name" {
		t.Errorf("go complexity name_field = %q, want %q", p.Complexity.NameField, "name")
	}
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
