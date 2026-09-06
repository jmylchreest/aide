package handler

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDisplayPath(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "project")
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	display := newDisplayPath(root)
	for _, tc := range []struct{ path, want string }{
		{filepath.Join(root, "src", "file.go"), "src/file.go"},
		{filepath.Join(dir, "project-other", "file.go"), ""},
		{"session-start", ""},
		{"src/file.go", ""},
		{root, "."},
	} {
		if got := display(tc.path); got != tc.want {
			t.Errorf("display(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
	alias := filepath.Join(dir, "alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if got := display(filepath.Join(alias, "src", "deleted.go")); got != "src/deleted.go" {
		t.Errorf("alias display = %q", got)
	}
	if got := newDisplayPath(alias)(filepath.Join(root, "src", "file.go")); got != "src/file.go" {
		t.Errorf("canonical display for aliased root = %q", got)
	}
}
