package clone

import (
	"os"
	"testing"
)

func TestClonesRejectUnknownWorkingDirectory(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.Remove(dir); err != nil {
		t.Skipf("cannot remove cwd on this platform: %v", err)
	}
	if _, _, err := DetectClones(Config{}); err == nil {
		t.Fatal("clone scan accepted an unresolved root")
	}
}
