package importresolve

import (
	"bufio"
	"os"
	"path"
	"sort"
	"strings"
)

// rustResolver maps Rust use-paths to source files. Use-paths are module
// paths, not file paths: the conventional mapping (mod foo → foo.rs or
// foo/mod.rs, crate root → src/lib.rs|main.rs) covers most real code, and a
// use-path's trailing segments are often items or inline modules — probing
// progressively shorter prefixes attributes those to the file that declares
// them. #[path] attributes and re-export chains fall through to "" (external).
//
// Crate names come from Cargo.toml [package] sections; hyphens are declared
// in manifests but referenced as underscores in code, so lookups normalize.
type rustResolver struct {
	fs      *projectFS
	byName  map[string]string // underscored crate name -> src dir
	srcDirs []string          // all crate src dirs, longest first
}

func newRustResolver(pfs *projectFS) *rustResolver {
	return &rustResolver{fs: pfs, byName: make(map[string]string)}
}

func (r *rustResolver) languages() []string { return []string{"rust"} }
func (r *rustResolver) manifests() []string { return []string{"Cargo.toml"} }

func (r *rustResolver) addManifest(relDir, absPath string) {
	name := cargoPackageName(absPath)
	if name == "" {
		return // workspace-only manifest
	}
	srcDir := path.Join(relDir, "src")
	if relDir == "" {
		srcDir = "src"
	}
	if !r.fs.dirExists(srcDir) {
		return
	}
	key := strings.ReplaceAll(name, "-", "_")
	if _, exists := r.byName[key]; !exists {
		r.byName[key] = srcDir
	}
	r.srcDirs = append(r.srcDirs, srcDir)
}

func (r *rustResolver) finalize() {
	sort.Slice(r.srcDirs, func(i, j int) bool {
		if len(r.srcDirs[i]) != len(r.srcDirs[j]) {
			return len(r.srcDirs[i]) > len(r.srcDirs[j])
		}
		return r.srcDirs[i] < r.srcDirs[j]
	})
}

func (r *rustResolver) resolve(fromFile, imp string) string {
	imp = strings.Trim(imp, ":")
	if imp == "" {
		return ""
	}
	segs := strings.Split(imp, "::")

	switch segs[0] {
	case "crate":
		if src := r.crateSrcOf(fromFile); src != "" {
			return r.probeChain(src, segs[1:], true)
		}
		return ""
	case "self":
		return r.probeChain(moduleDir(fromFile), segs[1:], true)
	case "super":
		base := moduleDir(fromFile)
		for len(segs) > 0 && segs[0] == "super" {
			base = path.Dir(base)
			segs = segs[1:]
		}
		if strings.HasPrefix(base, "../") {
			return ""
		}
		return r.probeChain(base, segs, true)
	case "std", "core", "alloc":
		return ""
	}

	// Workspace crate by name (use other_crate::x).
	if src, ok := r.byName[segs[0]]; ok {
		return r.probeChain(src, segs[1:], true)
	}

	// The import-extraction regex may strip a leading "crate::", making an
	// internal path look bare. Probing decides: a hit inside the current
	// crate is internal, anything else is an external dependency. The crate
	// root is NOT allowed as a landing spot here — with allowRoot an
	// external crate like serde would false-resolve to lib.rs/main.rs.
	if src := r.crateSrcOf(fromFile); src != "" {
		if u := r.probeChain(src, segs, false); u != "" {
			return u
		}
	}
	return ""
}

// moduleDir returns the directory representing a file's own module scope:
// x/y.rs is module y whose children live under x/y/; x/mod.rs (and the crate
// roots lib.rs/main.rs) already sit in their module's directory.
func moduleDir(file string) string {
	base := path.Base(file)
	if base == "mod.rs" || base == "lib.rs" || base == "main.rs" {
		return path.Dir(file)
	}
	return strings.TrimSuffix(file, ".rs")
}

func (r *rustResolver) unitOf(file string) string { return file }

func (r *rustResolver) resolveFiles(fromFile, imp string) []string {
	if f := r.resolve(fromFile, imp); f != "" {
		return []string{f}
	}
	return nil
}

// crateSrcOf returns the src dir of the crate containing file — the longest
// src dir that is a path prefix of it.
func (r *rustResolver) crateSrcOf(file string) string {
	for _, src := range r.srcDirs {
		if strings.HasPrefix(file, src+"/") {
			return src
		}
	}
	return ""
}

// probeChain resolves module path segments under base, dropping trailing
// segments until a file matches: `use a::b::Item` lands on a/b.rs, a/b/mod.rs,
// or a.rs (inline module b). With allowRoot, an exhausted chain lands on the
// module file for base itself (lib.rs/main.rs/mod.rs) — correct only when the
// path was explicitly anchored (crate::, a crate name, self/super), since for
// a bare path it would claim every external crate as internal.
func (r *rustResolver) probeChain(base string, segs []string, allowRoot bool) string {
	for k := len(segs); k >= 1; k-- {
		cand := path.Join(base, strings.Join(segs[:k], "/"))
		if f := cand + ".rs"; r.fs.fileExists(f) {
			return f
		}
		if f := path.Join(cand, "mod.rs"); r.fs.fileExists(f) {
			return f
		}
	}
	if allowRoot {
		for _, rootFile := range []string{"lib.rs", "main.rs", "mod.rs"} {
			if f := path.Join(base, rootFile); r.fs.fileExists(f) {
				return f
			}
		}
	}
	return ""
}

// cargoPackageName extracts the name from a Cargo.toml [package] section.
func cargoPackageName(cargoFile string) string {
	f, err := os.Open(cargoFile)
	if err != nil {
		return ""
	}
	defer f.Close()

	inPackage := false
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "[") {
			inPackage = line == "[package]"
			continue
		}
		if !inPackage {
			continue
		}
		if rest, ok := strings.CutPrefix(line, "name"); ok {
			rest = strings.TrimSpace(rest)
			if rest, ok = strings.CutPrefix(rest, "="); ok {
				return strings.Trim(strings.TrimSpace(rest), `"'`)
			}
		}
	}
	return ""
}
