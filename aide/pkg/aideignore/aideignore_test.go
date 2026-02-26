package aideignore

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBuiltinDefaults(t *testing.T) {
	m := NewFromDefaults()

	// Directories that should be ignored.
	dirs := []string{
		".git", ".svn", ".hg", ".aide", "node_modules", "dist",
		".next", ".nuxt", "coverage", ".cache", "__pycache__",
		".venv", "venv", ".tox", ".mypy_cache", ".pytest_cache",
		"vendor", "target", "build", ".gradle", "out",
		".idea", ".vscode", ".bundle", "bin", "obj",
		"_build", "deps", "_opam", ".bloop", ".metals", ".build",
	}
	for _, d := range dirs {
		if !m.ShouldIgnoreDir(d) {
			t.Errorf("expected directory %q to be ignored by defaults", d)
		}
	}

	// Files that should be ignored.
	files := []string{
		"foo.pb.go",
		"types_generated.go",
		"schema.gen.go",
		"api.pb.ts",
		"api.pb.js",
		"package-lock.lock",
	}
	for _, f := range files {
		if !m.ShouldIgnoreFile(f) {
			t.Errorf("expected file %q to be ignored by defaults", f)
		}
	}

	// Files that should NOT be ignored.
	okFiles := []string{
		"main.go",
		"index.ts",
		"README.md",
		"server.py",
	}
	for _, f := range okFiles {
		if m.ShouldIgnoreFile(f) {
			t.Errorf("expected file %q to NOT be ignored by defaults", f)
		}
	}
}

func TestDirOnlyPattern(t *testing.T) {
	m := NewFromDefaults()

	// "build/" is a dir-only pattern — should not match files named "build".
	if m.ShouldIgnoreFile("build") {
		t.Error("dir-only pattern 'build/' should not match file named 'build'")
	}
	if !m.ShouldIgnoreDir("build") {
		t.Error("dir-only pattern 'build/' should match directory named 'build'")
	}
}

func TestNegation(t *testing.T) {
	m := &Matcher{}
	m.rules = append(m.rules, parsePattern("*.pb.go"))
	m.rules = append(m.rules, parsePattern("!important.pb.go"))

	if !m.ShouldIgnoreFile("foo.pb.go") {
		t.Error("expected foo.pb.go to be ignored")
	}
	if m.ShouldIgnoreFile("important.pb.go") {
		t.Error("expected important.pb.go to be un-ignored by negation")
	}
}

func TestAnchoredPattern(t *testing.T) {
	m := &Matcher{}
	m.rules = append(m.rules, parsePattern("/rootfile.txt"))

	if !m.ShouldIgnoreFile("rootfile.txt") {
		t.Error("expected anchored pattern to match root file")
	}
	if m.ShouldIgnoreFile("sub/rootfile.txt") {
		t.Error("expected anchored pattern to NOT match nested file")
	}
}

func TestUnanchoredPattern(t *testing.T) {
	m := &Matcher{}
	m.rules = append(m.rules, parsePattern("*.log"))

	// Should match at any depth.
	if !m.ShouldIgnoreFile("error.log") {
		t.Error("expected *.log to match root-level file")
	}
	if !m.ShouldIgnoreFile("logs/error.log") {
		t.Error("expected *.log to match nested file")
	}
}

func TestDoubleStarPrefix(t *testing.T) {
	m := &Matcher{}
	m.rules = append(m.rules, parsePattern("**/test/"))

	if !m.ShouldIgnoreDir("test") {
		t.Error("expected **/test/ to match top-level test dir")
	}
	if !m.ShouldIgnoreDir("a/b/test") {
		t.Error("expected **/test/ to match deeply nested test dir")
	}
}

func TestDeepNestedDirMatch(t *testing.T) {
	m := NewFromDefaults()

	// node_modules deep in the tree.
	if !m.ShouldIgnoreDir("packages/foo/node_modules") {
		t.Error("expected node_modules to be ignored at any depth")
	}
	// .git at root.
	if !m.ShouldIgnoreDir(".git") {
		t.Error("expected .git to be ignored")
	}
}

func TestGeneratedGoFiles(t *testing.T) {
	m := NewFromDefaults()

	cases := map[string]bool{
		"aidememory.pb.go":     true,
		"pkg/api/types.pb.go":  true,
		"main.go":              false,
		"widget_generated.go":  true,
		"schema.gen.go":        true,
		"cmd/server/server.go": false,
	}

	for file, expectIgnore := range cases {
		got := m.ShouldIgnoreFile(file)
		if got != expectIgnore {
			t.Errorf("ShouldIgnoreFile(%q) = %v, want %v", file, got, expectIgnore)
		}
	}
}

