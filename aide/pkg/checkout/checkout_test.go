package checkout

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	git "github.com/go-git/go-git/v5"
)

func fixture(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	if _, err := git.PlainInit(root, false); err != nil {
		t.Fatal(err)
	}
	wt := t.TempDir()
	admin := filepath.Join(root, ".git", "worktrees", "test")
	if err := os.MkdirAll(admin, 0700); err != nil {
		t.Fatal(err)
	}
	for p, data := range map[string]string{
		filepath.Join(wt, ".git"):         "gitdir: " + admin + "\n",
		filepath.Join(admin, "commondir"): "../..\n",
		filepath.Join(admin, "gitdir"):    filepath.Join(wt, ".git") + "\n",
		filepath.Join(admin, "HEAD"):      "ref: refs/heads/feature\n",
	} {
		if err := os.WriteFile(p, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return root, wt
}

func TestIdentityLifetime(t *testing.T) {
	root, wt := fixture(t)
	main, err := Resolve(root, root)
	if err != nil {
		t.Fatal(err)
	}
	first, err := Resolve(root, wt)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == main.ID || first.Root != wt {
		t.Fatalf("not isolated: %+v %+v", main, first)
	}
	if err := os.WriteFile(filepath.Join(first.GitDir, "HEAD"), []byte("ref: refs/heads/other\n"), 0600); err != nil {
		t.Fatal(err)
	}
	again, err := Resolve(root, wt)
	if err != nil || again.ID != first.ID {
		t.Fatalf("branch switch changed identity: %+v %v", again, err)
	}
	moved := wt + "-moved"
	if err := os.Rename(wt, moved); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(moved) })
	if err := os.WriteFile(filepath.Join(first.GitDir, "gitdir"), []byte(filepath.Join(moved, ".git")), 0600); err != nil {
		t.Fatal(err)
	}
	again, err = Resolve(root, moved)
	if err != nil || again.ID != first.ID {
		t.Fatalf("move changed identity: %+v %v", again, err)
	}
	if err := os.RemoveAll(first.GitDir); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(first.GitDir, 0700); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string]string{"HEAD": "ref: refs/heads/other\n", "commondir": "../..\n", "gitdir": filepath.Join(moved, ".git")} {
		if err := os.WriteFile(filepath.Join(first.GitDir, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	again, err = Resolve(root, moved)
	if err != nil || again.ID == first.ID {
		t.Fatalf("recreated checkout reused identity: %+v %v", again, err)
	}
}

func TestConcurrentIdentityAndForeignCheckout(t *testing.T) {
	root, wt := fixture(t)
	var wg sync.WaitGroup
	ids := make(chan string, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, err := Resolve(root, wt)
			if err != nil {
				t.Error(err)
				return
			}
			ids <- c.ID
		}()
	}
	wg.Wait()
	close(ids)
	var want string
	for id := range ids {
		if want == "" {
			want = id
		}
		if id != want {
			t.Fatalf("multiple identities: %s %s", want, id)
		}
	}
	other, _ := fixture(t)
	if _, err := Resolve(root, other); err == nil {
		t.Fatal("foreign repository accepted")
	}
}
