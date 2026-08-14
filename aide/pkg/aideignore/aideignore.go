// Package aideignore provides gitignore-compatible file matching for aide.
//
// Patterns come from three layered sources, in increasing priority:
//
//  1. BuiltinDefaults — generated code, build output, common vendored dirs.
//  2. The repository's own ignore rules — .git/info/exclude plus every
//     .gitignore in the tree — when enabled. See Options.Gitignore.
//  3. <projectRoot>/.aideignore — project overrides, which can re-include
//     anything the layers above ignored via a leading "!".
//
// All three are parsed by go-git's gitignore package, so the semantics are
// git's rather than an approximation of them: the last matching pattern wins,
// "!" re-includes, a trailing "/" restricts a pattern to directories, "**"
// spans path segments, and a leading "/" anchors to the domain root.
package aideignore

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-git/go-billy/v5/osfs"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"

	"github.com/jmylchreest/aide/aide/pkg/config"
)

// IgnoreFileName is the project-relative ignore file aide reads for
// project-specific overrides.
const IgnoreFileName = ".aideignore"

// Matcher tests whether a path should be ignored.
type Matcher struct {
	matcher gitignore.Matcher
}

// Options tunes which pattern sources a Matcher loads. The zero value loads
// built-in defaults and .aideignore only.
type Options struct {
	// Gitignore layers the repository's own ignore rules between the built-in
	// defaults and .aideignore. It is a no-op when projectRoot has no .git.
	Gitignore bool
}

// BuiltinDefaults are patterns applied even when no .aideignore file exists.
// They cover the superset of all previously-hardcoded skip-dir lists plus
// common generated-file patterns.
var BuiltinDefaults = []string{
	// ── Version control ──────────────────────────────────────────────
	".git/",
	".svn/",
	".hg/",

	// ── Aide internal ────────────────────────────────────────────────
	".aide/",

	// ── Node / JavaScript / TypeScript ───────────────────────────────
	"node_modules/",
	"dist/",
	".next/",
	".nuxt/",
	"coverage/",
	".cache/",

	// ── Python ───────────────────────────────────────────────────────
	"__pycache__/",
	".venv/",
	"venv/",
	".tox/",
	".mypy_cache/",
	".pytest_cache/",
	"*.egg-info/",
	"site-packages/",

	// ── Go ───────────────────────────────────────────────────────────
	"vendor/",

	// ── Rust ─────────────────────────────────────────────────────────
	"target/",

	// ── Java / Kotlin / Gradle ───────────────────────────────────────
	"build/",
	".gradle/",
	"out/",

	// ── C / C++ ──────────────────────────────────────────────────────
	"cmake-build-debug/",
	"cmake-build-release/",
	".cmake/",
	".deps/",
	"Debug/",
	"Release/",

	// ── Ruby ─────────────────────────────────────────────────────────
	".bundle/",

	// ── C# ───────────────────────────────────────────────────────────
	"bin/",
	"obj/",

	// ── Elixir ───────────────────────────────────────────────────────
	"_build/",
	"deps/",

	// ── OCaml ────────────────────────────────────────────────────────
	"_opam/",

	// ── Scala ────────────────────────────────────────────────────────
	".bloop/",
	".metals/",

	// ── Swift ────────────────────────────────────────────────────────
	".build/",

	// ── IDE / Editor ─────────────────────────────────────────────────
	".idea/",
	".vscode/",

	// ── OS artefacts ─────────────────────────────────────────────────
	".DS_Store",

	// ── Generated code (common noise in findings) ────────────────────
	"*.pb.go",
	"*_generated.go",
	"*.gen.go",
	"*.pb.ts",
	"*.pb.js",

	// ── Minified / bundled assets (third-party, not human-edited) ────
	"*.min.js",
	"*.min.css",
	"*.min.mjs",
	"*.bundle.js",
	"*.bundle.css",
	"*.map",
	"*.js.map",
	"*.css.map",

	// ── Vendored third-party dirs by convention ──────────────────────
	"**/third_party/",
	"**/third-party/",
	"**/thirdparty/",
	"**/external/",
	"**/contrib/",

	// ── Web-asset vendored subdirs (Django/Flask/Rails conventions) ──
	"**/static/vendor/",
	"**/static/lib/",
	"**/static/libs/",
	"**/public/vendor/",
	"**/public/lib/",
	"**/public/libs/",
	"**/assets/vendor/",
	"**/assets/lib/",
	"**/assets/libs/",

	// ── Test fixtures (embedded secrets, high-complexity samples) ────
	"**/testdata/",
	"**/fixtures/",

	// ── Test files (reduce clone noise from repeated patterns) ───────
	"*_test.go",

	// ── Lock / binary / archive (not useful for analysis) ────────────
	"*.lock",
}

