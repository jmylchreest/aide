package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
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

func TestCheckoutCleanupReusedPath(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(root, ".aide", "memory", "memory.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0700); err != nil {
		t.Fatal(err)
	}
	st, err := NewBoltStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	wt := filepath.Join(root, "worktree")
	create := func() checkout.Info {
		t.Helper()
		if _, err := git.PlainInit(wt, false); err != nil {
			t.Fatal(err)
		}
		c, err := checkout.Resolve(wt, wt)
		if err != nil {
			t.Fatal(err)
		}
		if err := RegisterCheckout(dbPath, c); err != nil {
			t.Fatal(err)
		}
		return c
	}
	old := create()
	if err := os.RemoveAll(wt); err != nil {
		t.Fatal(err)
	}
	replacement := create()
	if old.ID == replacement.ID {
		t.Fatal("reused identity")
	}
	now := time.Now()
	if n, err := PruneCheckouts(dbPath, st, now, nil, nil); err != nil || n != 0 {
		t.Fatalf("grace period: %d %v", n, err)
	}
	closed := ""
	n, err := PruneCheckouts(dbPath, st, now.Add(8*24*time.Hour), nil, func(c checkout.Info) error { closed = c.ID; return nil })
	if err != nil || n != 1 || closed != old.ID {
		t.Fatalf("old cache not pruned: %d %s %v", n, closed, err)
	}
	if _, err := os.Stat(CheckoutDir(dbPath, old)); !os.IsNotExist(err) {
		t.Fatal("old cache retained")
	}
	for _, path := range []string{CheckoutDir(dbPath, replacement), wt, dbPath} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("live data removed: %s: %v", path, err)
		}
	}
}
