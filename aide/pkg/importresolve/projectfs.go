package importresolve

import (
	"os"
	"path/filepath"
)

// projectFS answers existence and listing probes against the project root,
// caching results — resolution probes the same paths and directories over
// and over across imports.
type projectFS struct {
	root      string
	fileCache map[string]bool
	dirCache  map[string][]string // rel dir -> names of regular files, nil sentinel = missing dir
}

func newProjectFS(root string) *projectFS {
	return &projectFS{
		root:      root,
		fileCache: make(map[string]bool),
		dirCache:  make(map[string][]string),
	}
}

// fileExists reports whether rel is a regular file under the project root.
func (p *projectFS) fileExists(rel string) bool {
	if cached, ok := p.fileCache[rel]; ok {
		return cached
	}
	info, err := os.Stat(filepath.Join(p.root, filepath.FromSlash(rel)))
	exists := err == nil && info.Mode().IsRegular()
	p.fileCache[rel] = exists
	return exists
}

// dirExists reports whether rel is a directory under the project root.
func (p *projectFS) dirExists(rel string) bool {
	info, err := os.Stat(filepath.Join(p.root, filepath.FromSlash(rel)))
	return err == nil && info.IsDir()
}

// listFiles returns the names of regular files directly inside rel
// (nil for a missing/unreadable directory).
func (p *projectFS) listFiles(rel string) []string {
	if cached, ok := p.dirCache[rel]; ok {
		return cached
	}
	entries, err := os.ReadDir(filepath.Join(p.root, filepath.FromSlash(rel)))
	var names []string
	if err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				names = append(names, e.Name())
			}
		}
	}
	p.dirCache[rel] = names
	return names
}
