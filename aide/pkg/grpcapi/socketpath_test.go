package grpcapi

import (
	"os"
	"path/filepath"
	"testing"
)

// deepProject builds a project directory deep enough that its in-project
// socket path would exceed maxSocketPathLen, forcing the hashed fallback.
func deepProject(t *testing.T, base string) string {
	t.Helper()
	dir := base
	for len(filepath.Join(dir, ".aide", "aide.sock")) <= maxSocketPathLen {
		dir = filepath.Join(dir, "nested")
	}
	if err := os.MkdirAll(filepath.Join(dir, ".aide", "memory"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func dbIn(root string) string {
	return filepath.Join(root, ".aide", "memory", "store.db")
}

// aliasOf returns real reached through a symlink, skipping when the platform
// will not let us create one (unprivileged Windows).
func aliasOf(t *testing.T, real, inner string) string {
	t.Helper()
	link := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	return filepath.Join(link, inner)
}

// TestSocketPathFromDB_AliasAgrees is the regression this guards: one project
// reached by two spellings must resolve to one socket. When it did not, a
// daemon started under one spelling and a client invoked under the other
// hashed to different paths and never met — reported to the user as "no
// daemon" while the daemon was running.
func TestSocketPathFromDB_AliasAgrees(t *testing.T) {
	t.Run("deep project (hashed fallback)", func(t *testing.T) {
		real := t.TempDir()
		deep := deepProject(t, real)
		alias := aliasOf(t, real, deep[len(real)+1:])

		viaReal := SocketPathFromDB(dbIn(deep))
		viaAlias := SocketPathFromDB(dbIn(alias))
		if viaReal != viaAlias {
			t.Errorf("aliased spellings produced different sockets:\n  real  %s\n  alias %s", viaReal, viaAlias)
		}
		if len(viaReal) > maxSocketPathLen {
			t.Errorf("fallback path is still too long: %d bytes (%s)", len(viaReal), viaReal)
		}
	})

	// A shallow project stays in-project. The two spellings must still agree,
	// including on WHICH branch they take: an alias short enough to fit while
	// the resolved path is not would otherwise put one caller in-project and
	// the other on the hashed path.
	t.Run("shallow project (in-project socket)", func(t *testing.T) {
		real := t.TempDir()
		if err := os.MkdirAll(filepath.Join(real, ".aide", "memory"), 0o755); err != nil {
			t.Fatal(err)
		}
		alias := aliasOf(t, real, "")

		viaReal := SocketPathFromDB(dbIn(real))
		viaAlias := SocketPathFromDB(dbIn(alias))
		if viaReal != viaAlias {
			t.Errorf("aliased spellings produced different sockets:\n  real  %s\n  alias %s", viaReal, viaAlias)
		}
	})
}

// TestSocketPathFromDB_StaysUnderLimit: the whole point of the fallback.
func TestSocketPathFromDB_StaysUnderLimit(t *testing.T) {
	deep := deepProject(t, t.TempDir())
	got := SocketPathFromDB(dbIn(deep))
	if len(got) > maxSocketPathLen {
		t.Errorf("socket path %d bytes exceeds maxSocketPathLen %d: %s", len(got), maxSocketPathLen, got)
	}
}

// TestSocketPathFromDB_Deterministic: server and client derive independently,
// so repeated calls must agree.
func TestSocketPathFromDB_Deterministic(t *testing.T) {
	deep := deepProject(t, t.TempDir())
	first := SocketPathFromDB(dbIn(deep))
	for i := 0; i < 3; i++ {
		if got := SocketPathFromDB(dbIn(deep)); got != first {
			t.Fatalf("call %d returned %s, want %s", i+2, got, first)
		}
	}
}

// TestSocketPathFromDB_DistinctProjects: different projects must not collide.
func TestSocketPathFromDB_DistinctProjects(t *testing.T) {
	a := deepProject(t, t.TempDir())
	b := deepProject(t, t.TempDir())
	if SocketPathFromDB(dbIn(a)) == SocketPathFromDB(dbIn(b)) {
		t.Error("two distinct projects share one socket path")
	}
}
