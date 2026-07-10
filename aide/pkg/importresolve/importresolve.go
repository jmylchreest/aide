// Package importresolve maps import strings to project-relative dependency
// units. Resolution happens at analysis time — the code index keeps storing
// raw import strings, and an unresolvable import returns "" (external), so
// the failure mode is a missing edge, never a guessed one.
//
// A "unit" is the granularity at which a language couples:
//   - Go: the package directory (imports and import cycles are package-level)
//   - TypeScript/JavaScript, Python: the target source file
//
// # Adding a language
//
// Implement languageResolver in a new file (see golang.go, typescript.go,
// python.go for the three existing shapes) and add its constructor to the
// list in newLanguageResolvers. Nothing else changes: the project scan feeds
// your resolver the manifests it asks for, and dispatch is automatic for the
// pack language names you declare.
//
// A Resolver is not safe for concurrent use; build one per analysis run.
package importresolve

import (
	"io/fs"
	"path/filepath"
	"strings"
)

// languageResolver resolves imports for one language family. Implementations
// receive manifest files during the project scan, then answer per-import
// resolution queries.
type languageResolver interface {
	// languages returns the grammar-pack language names this resolver
	// handles (e.g. "go", "typescript").
	languages() []string
	// manifests returns the filenames this resolver wants to see during
	// the project scan: exact names ("go.mod", "package.json") or
	// extension patterns ("*.csproj", "*.cs") for languages whose layout
	// facts live in variably-named or source files.
	manifests() []string
	// addManifest is called once per discovered manifest, with the
	// project-relative directory containing it ("" = root) and its
	// absolute path for reading. Parsing is best-effort: garbage in a
	// manifest means skip it, never fail the scan.
	addManifest(relDir, absPath string)
	// finalize is called after the scan completes, before any resolve call.
	finalize()
	// resolve maps a normalized import string in fromFile to a
	// project-relative unit, or "" for external/unresolvable.
	resolve(fromFile, importStr string) string
	// unitOf returns the unit a source file itself belongs to.
	unitOf(file string) string
}

// newLanguageResolvers constructs every supported language resolver.
// Registration point: add new languages here.
func newLanguageResolvers(pfs *projectFS) []languageResolver {
	return []languageResolver{
		newGoResolver(pfs),
		newTSResolver(pfs),
		newPythonResolver(pfs),
		newRustResolver(pfs),
		newJVMResolver(pfs),
		newCSResolver(pfs),
	}
}

// Resolver resolves import strings against a scanned project layout.
type Resolver struct {
	byLang map[string]languageResolver
}

// Directories never descended into during project scanning. Dot-directories
// are skipped separately.
var skipDirs = map[string]bool{
	"node_modules": true, "vendor": true, "dist": true, "build": true,
	"target": true, "__pycache__": true, "venv": true, "testdata": true,
	"obj": true, "bin": true,
}

// New scans rootDir for the manifests each language resolver asks for and
// returns a ready Resolver. Scanning is best-effort: unreadable files are
// skipped, and a project with no manifests still resolves layout-derivable
// imports (relative TS specifiers, root-anchored Python modules).
func New(rootDir string) *Resolver {
	pfs := newProjectFS(rootDir)
	resolvers := newLanguageResolvers(pfs)

	byManifest := make(map[string][]languageResolver)
	bySuffix := make(map[string][]languageResolver)
	byLang := make(map[string]languageResolver)
	for _, lr := range resolvers {
		for _, m := range lr.manifests() {
			if suffix, ok := strings.CutPrefix(m, "*"); ok {
				bySuffix[suffix] = append(bySuffix[suffix], lr)
				continue
			}
			byManifest[m] = append(byManifest[m], lr)
		}
		for _, lang := range lr.languages() {
			byLang[lang] = lr
		}
	}

	_ = filepath.WalkDir(rootDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if p != rootDir && (skipDirs[d.Name()] || strings.HasPrefix(d.Name(), ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		interested := byManifest[d.Name()]
		if ext := filepath.Ext(d.Name()); ext != "" {
			interested = append(interested, bySuffix[ext]...)
		}
		if len(interested) == 0 {
			return nil
		}
		rel, relErr := filepath.Rel(rootDir, filepath.Dir(p))
		if relErr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if rel == "." {
			rel = ""
		}
		for _, lr := range interested {
			lr.addManifest(rel, p)
		}
		return nil
	})

	for _, lr := range resolvers {
		lr.finalize()
	}
	return &Resolver{byLang: byLang}
}

// ResolveUnit maps an import string in fromFile (project-relative) to the
// project-relative unit it lands on, or "" when the import is external,
// unresolvable, or the language has no resolver.
func (r *Resolver) ResolveUnit(lang, fromFile, importStr string) string {
	lr, ok := r.byLang[lang]
	if !ok {
		return ""
	}
	imp := normalizeImport(importStr)
	if imp == "" {
		return ""
	}
	return lr.resolve(fromFile, imp)
}

// UnitOf returns the unit a source file itself belongs to: its package
// directory for Go, the file itself for languages without a resolver or
// whose unit is the file.
func (r *Resolver) UnitOf(lang, file string) string {
	file = filepath.ToSlash(file)
	if lr, ok := r.byLang[lang]; ok {
		return lr.unitOf(file)
	}
	return file
}

// normalizeImport trims whitespace and one layer of matching string quotes —
// tree-sitter captures for import paths include the quote characters
// (e.g. Go's interpreted_string_literal), regex captures do not.
func normalizeImport(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 {
		first, last := s[0], s[len(s)-1]
		if first == last && (first == '"' || first == '\'' || first == '`') {
			s = s[1 : len(s)-1]
		}
	}
	return s
}
