package grpcapi

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestSocketPathFromDBShort(t *testing.T) {
	p := SocketPathFromDB("/home/u/proj/.aide/memory/memory.db")
	if p != "/home/u/proj/.aide/aide.sock" {
		t.Fatalf("short path = %q", p)
	}
}

func TestSocketPathFromDBLongFallsBack(t *testing.T) {
	deep := "/" + strings.Repeat("deeply-nested-dir/", 8) + "proj"
	dbPath := filepath.Join(deep, ".aide", "memory", "memory.db")

	xdg := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", xdg)

	p := SocketPathFromDB(dbPath)
	if len(p) > maxSocketPathLen {
		t.Fatalf("fallback still too long (%d): %q", len(p), p)
	}
	if !strings.HasPrefix(p, filepath.Join(xdg, "aide")) {
		t.Fatalf("fallback not under runtime dir: %q", p)
	}
	if p != SocketPathFromDB(dbPath) {
		t.Fatal("fallback is not deterministic")
	}
	// A different project must get a different socket.
	other := SocketPathFromDB(filepath.Join(deep+"2", ".aide", "memory", "memory.db"))
	if other == p {
		t.Fatal("distinct projects share a fallback socket")
	}
}

func TestSocketPathFromDBLongWithoutXDG(t *testing.T) {
	deep := "/" + strings.Repeat("deeply-nested-dir/", 8) + "proj"
	dbPath := filepath.Join(deep, ".aide", "memory", "memory.db")
	t.Setenv("XDG_RUNTIME_DIR", "")

	p := SocketPathFromDB(dbPath)
	if len(p) > maxSocketPathLen {
		t.Fatalf("tempdir fallback too long (%d): %q", len(p), p)
	}
}