func TestLoadFile(t *testing.T) {
	// Create a temp .aideignore file.
	dir := t.TempDir()
	content := `# Project-specific ignores
*.generated.ts
testdata/
!testdata/important.txt
/config.local.yaml
`
	if err := os.WriteFile(filepath.Join(dir, ".aideignore"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	m, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}

	// User pattern should work.
	if !m.ShouldIgnoreFile("foo.generated.ts") {
		t.Error("expected *.generated.ts to be ignored")
	}

	// Dir-only pattern from file.
	if !m.ShouldIgnoreDir("testdata") {
		t.Error("expected testdata/ to be ignored")
	}

	// Negation from file.
	if m.ShouldIgnoreFile("testdata/important.txt") {
		t.Error("expected testdata/important.txt to be un-ignored")
	}

	// Anchored pattern from file.
	if !m.ShouldIgnoreFile("config.local.yaml") {
		t.Error("expected /config.local.yaml to match root file")
	}
	if m.ShouldIgnoreFile("sub/config.local.yaml") {
		t.Error("expected /config.local.yaml to NOT match nested file")
	}

	// Builtins should still work.
	if !m.ShouldIgnoreDir("node_modules") {
		t.Error("expected node_modules to still be ignored from builtins")
	}
}

func TestMissingFileUsesDefaults(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Should still have builtins.
	if !m.ShouldIgnoreDir("node_modules") {
		t.Error("expected node_modules to be ignored from builtins")
	}
}

func TestWalkFunc(t *testing.T) {
	m := NewFromDefaults()
	root := "/project"
	shouldSkip := m.WalkFunc(root)

	// Simulate a directory.
	dirInfo := &fakeFileInfo{name: "node_modules", dir: true}
	skip, skipDir := shouldSkip(filepath.Join(root, "node_modules"), dirInfo)
	if !skip || !skipDir {
		t.Error("expected WalkFunc to skip node_modules directory")
	}

	// Simulate a normal file.
	fileInfo := &fakeFileInfo{name: "main.go", dir: false}
	skip, skipDir = shouldSkip(filepath.Join(root, "main.go"), fileInfo)
	if skip {
		t.Error("expected WalkFunc to NOT skip main.go")
	}
	if skipDir {
		t.Error("skipDir should be false for files")
	}

	// Simulate a generated file.
	genInfo := &fakeFileInfo{name: "api.pb.go", dir: false}
	skip, skipDir = shouldSkip(filepath.Join(root, "pkg", "api.pb.go"), genInfo)
	if !skip {
		t.Error("expected WalkFunc to skip api.pb.go")
	}
	if skipDir {
		t.Error("skipDir should be false for files")
	}
}

// fakeFileInfo is a minimal os.FileInfo for testing.
type fakeFileInfo struct {
	name string
	dir  bool
}

func (f *fakeFileInfo) Name() string       { return f.name }
func (f *fakeFileInfo) Size() int64        { return 0 }
func (f *fakeFileInfo) Mode() os.FileMode  { return 0 }
func (f *fakeFileInfo) ModTime() time.Time { return time.Time{} }
func (f *fakeFileInfo) IsDir() bool        { return f.dir }
func (f *fakeFileInfo) Sys() any           { return nil }

func TestAnchoredDirChildPaths(t *testing.T) {
	// An anchored dir-only pattern like "packages/opencode-plugin/src/"
	// should match files inside that directory, not just the directory itself.
	m := &Matcher{}
	m.rules = append(m.rules, parsePattern("packages/opencode-plugin/src/"))

	// Should match the directory itself.
	if !m.ShouldIgnoreDir("packages/opencode-plugin/src") {
		t.Error("expected anchored dir pattern to match the directory itself")
	}

	// Should match files inside the directory.
	if !m.ShouldIgnoreFile("packages/opencode-plugin/src/index.ts") {
		t.Error("expected anchored dir pattern to match file inside directory")
	}

	// Should match deeply nested files.
	if !m.ShouldIgnoreFile("packages/opencode-plugin/src/utils/helper.ts") {
		t.Error("expected anchored dir pattern to match deeply nested file")
	}

	// Should NOT match files outside the directory.
	if m.ShouldIgnoreFile("packages/opencode-plugin/README.md") {
		t.Error("expected anchored dir pattern to NOT match file outside directory")
	}

	// Should NOT match a file whose name starts with the dir name.
	if m.ShouldIgnoreFile("packages/opencode-plugin/src-backup/file.ts") {
		t.Error("expected anchored dir pattern to NOT match similarly-named directory")
	}
}

func TestUnanchoredDirChildPaths(t *testing.T) {
	// An unanchored dir-only pattern like "node_modules/" should match
	// files inside node_modules at any depth, even when called with
	// ShouldIgnoreFile (e.g. from OnChanges with individual file paths).
	m := NewFromDefaults()

	// File directly inside node_modules.
	if !m.ShouldIgnoreFile("node_modules/express/index.js") {
		t.Error("expected unanchored dir pattern to match file inside node_modules")
	}

	// File inside nested node_modules.
	if !m.ShouldIgnoreFile("packages/app/node_modules/lodash/lodash.js") {
		t.Error("expected unanchored dir pattern to match file inside nested node_modules")
	}

	// Vendor at root.
	if !m.ShouldIgnoreFile("vendor/github.com/foo/bar.go") {
		t.Error("expected unanchored dir pattern to match file inside vendor")
	}
}
