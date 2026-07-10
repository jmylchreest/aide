package importresolve

import (
	"encoding/json"
	"os"
	"path"
	"strings"
)

var tsSourceExts = []string{".ts", ".tsx", ".mts", ".cts", ".js", ".jsx", ".mjs", ".cjs"}

// tsExtSwaps maps a JS extension written in an import specifier to the TS
// extensions it may compile from — NodeNext-style projects import './foo.js'
// for a file that exists on disk as foo.ts.
var tsExtSwaps = map[string][]string{
	".js":  {".ts", ".tsx"},
	".mjs": {".mts"},
	".cjs": {".cts"},
	".jsx": {".tsx"},
}

// tsResolver maps TS/JS import specifiers to source files. Relative
// specifiers are probed against the importing file's directory; bare
// specifiers are matched against workspace package.json names (with optional
// subpath). npm dependencies, node builtins, and tsconfig path aliases
// resolve to "".
type tsResolver struct {
	fs       *projectFS
	packages map[string]string // package.json name -> project-relative dir
}

func newTSResolver(pfs *projectFS) *tsResolver {
	return &tsResolver{fs: pfs, packages: make(map[string]string)}
}

func (t *tsResolver) languages() []string {
	return []string{"typescript", "javascript"}
}
func (t *tsResolver) manifests() []string { return []string{"package.json"} }

func (t *tsResolver) addManifest(relDir, absPath string) {
	name := packageJSONName(absPath)
	if name == "" {
		return
	}
	if _, exists := t.packages[name]; !exists {
		t.packages[name] = relDir
	}
}

func (t *tsResolver) finalize() {}

func (t *tsResolver) resolve(fromFile, imp string) string {
	if strings.HasPrefix(imp, "./") || strings.HasPrefix(imp, "../") || imp == "." || imp == ".." {
		cand := path.Join(path.Dir(fromFile), imp)
		if strings.HasPrefix(cand, "../") {
			return "" // escapes the project root
		}
		return t.probeModule(cand)
	}

	if pkgDir, ok := t.packages[imp]; ok {
		return t.probeEntry(pkgDir)
	}
	if name, sub := splitBareSpecifier(imp); sub != "" {
		if pkgDir, ok := t.packages[name]; ok {
			if u := t.probeModule(path.Join(pkgDir, sub)); u != "" {
				return u
			}
			// Workspace subpath exports conventionally map into src/.
			return t.probeModule(path.Join(pkgDir, "src", sub))
		}
	}
	return ""
}

func (t *tsResolver) unitOf(file string) string { return file }

// splitBareSpecifier separates a bare import into package name and subpath.
// Scoped packages (@scope/name/sub) keep two segments in the name.
func splitBareSpecifier(imp string) (name, sub string) {
	i := strings.Index(imp, "/")
	if i <= 0 {
		return imp, ""
	}
	if imp[0] == '@' {
		j := strings.Index(imp[i+1:], "/")
		if j < 0 {
			return imp, ""
		}
		return imp[:i+1+j], imp[i+j+2:]
	}
	return imp[:i], imp[i+1:]
}

// probeModule finds the source file a module specifier lands on: the exact
// file, an extension-swapped TS sibling, extension probing, or a directory
// index file.
func (t *tsResolver) probeModule(cand string) string {
	ext := path.Ext(cand)
	if swaps, ok := tsExtSwaps[ext]; ok {
		base := strings.TrimSuffix(cand, ext)
		for _, swap := range swaps {
			if t.fs.fileExists(base + swap) {
				return base + swap
			}
		}
	}
	if ext != "" && t.fs.fileExists(cand) {
		return cand
	}
	for _, e := range tsSourceExts {
		if t.fs.fileExists(cand + e) {
			return cand + e
		}
	}
	for _, e := range tsSourceExts {
		if idx := path.Join(cand, "index"+e); t.fs.fileExists(idx) {
			return idx
		}
	}
	return ""
}

// probeEntry finds a workspace package's entry source without parsing its
// package.json main/module fields: conventional src/index then index.
func (t *tsResolver) probeEntry(pkgDir string) string {
	if u := t.probeModule(path.Join(pkgDir, "src", "index")); u != "" {
		return u
	}
	return t.probeModule(path.Join(pkgDir, "index"))
}

// packageJSONName extracts the "name" field from a package.json.
func packageJSONName(pkgJSONFile string) string {
	data, err := os.ReadFile(pkgJSONFile)
	if err != nil {
		return ""
	}
	var pkg struct {
		Name string `json:"name"`
	}
	if json.Unmarshal(data, &pkg) != nil {
		return ""
	}
	return pkg.Name
}
