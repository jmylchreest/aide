package aideignore

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/config"
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
		"vendor/tabulator.min.js",
		"static/site.min.css",
		"src/app.bundle.js",
		"dist/app.js.map",
		"third_party/leveldb/db.go",
		"web/static/vendor/jquery.js",
		"public/lib/d3.js",
		"src/assets/libs/charts.js",
		"app/external/fetcher.go",
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
	m := newFromPatternStrings("*.pb.go", "!important.pb.go")

	if !m.ShouldIgnoreFile("foo.pb.go") {
		t.Error("expected foo.pb.go to be ignored")
	}
	if m.ShouldIgnoreFile("important.pb.go") {
		t.Error("expected important.pb.go to be un-ignored by negation")
	}
}

func TestAnchoredPattern(t *testing.T) {
	m := newFromPatternStrings("/rootfile.txt")

	if !m.ShouldIgnoreFile("rootfile.txt") {
		t.Error("expected anchored pattern to match root file")
	}
	if m.ShouldIgnoreFile("sub/rootfile.txt") {
		t.Error("expected anchored pattern to NOT match nested file")
	}
}

func TestUnanchoredPattern(t *testing.T) {
	m := newFromPatternStrings("*.log")

	// Should match at any depth.
	if !m.ShouldIgnoreFile("error.log") {
		t.Error("expected *.log to match root-level file")
	}
	if !m.ShouldIgnoreFile("logs/error.log") {
		t.Error("expected *.log to match nested file")
	}
}

