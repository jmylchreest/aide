package anchor

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestRealPath_ResolvesAlias covers the shape that keeps confusing readers: one
// tree reachable by two spellings, because a directory on the way is a symlink.
func TestRealPath_ResolvesAlias(t *testing.T) {
	real := t.TempDir()
	project := filepath.Join(real, "project")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(real, alias); err != nil {
		t.Skipf("symlinks unavailable: %v", err) // unprivileged Windows
	}

	viaAlias := RealPath(filepath.Join(alias, "project"))
	viaReal := RealPath(project)
	if viaAlias != viaReal {
		t.Errorf("aliased spellings did not converge:\n  alias -> %q\n  real  -> %q", viaAlias, viaReal)
	}
}

// TestRealPath_FallsBackToClean: a path that cannot be resolved (it does not
// exist yet) must still come back normalised, not as the raw input.
func TestRealPath_FallsBackToClean(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope", "..", "nope", "child")
	got := RealPath(missing)
	if got != filepath.Clean(missing) {
		t.Errorf("RealPath(%q) = %q, want the cleaned form %q", missing, got, filepath.Clean(missing))
	}
	if strings.Contains(got, ".."+string(filepath.Separator)) {
		t.Errorf("RealPath left an unnormalised .. segment: %q", got)
	}
}

func TestContains(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "pkg", "thing")
	if err := os.MkdirAll(inside, 0o755); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		root string
		path string
		want bool
	}{
		{"self is contained", root, root, true},
		{"descendant", root, inside, true},
		{"parent is not inside child", inside, root, false},
		// The bug strings.HasPrefix had: a sibling whose name extends the root.
		{"sibling sharing a name prefix", filepath.Join(root, "aide"), filepath.Join(root, "aide-web"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Contains(tt.root, tt.path); got != tt.want {
				t.Errorf("Contains(%q, %q) = %v, want %v", tt.root, tt.path, got, tt.want)
			}
		})
	}
}

// TestContains_ThroughAlias is the regression this whole change exists for:
// cwd reached by the alias spelling, root recorded by the real one.
func TestContains_ThroughAlias(t *testing.T) {
	real := t.TempDir()
	project := filepath.Join(real, "project", "pkg")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(real, alias); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	root := filepath.Join(real, "project")        // recorded spelling
	cwd := filepath.Join(alias, "project", "pkg") // caller's spelling
	if !Contains(root, cwd) {
		t.Errorf("cwd reached through the alias read as outside its own root:\n  root %q\n  cwd  %q", root, cwd)
	}
}

// TestContains_CaseFolding: Windows filesystems are case-insensitive, so the
// comparison must be too — and must NOT be on platforms where case matters.
func TestContains_CaseFolding(t *testing.T) {
	root := t.TempDir()
	upper := strings.ToUpper(root)
	got := Contains(root, upper)
	if runtime.GOOS == "windows" && !got {
		t.Errorf("Contains should fold case on Windows: %q vs %q", root, upper)
	}
	if runtime.GOOS != "windows" && root != upper && got {
		t.Errorf("Contains must stay case-sensitive on %s: %q vs %q", runtime.GOOS, root, upper)
	}
}
