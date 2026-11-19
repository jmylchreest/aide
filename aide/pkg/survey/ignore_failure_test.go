package survey

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSurveyRejectsUnreadableIgnoreFile(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".aideignore"), 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := RunTopology(root); err == nil {
		t.Error("topology silently ignored invalid ignore file")
	}
	if _, err := RunEntrypoints(root, nil); err == nil {
		t.Error("entrypoints silently ignored invalid ignore file")
	}
}
