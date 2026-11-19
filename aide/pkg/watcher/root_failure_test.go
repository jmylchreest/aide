package watcher

import (
	"os"
	"testing"
)

func TestNewRejectsUnknownWorkingDirectory(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.Remove(dir); err != nil {
		t.Skipf("cannot remove cwd on this platform: %v", err)
	}
	w, err := New(Config{})
	if err == nil {
		w.Stop()
		t.Fatal("watcher accepted an unresolved working directory")
	}
}