// New creates a Matcher for a project root, reading the gitignore layer
// according to code.respect_gitignore (default on). Callers that must pin the
// behaviour regardless of user config should use NewWithOptions.
func New(projectRoot string) (*Matcher, error) {
	return NewWithOptions(projectRoot, Options{
		Gitignore: config.Get().Code.RespectGitignoreEnabled(),
	})
}

// NewWithOptions creates a Matcher from built-in defaults, optionally the
// repository's gitignore rules, and an optional <projectRoot>/.aideignore
// file. A missing .aideignore is not an error — the Matcher still works using
// the layers below it.
func NewWithOptions(projectRoot string, opts Options) (*Matcher, error) {
	// Ordered lowest-priority first: go-git's matcher walks the slice in
	// reverse and stops at the first pattern that matches, so appending later
	// means overriding earlier. .aideignore going last is what lets a "!" line
	// there re-include something a builtin or a .gitignore excluded.
	ps := builtinPatterns()

	if opts.Gitignore {
		ps = append(ps, gitignorePatterns(projectRoot)...)
	}

	userPS, err := readPatternFile(filepath.Join(projectRoot, IgnoreFileName), nil)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	ps = append(ps, userPS...)

	return newMatcher(ps), nil
}

// NewFromDefaults creates a Matcher using only built-in defaults (no file,
// no gitignore layer).
func NewFromDefaults() *Matcher {
	return newMatcher(builtinPatterns())
}

// NewEmpty creates a Matcher with no patterns at all — nothing is ignored.
// Use this in tests that need to scan testdata or other normally-excluded paths.
func NewEmpty() *Matcher {
	return newMatcher(nil)
}

func newMatcher(ps []gitignore.Pattern) *Matcher {
	return &Matcher{matcher: gitignore.NewMatcher(ps)}
}

// newFromPatternStrings builds a Matcher from raw pattern strings rooted at the
// project root. Used by tests to exercise individual patterns in isolation.
func newFromPatternStrings(patterns ...string) *Matcher {
	ps := make([]gitignore.Pattern, 0, len(patterns))
	for _, p := range patterns {
		ps = append(ps, gitignore.ParsePattern(p, nil))
	}
	return newMatcher(ps)
}

var (
	builtinOnce sync.Once
	builtinPS   []gitignore.Pattern
)

func builtinPatterns() []gitignore.Pattern {
	builtinOnce.Do(func() {
		builtinPS = make([]gitignore.Pattern, 0, len(BuiltinDefaults))
		for _, p := range BuiltinDefaults {
			builtinPS = append(builtinPS, gitignore.ParsePattern(p, nil))
		}
	})
	// Copied so a caller appending later layers cannot write into the shared
	// backing array.
	out := make([]gitignore.Pattern, len(builtinPS))
	copy(out, builtinPS)
	return out
}

// ShouldIgnore reports whether the given path (relative to the project root)
// should be ignored. isDir must be true when path refers to a directory.
//
// The path should use forward slashes and be relative to the project root.
// Both "foo/bar" and "foo/bar/" are accepted for directories (the trailing
// slash is stripped internally; use the isDir flag instead).
func (m *Matcher) ShouldIgnore(path string, isDir bool) bool {
	path = filepath.ToSlash(path)
	path = strings.TrimSuffix(path, "/")

	if path == "" || path == "." {
		return false
	}

	return m.matcher.Match(strings.Split(path, "/"), isDir)
}

// ShouldIgnoreDir is a convenience for ShouldIgnore(path, true).
func (m *Matcher) ShouldIgnoreDir(path string) bool {
	return m.ShouldIgnore(path, true)
}

