// Package aideignore provides gitignore-compatible file matching for aide.
//
// It loads patterns from a project's .aideignore file (if present), merges them
// with built-in defaults for generated code, build artifacts, and common
// non-source directories, and exposes a single ShouldIgnore method used by all
// findings analysers, the file watcher, and the Runner.
//
// Pattern syntax mirrors .gitignore:
//
//	# comment
//	*.pb.go          — match files by extension
//	vendor/          — match directories by name (trailing slash)
//	**/test/         — match at any depth
//	!important.go    — negate a previous pattern
//	build/           — directory name anywhere in tree
//	/rootonly        — anchored to project root (leading slash)
package aideignore

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// Matcher tests whether a path should be ignored.
type Matcher struct {
	rules []rule
}

type rule struct {
	pattern  string
	negation bool
	dirOnly  bool
	anchored bool // pattern contains '/' (other than trailing) — anchored to root
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

	// ── Test fixtures (embedded secrets, high-complexity samples) ────
	"**/testdata/",
	"**/fixtures/",

	// ── Test files (reduce clone noise from repeated patterns) ───────
	"*_test.go",

	// ── Lock / binary / archive (not useful for analysis) ────────────
	"*.lock",
}

// New creates a Matcher from built-in defaults plus an optional .aideignore
// file located at <projectRoot>/.aideignore. If the file does not exist the
// Matcher still works using only built-in defaults.
func New(projectRoot string) (*Matcher, error) {
	m := &Matcher{}

	// Load built-in defaults first (lowest priority).
	for _, p := range BuiltinDefaults {
		m.rules = append(m.rules, parsePattern(p))
	}

	// Load user overrides from .aideignore (higher priority — can negate builtins).
	ignoreFile := filepath.Join(projectRoot, ".aideignore")
	if err := m.loadFile(ignoreFile); err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	return m, nil
}

// NewFromDefaults creates a Matcher using only built-in defaults (no file).
func NewFromDefaults() *Matcher {
	m := &Matcher{}
	for _, p := range BuiltinDefaults {
		m.rules = append(m.rules, parsePattern(p))
	}
	return m
}

// NewEmpty creates a Matcher with no rules at all — nothing is ignored.
// Use this in tests that need to scan testdata or other normally-excluded paths.
func NewEmpty() *Matcher {
	return &Matcher{}
}

// ShouldIgnore reports whether the given path (relative to the project root)
// should be ignored. isDir must be true when path refers to a directory.
//
// The path should use forward slashes and be relative to the project root.
// Both "foo/bar" and "foo/bar/" are accepted for directories (the trailing
// slash is stripped internally; use the isDir flag instead).
func (m *Matcher) ShouldIgnore(path string, isDir bool) bool {
	// Normalise to forward slashes and strip any trailing slash.
	path = filepath.ToSlash(path)
	path = strings.TrimSuffix(path, "/")

	if path == "" || path == "." {
		return false
	}

	// Evaluate rules in order — last matching rule wins.
	ignored := false
	matched := false // whether any rule matched this exact path
	for _, r := range m.rules {
		if r.dirOnly && !isDir {
			continue
		}
		if r.match(path) {
			ignored = !r.negation
			matched = true
		}
	}

	if ignored {
		return true
	}

	// If a rule explicitly un-ignored this file (negation), respect that
	// and skip the parent directory check. This allows patterns like
	// "!testdata/important.txt" to override "testdata/".
	if matched {
		return false
	}

	// If the path is a file (not a directory), also check whether any parent
	// directory is ignored. This handles the case where OnChanges receives
	// individual file paths like "vendor/github.com/foo/bar.go" — the
	// dir-only pattern "vendor/" should cause this file to be ignored even
	// though filepath.Walk would normally skip the directory before reaching
	// the file.
	if !isDir {
		parts := strings.Split(path, "/")
		for i := 1; i <= len(parts)-1; i++ {
			parent := strings.Join(parts[:i], "/")
			if m.ShouldIgnore(parent, true) {
				return true
			}
		}
	}

	return false
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

// loadFile reads patterns from a .aideignore file.
func (m *Matcher) loadFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments.
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		m.rules = append(m.rules, parsePattern(line))
	}
	return scanner.Err()
}

