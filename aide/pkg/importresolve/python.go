package importresolve

import (
	"path"
	"strings"
)

// pyResolver maps Python module strings to source files. Leading dots are
// PEP 328 relative imports resolved against the importing file's package;
// absolute dotted modules are probed under each discovered source root.
// Stdlib and site-packages modules resolve to "".
type pyResolver struct {
	fs    *projectFS
	roots []string // project-relative python source roots
}

func newPythonResolver(pfs *projectFS) *pyResolver {
	return &pyResolver{fs: pfs, roots: []string{""}}
}

func (p *pyResolver) languages() []string { return []string{"python"} }
func (p *pyResolver) manifests() []string {
	return []string{"pyproject.toml", "setup.py", "setup.cfg"}
}

func (p *pyResolver) addManifest(relDir, _ string) {
	p.addRoot(relDir)
	p.addRoot(path.Join(relDir, "src"))
}

func (p *pyResolver) finalize() {}

func (p *pyResolver) resolve(fromFile, imp string) string {
	dots := 0
	for dots < len(imp) && imp[dots] == '.' {
		dots++
	}
	rest := imp[dots:]

	if dots > 0 {
		base := path.Dir(fromFile)
		for i := 1; i < dots; i++ {
			base = path.Dir(base)
		}
		cand := base
		if rest != "" {
			cand = path.Join(base, strings.ReplaceAll(rest, ".", "/"))
		}
		if strings.HasPrefix(cand, "../") {
			return ""
		}
		return p.probeModule(cand)
	}

	modPath := strings.ReplaceAll(imp, ".", "/")
	for _, root := range p.roots {
		if u := p.probeModule(path.Join(root, modPath)); u != "" {
			return u
		}
	}
	return ""
}

func (p *pyResolver) unitOf(file string) string { return file }

func (p *pyResolver) resolveFiles(fromFile, imp string) []string {
	if f := p.resolve(fromFile, imp); f != "" {
		return []string{f}
	}
	return nil
}

func (p *pyResolver) addRoot(rel string) {
	if rel != "" && !p.fs.dirExists(rel) {
		return
	}
	for _, existing := range p.roots {
		if existing == rel {
			return
		}
	}
	p.roots = append(p.roots, rel)
}

// probeModule resolves a module path to its file: a plain module or a
// package's __init__.py.
func (p *pyResolver) probeModule(cand string) string {
	if f := cand + ".py"; p.fs.fileExists(f) {
		return f
	}
	if f := path.Join(cand, "__init__.py"); p.fs.fileExists(f) {
		return f
	}
	return ""
}
