package importresolve

import (
	"os"
	"path/filepath"
	"testing"
)

// fixtureProject builds a multi-language project layout in a temp dir:
// a two-module Go workspace, a TS workspace with two packages, and a
// Python package with a src/ layout.
func fixtureProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	files := map[string]string{
		"go.mod":                 "module example.com/proj\n\ngo 1.22\n",
		"main.go":                "package main\n",
		"pkg/store/store.go":     "package store\n",
		"pkg/store/bolt_test.go": "package store\n",
		"pkg/empty/README.md":    "no go files here\n",
		"web/go.mod":             "module example.com/proj/web\n",
		"web/server/server.go":   "package server\n",

		"package.json":                   `{"name": "proj-root", "private": true}`,
		"src/lib/logger.ts":              "export const log = () => {}\n",
		"src/hooks/session.ts":           "import { log } from '../lib/logger.js'\n",
		"src/core/index.ts":              "export {}\n",
		"packages/plugin/package.json":   `{"name": "@proj/plugin"}`,
		"packages/plugin/src/index.ts":   "export {}\n",
		"packages/plugin/src/matcher.ts": "export {}\n",

		"pyproject.toml":           "[project]\nname = \"proj\"\n",
		"src/proj/__init__.py":     "",
		"src/proj/util.py":         "",
		"src/proj/sub/__init__.py": "",
		"tools/script.py":          "",
	}
	for rel, content := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}
	return root
}

func TestResolveGo(t *testing.T) {
	r := New(fixtureProject(t))

	cases := []struct {
		imp  string
		want string
	}{
		{`"example.com/proj/pkg/store"`, "pkg/store"}, // quoted, as tree-sitter captures it
		{"example.com/proj/pkg/store", "pkg/store"},   // unquoted, as regex captures it
		{"example.com/proj", "."},                     // module root package
		{"example.com/proj/web/server", "web/server"}, // nested module wins by longest prefix
		{"example.com/proj/pkg/empty", ""},            // dir exists but has no .go files
		{"example.com/proj/pkg/missing", ""},          // dir does not exist
		{"fmt", ""},                                   // stdlib
		{"github.com/other/dep", ""},                  // third-party
	}
	for _, c := range cases {
		if got := r.ResolveUnit("go", "main.go", c.imp); got != c.want {
			t.Errorf("ResolveUnit(go, %q) = %q, want %q", c.imp, got, c.want)
		}
	}

	if got := r.UnitOf("go", "pkg/store/store.go"); got != "pkg/store" {
		t.Errorf("UnitOf(go) = %q, want %q", got, "pkg/store")
	}
}

func TestResolveTypeScript(t *testing.T) {
	r := New(fixtureProject(t))

	cases := []struct {
		from, imp, want string
	}{
		{"src/hooks/session.ts", "../lib/logger", "src/lib/logger.ts"},                     // extension probing
		{"src/hooks/session.ts", "../lib/logger.js", "src/lib/logger.ts"},                  // NodeNext .js -> .ts
		{"src/hooks/session.ts", "../core", "src/core/index.ts"},                           // directory index
		{"src/hooks/session.ts", "./missing", ""},                                          // no such file
		{"src/hooks/session.ts", "../../../../etc/passwd", ""},                             // escapes root
		{"src/hooks/session.ts", "@proj/plugin", "packages/plugin/src/index.ts"},           // workspace entry
		{"src/hooks/session.ts", "@proj/plugin/matcher", "packages/plugin/src/matcher.ts"}, // workspace subpath
		{"src/hooks/session.ts", "react", ""},                                              // npm dependency
		{"src/hooks/session.ts", "node:fs", ""},                                            // node builtin
	}
	for _, c := range cases {
		if got := r.ResolveUnit("typescript", c.from, c.imp); got != c.want {
			t.Errorf("ResolveUnit(ts, %q from %q) = %q, want %q", c.imp, c.from, got, c.want)
		}
	}

	if got := r.UnitOf("typescript", "src/lib/logger.ts"); got != "src/lib/logger.ts" {
		t.Errorf("UnitOf(ts) = %q, want the file itself", got)
	}
}

func TestResolvePython(t *testing.T) {
	r := New(fixtureProject(t))

	cases := []struct {
		from, imp, want string
	}{
		{"tools/script.py", "proj.util", "src/proj/util.py"},        // src/ layout root
		{"tools/script.py", "proj", "src/proj/__init__.py"},         // package __init__
		{"tools/script.py", "proj.sub", "src/proj/sub/__init__.py"}, // subpackage
		{"src/proj/util.py", ".sub", "src/proj/sub/__init__.py"},    // relative import
		{"src/proj/sub/__init__.py", "..util", "src/proj/util.py"},  // parent-relative
		{"tools/script.py", "os", ""},                               // stdlib
		{"tools/script.py", "requests", ""},                         // site-packages
	}
	for _, c := range cases {
		if got := r.ResolveUnit("python", c.from, c.imp); got != c.want {
			t.Errorf("ResolveUnit(py, %q from %q) = %q, want %q", c.imp, c.from, got, c.want)
		}
	}
}

func TestResolveUnknownLanguage(t *testing.T) {
	r := New(fixtureProject(t))
	if got := r.ResolveUnit("rust", "src/main.rs", "crate::store"); got != "" {
		t.Errorf("unknown language resolved to %q, want empty", got)
	}
}
