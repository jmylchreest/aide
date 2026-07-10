package importresolve

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// csNamespaceRe matches both block-scoped and file-scoped namespace
// declarations near the top of a C# file.
var csNamespaceRe = regexp.MustCompile(`(?m)^\s*namespace\s+([A-Za-z0-9_.]+)`)

// csHeaderReadCap bounds how much of each .cs file is read looking for its
// namespace declaration — it sits near the top; reading whole files would
// make the scan O(codebase).
const csHeaderReadCap = 8 * 1024

// csResolver maps C# using-directives to namespaces. Namespaces have no
// required filesystem correspondence — any file anywhere can declare
// `namespace MyApp.Services`, and one namespace usually spans many files
// (like Go packages). Folder-convention probing would guess wrong on
// non-conventional repos, so resolution is driven by a namespace pre-scan
// of .cs file headers instead, and the unit IS the namespace string.
// Framework namespaces (System.*, third-party) appear in no scanned file
// and resolve to "".
type csResolver struct {
	fs      *projectFS
	nsSeen  map[string]bool     // namespaces declared somewhere in the project
	fileNS  map[string]string   // .cs file -> its declared namespace
	nsFiles map[string][]string // namespace -> files declaring exactly it
}

func newCSResolver(pfs *projectFS) *csResolver {
	return &csResolver{
		fs:      pfs,
		nsSeen:  make(map[string]bool),
		fileNS:  make(map[string]string),
		nsFiles: make(map[string][]string),
	}
}

func (c *csResolver) languages() []string { return []string{"csharp"} }
func (c *csResolver) manifests() []string { return []string{"*.cs"} }

func (c *csResolver) addManifest(relDir, absPath string) {
	base := filepath.Base(absPath)
	if !strings.HasSuffix(base, ".cs") {
		return
	}
	ns := csFileNamespace(absPath)
	if ns == "" {
		return
	}
	rel := base
	if relDir != "" {
		rel = relDir + "/" + base
	}
	c.fileNS[rel] = ns
	c.nsFiles[ns] = append(c.nsFiles[ns], rel)
	// A declaration of a.b.c also makes ancestors a.b and a real namespaces.
	for ns != "" {
		c.nsSeen[ns] = true
		if i := strings.LastIndexByte(ns, '.'); i >= 0 {
			ns = ns[:i]
		} else {
			ns = ""
		}
	}
}

// finalize orders each namespace's file list — WalkDir feeds addManifest in
// lexical order already, but sorting here keeps that a guarantee rather
// than an accident.
func (c *csResolver) finalize() {
	for _, files := range c.nsFiles {
		sort.Strings(files)
	}
}

// resolve maps a using-directive to the namespace unit it names. `using
// static a.b.Class` drops trailing segments until a declared namespace
// matches.
func (c *csResolver) resolve(_ string, imp string) string {
	imp = strings.Trim(imp, ".")
	for imp != "" {
		if c.nsSeen[imp] {
			return imp
		}
		i := strings.LastIndexByte(imp, '.')
		if i < 0 {
			return ""
		}
		imp = imp[:i]
	}
	return ""
}

// resolveFiles fans a namespace out to the files declaring exactly it,
// including descendants when the using names an ancestor namespace.
func (c *csResolver) resolveFiles(fromFile, imp string) []string {
	ns := c.resolve(fromFile, imp)
	if ns == "" {
		return nil
	}
	if files := c.nsFiles[ns]; len(files) > 0 {
		return files
	}
	// Ancestor namespace with no direct declarations: gather descendants.
	var files []string
	prefix := ns + "."
	descendants := make([]string, 0)
	for child := range c.nsFiles {
		if strings.HasPrefix(child, prefix) {
			descendants = append(descendants, child)
		}
	}
	sort.Strings(descendants)
	for _, child := range descendants {
		files = append(files, c.nsFiles[child]...)
	}
	return files
}

// unitOf returns the file's declared namespace, so intra-namespace usings
// collapse to self-edges and cross-namespace coupling aggregates the way C#
// code actually organizes.
func (c *csResolver) unitOf(file string) string {
	if ns, ok := c.fileNS[file]; ok {
		return ns
	}
	return file
}

// csFileNamespace extracts the first namespace declaration from a .cs file
// header.
func csFileNamespace(absPath string) string {
	f, err := os.Open(absPath)
	if err != nil {
		return ""
	}
	defer f.Close()
	buf := make([]byte, csHeaderReadCap)
	n, _ := f.Read(buf)
	m := csNamespaceRe.FindSubmatch(buf[:n])
	if m == nil {
		return ""
	}
	return string(m[1])
}