// parsePattern converts a gitignore-style pattern string into a rule.
func parsePattern(pattern string) rule {
	r := rule{}

	// Handle negation.
	if strings.HasPrefix(pattern, "!") {
		r.negation = true
		pattern = pattern[1:]
	}

	// Handle trailing slash (directory-only pattern).
	if strings.HasSuffix(pattern, "/") {
		r.dirOnly = true
		pattern = strings.TrimSuffix(pattern, "/")
	}

	// Handle leading slash (anchored to root).
	if strings.HasPrefix(pattern, "/") {
		r.anchored = true
		pattern = strings.TrimPrefix(pattern, "/")
	}

	// A pattern is also anchored if it contains a slash anywhere (like "foo/bar").
	// Patterns without slashes match against the basename at any depth.
	if !r.anchored && strings.Contains(pattern, "/") {
		// Contains a slash but not leading — still anchored to root per gitignore rules.
		r.anchored = true
	}

	r.pattern = pattern
	return r
}

// match tests whether a rule matches the given path.
// path is relative to the project root, forward-slash separated, no trailing slash.
func (r *rule) match(path string) bool {
	pattern := r.pattern

	// Handle ** prefix: matches zero or more directories.
	if strings.HasPrefix(pattern, "**/") {
		// "**/<rest>" matches <rest> at any depth.
		rest := pattern[3:]
		return matchGlob(rest, path) || matchGlob(rest, basename(path)) || matchPathSuffix(rest, path)
	}

	// Handle ** suffix: "<prefix>/**" matches everything under prefix.
	if strings.HasSuffix(pattern, "/**") {
		prefix := pattern[:len(pattern)-3]
		return path == prefix || strings.HasPrefix(path, prefix+"/")
	}

	// Handle interior **: "a/**/b" matches a/b, a/x/b, a/x/y/b, etc.
	if strings.Contains(pattern, "/**/") {
		parts := strings.SplitN(pattern, "/**/", 2)
		// Left part must match a prefix, right part must match the suffix.
		if matchGlob(parts[0], path) {
			return true
		}
		// Check: left prefix + any middle + right suffix.
		return matchDoublestar(parts[0], parts[1], path)
	}

	if r.anchored {
		// Anchored: pattern must match from the root.
		return matchGlob(pattern, path)
	}

	// Unanchored: pattern matches against basename or any path component.
	if matchGlob(pattern, basename(path)) {
		return true
	}
	// Also check the full path for patterns like "*.pb.go" that should match
	// "foo/bar.pb.go".
	return matchGlob(pattern, path)
}

// matchGlob performs filepath.Match but segment-by-segment for patterns
// containing "/", so that "foo/*.go" properly matches "foo/bar.go".
func matchGlob(pattern, name string) bool {
	// If pattern has no slash, simple match.
	if !strings.Contains(pattern, "/") {
		ok, _ := filepath.Match(pattern, name)
		return ok
	}

	// Match segment-by-segment.
	patParts := strings.Split(pattern, "/")
	nameParts := strings.Split(name, "/")

	if len(patParts) != len(nameParts) {
		return false
	}

	for i, pp := range patParts {
		ok, _ := filepath.Match(pp, nameParts[i])
		if !ok {
			return false
		}
	}
	return true
}

// matchPathSuffix checks if pattern matches any suffix of path split by "/".
// For example, pattern "test/*.go" matches path "a/b/test/foo.go".
func matchPathSuffix(pattern string, path string) bool {
	parts := strings.Split(path, "/")
	patParts := strings.Split(pattern, "/")

	if len(patParts) > len(parts) {
		return false
	}

	// Slide window over path parts.
	for i := 0; i <= len(parts)-len(patParts); i++ {
		candidate := strings.Join(parts[i:i+len(patParts)], "/")
		if matchGlob(pattern, candidate) {
			return true
		}
	}
	return false
}

// matchDoublestar matches "left/**/right" against path.
func matchDoublestar(left, right, path string) bool {
	parts := strings.Split(path, "/")
	// Find positions where left matches parts[:i] and right matches parts[j:]
	for i := 0; i <= len(parts); i++ {
		leftCandidate := strings.Join(parts[:i], "/")
		if !matchGlob(left, leftCandidate) {
			continue
		}
		for j := i; j <= len(parts); j++ {
			rightCandidate := strings.Join(parts[j:], "/")
			if matchGlob(right, rightCandidate) {
				return true
			}
		}
	}
	return false
}

// basename returns the last path component.
func basename(path string) string {
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[i+1:]
	}
	return path
}