// ShouldIgnoreFile is a convenience for ShouldIgnore(path, false).
func (m *Matcher) ShouldIgnoreFile(path string) bool {
	return m.ShouldIgnore(path, false)
}

// WalkFunc returns a filepath.WalkFunc skip-check for use inside
// filepath.Walk callbacks. It converts absolute paths to relative paths
// using projectRoot.
//
// Usage:
//
//	shouldSkip := matcher.WalkFunc(projectRoot)
//	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
//	    if skip, skipDir := shouldSkip(path, info); skip {
//	        if skipDir { return filepath.SkipDir }
//	        return nil
//	    }
//	    // ... process file ...
//	})
func (m *Matcher) WalkFunc(projectRoot string) func(path string, info os.FileInfo) (skip bool, skipDir bool) {
	return func(path string, info os.FileInfo) (bool, bool) {
		rel, err := filepath.Rel(projectRoot, path)
		if err != nil {
			rel = path
		}

		isDir := info != nil && info.IsDir()
		if m.ShouldIgnore(rel, isDir) {
			if isDir {
				return true, true // skip this directory entirely
			}
			return true, false // skip this file
		}
		return false, false
	}
}

// readPatternFile parses one ignore file into patterns rooted at domain. It
// mirrors go-git's unexported readIgnoreFile. Unlike git, leading whitespace is
// trimmed — .aideignore has always been lenient about indented patterns.
func readPatternFile(path string, domain []string) ([]gitignore.Pattern, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var ps []gitignore.Pattern
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		ps = append(ps, gitignore.ParsePattern(line, domain))
	}
	return ps, scanner.Err()
}

// gitignoreTTL bounds how long a cached gitignore scan is reused. Long enough
// that a findings run — which builds a Matcher per analyser — walks the tree
// once, short enough that editing .gitignore takes effect in a running daemon
// without a restart.
const gitignoreTTL = 30 * time.Second

type gitignoreEntry struct {
	patterns []gitignore.Pattern
	loadedAt time.Time
}

var (
	gitignoreMu    sync.Mutex
	gitignoreCache = map[string]gitignoreEntry{}
)

// gitignorePatterns reads .git/info/exclude and every .gitignore in the tree
// below projectRoot, in git's own precedence order.
//
// Deliberately excluded: the global (core.excludesfile) and system ignore
// files. Those live outside the repository and differ per machine, so folding
// them in would let the same tree produce different findings on different
// checkouts with nothing in the repo to explain why.
//
// Errors are swallowed rather than surfaced: an unreadable ignore file should
// degrade to "ignore less", never fail a scan. ReadPatterns also returns the
// patterns it collected before an error, so partial results are worth keeping.
func gitignorePatterns(projectRoot string) []gitignore.Pattern {
	// A project root that is not a worktree root has nothing to read, and
	// ReadPatterns would walk the whole tree to discover that. This also means
	// a projectRoot nested inside a repo does not inherit its parents' rules.
	if _, err := os.Stat(filepath.Join(projectRoot, ".git")); err != nil {
		return nil
	}

	key, err := filepath.Abs(projectRoot)
	if err != nil {
		key = projectRoot
	}

	gitignoreMu.Lock()
	defer gitignoreMu.Unlock()

	if e, ok := gitignoreCache[key]; ok && time.Since(e.loadedAt) < gitignoreTTL {
		return e.patterns
	}

	// ReadPatterns prunes as it recurses — a directory already excluded by the
	// patterns collected so far is not descended into — so this does not walk
	// node_modules or any other gitignored tree.
	ps, _ := gitignore.ReadPatterns(osfs.New(key), nil)
	gitignoreCache[key] = gitignoreEntry{patterns: ps, loadedAt: time.Now()}
	return ps
}

// InvalidateGitignoreCache drops every cached gitignore scan. Call it after
// knowingly rewriting a .gitignore rather than waiting out gitignoreTTL.
func InvalidateGitignoreCache() {
	gitignoreMu.Lock()
	gitignoreCache = map[string]gitignoreEntry{}
	gitignoreMu.Unlock()
}