func TestDoubleStarPrefix(t *testing.T) {
	m := newFromPatternStrings("**/test/")

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
	m := newFromPatternStrings("packages/opencode-plugin/src/")

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

// writeGitRepo creates a directory that looks enough like a worktree root for
// the gitignore layer to engage, with the given files written relative to it.
func writeGitRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range files {
		full := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	InvalidateGitignoreCache()
	return dir
}

func TestGitignoreLayer(t *testing.T) {
	dir := writeGitRepo(t, map[string]string{
		".gitignore": ".playwright-mcp/\ndesign/\n*.tmp\n",
	})

	m, err := NewWithOptions(dir, Options{Gitignore: true})
	if err != nil {
		t.Fatal(err)
	}

	if !m.ShouldIgnoreDir(".playwright-mcp") {
		t.Error("expected gitignored directory to be ignored")
	}
	if !m.ShouldIgnoreFile(".playwright-mcp/page-2026-07-21.yml") {
		t.Error("expected file inside a gitignored directory to be ignored")
	}
	if !m.ShouldIgnoreFile("design/notes.md") {
		t.Error("expected file inside gitignored design/ to be ignored")
	}
	if !m.ShouldIgnoreFile("scratch.tmp") {
		t.Error("expected *.tmp from .gitignore to be ignored")
	}
	if m.ShouldIgnoreFile("main.go") {
		t.Error("expected an untracked-by-pattern source file to be scanned")
	}
}

func TestGitignoreLayerDisabled(t *testing.T) {
	dir := writeGitRepo(t, map[string]string{
		".gitignore": ".playwright-mcp/\n",
	})

	m, err := NewWithOptions(dir, Options{Gitignore: false})
	if err != nil {
		t.Fatal(err)
	}

	if m.ShouldIgnoreDir(".playwright-mcp") {
		t.Error("expected gitignore rules to be skipped when the option is off")
	}
	// Builtins must still apply.
	if !m.ShouldIgnoreDir("node_modules") {
		t.Error("expected builtins to still apply with the gitignore layer off")
	}
}

func TestGitignoreNestedDomain(t *testing.T) {
	// A pattern in docs/.gitignore is scoped to docs/ — flattening the
	// patterns into one root-level list would wrongly ignore build/ elsewhere.
	dir := writeGitRepo(t, map[string]string{
		"docs/.gitignore": "/generated/\n",
	})

	m, err := NewWithOptions(dir, Options{Gitignore: true})
	if err != nil {
		t.Fatal(err)
	}

	if !m.ShouldIgnoreDir("docs/generated") {
		t.Error("expected nested .gitignore to ignore within its own directory")
	}
	if m.ShouldIgnoreDir("generated") {
		t.Error("expected nested .gitignore pattern NOT to escape its directory")
	}
}

func TestAideignoreOverridesGitignore(t *testing.T) {
	// .aideignore is layered last, so a "!" line there wins over .gitignore.
	dir := writeGitRepo(t, map[string]string{
		".gitignore":  "generated/\n",
		".aideignore": "!generated/schema.go\n",
	})

	m, err := NewWithOptions(dir, Options{Gitignore: true})
	if err != nil {
		t.Fatal(err)
	}

	if !m.ShouldIgnoreFile("generated/other.go") {
		t.Error("expected gitignored file to stay ignored")
	}
	if m.ShouldIgnoreFile("generated/schema.go") {
		t.Error("expected .aideignore negation to re-include the file")
	}
}

func TestGitignoreSkippedOutsideRepo(t *testing.T) {
	// No .git — a stray .gitignore must not engage the layer, and the walk
	// that would discover it is skipped entirely.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("secret/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	InvalidateGitignoreCache()

	m, err := NewWithOptions(dir, Options{Gitignore: true})
	if err != nil {
		t.Fatal(err)
	}

	if m.ShouldIgnoreDir("secret") {
		t.Error("expected .gitignore to be skipped when the root is not a worktree")
	}
}

func TestNewHonoursConfig(t *testing.T) {
	dir := writeGitRepo(t, map[string]string{
		".gitignore": "artifacts/\n",
	})

	off := false
	config.Set(&config.Config{Code: config.CodeConfig{RespectGitignore: &off}})
	t.Cleanup(func() { config.Set(nil) })

	m, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if m.ShouldIgnoreDir("artifacts") {
		t.Error("expected code.respect_gitignore=false to disable the layer")
	}

	on := true
	config.Set(&config.Config{Code: config.CodeConfig{RespectGitignore: &on}})
	InvalidateGitignoreCache()

	m, err = New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !m.ShouldIgnoreDir("artifacts") {
		t.Error("expected code.respect_gitignore=true to enable the layer")
	}
}

func TestUnloadedConfigDefaultsGitignoreOn(t *testing.T) {
	// config.Get() returns a zero Config before Load runs; the *bool must
	// still resolve to on so a fresh process does not silently scan
	// gitignored paths.
	config.Set(nil)
	if !config.Get().Code.RespectGitignoreEnabled() {
		t.Error("expected RespectGitignore to default on with no config loaded")
	}
}

func TestGitInfoExcludeRead(t *testing.T) {
	// .git/info/exclude is the untracked-but-ignored list. It is documented as
	// part of the gitignore layer but nothing exercised it.
	dir := writeGitRepo(t, map[string]string{
		".git/info/exclude": "scratch/\n",
	})

	m, err := NewWithOptions(dir, Options{Gitignore: true})
	if err != nil {
		t.Fatal(err)
	}
	if !m.ShouldIgnoreDir("scratch") {
		t.Error("expected .git/info/exclude patterns to be honoured")
	}
}

func TestLinkedWorktreeGitFile(t *testing.T) {
	// In a linked worktree .git is a file pointing at the real gitdir, not a
	// directory. The root .gitignore must still be read, and the failed
	// .git/info/exclude open must not surface as an error.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".git"), []byte("gitdir: /elsewhere/.git/worktrees/wt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("artifacts/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	InvalidateGitignoreCache()

	m, err := NewWithOptions(dir, Options{Gitignore: true})
	if err != nil {
		t.Fatalf("linked worktree should not error: %v", err)
	}
	if !m.ShouldIgnoreDir("artifacts") {
		t.Error("expected .gitignore to be read in a linked worktree")
	}
}

func TestGitignoreCacheReuseAndInvalidate(t *testing.T) {
	dir := writeGitRepo(t, map[string]string{".gitignore": "before/\n"})

	m, err := NewWithOptions(dir, Options{Gitignore: true})
	if err != nil {
		t.Fatal(err)
	}
	if !m.ShouldIgnoreDir("before") {
		t.Fatal("expected the initial pattern to apply")
	}

	// Rewrite within the TTL: the cached scan is reused, so the new pattern
	// must not be visible yet. This is what keeps a findings run from walking
	// the tree once per analyser.
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("after/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err = NewWithOptions(dir, Options{Gitignore: true})
	if err != nil {
		t.Fatal(err)
	}
	if !m.ShouldIgnoreDir("before") || m.ShouldIgnoreDir("after") {
		t.Error("expected the cached scan to be reused within the TTL")
	}

	InvalidateGitignoreCache()
	m, err = NewWithOptions(dir, Options{Gitignore: true})
	if err != nil {
		t.Fatal(err)
	}
	if m.ShouldIgnoreDir("before") || !m.ShouldIgnoreDir("after") {
		t.Error("expected InvalidateGitignoreCache to force a re-read")
	}
}

func TestGitignoreCacheIsPerRoot(t *testing.T) {
	a := writeGitRepo(t, map[string]string{".gitignore": "only-a/\n"})
	b := writeGitRepo(t, map[string]string{".gitignore": "only-b/\n"})

	ma, err := NewWithOptions(a, Options{Gitignore: true})
	if err != nil {
		t.Fatal(err)
	}
	mb, err := NewWithOptions(b, Options{Gitignore: true})
	if err != nil {
		t.Fatal(err)
	}

	if !ma.ShouldIgnoreDir("only-a") || ma.ShouldIgnoreDir("only-b") {
		t.Error("first root picked up the wrong cache entry")
	}
	if !mb.ShouldIgnoreDir("only-b") || mb.ShouldIgnoreDir("only-a") {
		t.Error("second root picked up the wrong cache entry")
	}
}

func TestRootPathNeverIgnored(t *testing.T) {
	// filepath.Rel(root, root) yields "." — if that were treated as a match,
	// a walk would skip the entire project.
	m := NewFromDefaults()
	for _, p := range []string{"", ".", "./"} {
		if m.ShouldIgnoreDir(p) {
			t.Errorf("ShouldIgnoreDir(%q) = true, want false", p)
		}
	}

	root := "/project"
	skip, _ := m.WalkFunc(root)(root, &fakeFileInfo{name: "project", dir: true})
	if skip {
		t.Error("expected WalkFunc not to skip the project root itself")
	}
}

func TestWalkFuncUnrelatedPath(t *testing.T) {
	// A path that is not under projectRoot cannot be made relative; the raw
	// path is matched instead of silently ignoring everything.
	m := NewFromDefaults()
	shouldSkip := m.WalkFunc("relative-root")

	skip, _ := shouldSkip("/elsewhere/node_modules", &fakeFileInfo{name: "node_modules", dir: true})
	if !skip {
		t.Error("expected the fallback to still match on the raw path")
	}
}

func TestNewEmptyIgnoresNothing(t *testing.T) {
	m := NewEmpty()
	for _, p := range []string{"node_modules", ".git", "vendor"} {
		if m.ShouldIgnoreDir(p) {
			t.Errorf("NewEmpty should ignore nothing, but ignored %q", p)
		}
	}
	if m.ShouldIgnoreFile("api.pb.go") {
		t.Error("NewEmpty should not ignore generated files either")
	}
}

func TestUnreadableAideignoreErrors(t *testing.T) {
	// A .aideignore that cannot be read is a real misconfiguration and must
	// surface, unlike a missing one. A directory in its place reads as EISDIR.
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, IgnoreFileName), 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := NewWithOptions(dir, Options{}); err == nil {
		t.Error("expected an unreadable .aideignore to surface an error")
	}
}
