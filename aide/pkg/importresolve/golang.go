package importresolve

import (
	"bufio"
	"os"
	"path"
	"sort"
	"strings"
)

type goModule struct {
	modPath string // module path from the go.mod "module" directive
	dir     string // project-relative directory of the go.mod ("" = root)
}

// goResolver maps Go import paths to package directories via longest-prefix
// match against the module paths discovered from go.mod files. The unit of
// Go coupling is the package: imports and import cycles are package-level.
type goResolver struct {
	fs      *projectFS
	modules []goModule
}

func newGoResolver(pfs *projectFS) *goResolver {
	return &goResolver{fs: pfs}
}

func (g *goResolver) languages() []string { return []string{"go"} }
func (g *goResolver) manifests() []string { return []string{"go.mod"} }

func (g *goResolver) addManifest(relDir, absPath string) {
	if mod := goModulePath(absPath); mod != "" {
		g.modules = append(g.modules, goModule{modPath: mod, dir: relDir})
	}
}

// finalize orders modules longest path first so nested modules win prefix
// matching over their parent.
func (g *goResolver) finalize() {
	sort.Slice(g.modules, func(i, j int) bool {
		if len(g.modules[i].modPath) != len(g.modules[j].modPath) {
			return len(g.modules[i].modPath) > len(g.modules[j].modPath)
		}
		return g.modules[i].modPath < g.modules[j].modPath
	})
}

// resolve returns the package directory an import path lands on. Stdlib and
// third-party imports match no module and resolve to "".
func (g *goResolver) resolve(_ string, imp string) string {
	for _, mod := range g.modules {
		var sub string
		switch {
		case imp == mod.modPath:
			sub = ""
		case strings.HasPrefix(imp, mod.modPath+"/"):
			sub = imp[len(mod.modPath)+1:]
		default:
			continue
		}
		pkgDir := mod.dir
		if sub != "" {
			pkgDir = path.Join(mod.dir, sub)
		}
		if pkgDir == "" {
			pkgDir = "."
		}
		if g.dirHasGoFiles(pkgDir) {
			return pkgDir
		}
	}
	return ""
}

func (g *goResolver) unitOf(file string) string {
	return path.Dir(file)
}

// resolveFiles fans a package import out to the package's non-test .go files.
func (g *goResolver) resolveFiles(fromFile, imp string) []string {
	pkgDir := g.resolve(fromFile, imp)
	if pkgDir == "" {
		return nil
	}
	var files []string
	for _, name := range g.fs.listFiles(pkgDir) {
		if strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go") {
			files = append(files, path.Join(pkgDir, name))
		}
	}
	return files
}

// dirHasGoFiles reports whether rel contains at least one non-test .go file —
// the check that a longest-prefix match actually landed on a real package.
func (g *goResolver) dirHasGoFiles(rel string) bool {
	for _, name := range g.fs.listFiles(rel) {
		if strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go") {
			return true
		}
	}
	return false
}

// goModulePath extracts the module directive from a go.mod file.
func goModulePath(goModFile string) string {
	f, err := os.Open(goModFile)
	if err != nil {
		return ""
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if rest, ok := strings.CutPrefix(line, "module "); ok {
			return strings.TrimSpace(rest)
		}
	}
	return ""
}
