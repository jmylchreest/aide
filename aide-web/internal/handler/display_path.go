package handler

import (
	"path/filepath"
	"strings"
)

// Display paths are separate from recorded paths so expanded events and tooltips
// retain their original value. Cache parent resolution within a list or stream.
func newDisplayPath(root string) func(string) string {
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		canonicalRoot = root
	}
	parents := map[string]string{}
	inside := func(base, path string) string {
		rel, err := filepath.Rel(base, path)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			return ""
		}
		return filepath.ToSlash(rel)
	}
	return func(path string) string {
		if !filepath.IsAbs(path) {
			return ""
		}
		if rel := inside(root, path); rel != "" {
			return rel
		}
		parent := filepath.Dir(path)
		resolved, ok := parents[parent]
		if !ok {
			resolved, _ = filepath.EvalSymlinks(parent)
			if len(parents) >= 128 {
				clear(parents)
			}
			parents[parent] = resolved
		}
		if resolved == "" {
			return ""
		}
		return inside(canonicalRoot, filepath.Join(resolved, filepath.Base(path)))
	}
}
