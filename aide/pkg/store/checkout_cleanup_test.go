package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/checkout"
)

func TestCheckoutCleanupGraceAndRegistration(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(root, ".aide", "memory", "memory.db")
	os.MkdirAll(filepath.Dir(dbPath), 0700)
	st, err := NewBoltStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	c := checkout.Info{ID: "0123456789abcdef0123456789abcdef", Root: filepath.Join(root, "gone"), GitDir: filepath.Join(root, ".git", "worktrees", "gone")}
	if err := RegisterCheckout(dbPath, c); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	closed := 0
	prune := func(at time.Time) int {
		t.Helper()
		n, e := PruneCheckouts(dbPath, st, at, nil, func(checkout.Info) error { closed++; return nil })
		if e != nil {
			t.Fatal(e)
		}
		return n
	}
	if n := prune(now); n != 0 {
		t.Fatal("removed before grace")
	}
	if err := os.MkdirAll(c.GitDir, 0700); err != nil {
		t.Fatal(err)
	}
	if n := prune(now.Add(8 * 24 * time.Hour)); n != 0 {
		t.Fatal("removed registered worktree")
	}
	if err := os.RemoveAll(c.GitDir); err != nil {
		t.Fatal(err)
	}
	if n := prune(now.Add(9 * 24 * time.Hour)); n != 0 {
		t.Fatal("registration recovery did not reset grace")
	}
	if n := prune(now.Add(17 * 24 * time.Hour)); n != 1 || closed != 1 {
		t.Fatalf("cleanup: %d closed %d", n, closed)
	}
	if _, e := os.Stat(CheckoutDir(dbPath, c)); !os.IsNotExist(e) {
		t.Fatal("cache still exists")
	}
	if _, e := os.Stat(dbPath); e != nil {
		t.Fatal("shared store removed")
	}
}
