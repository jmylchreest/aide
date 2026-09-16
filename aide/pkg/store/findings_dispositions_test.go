package store

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jmylchreest/aide/aide/pkg/checkout"
	"github.com/jmylchreest/aide/aide/pkg/findings"
)

func TestFindingDispositionSurvivesCacheRemoval(t *testing.T) {
	root := t.TempDir()
	st, err := NewBoltStore(filepath.Join(root, "memory.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	c := checkout.Info{ID: "checkout-one", Root: root}
	dir := filepath.Join(root, "cache")
	fs, err := NewCheckoutFindingsStore(dir, st, c)
	if err != nil {
		t.Fatal(err)
	}
	f := &findings.Finding{ID: "finding-1", Analyzer: "complexity", FilePath: "same.go", Title: "Too complex"}
	if err := fs.AddFinding(f); err != nil {
		t.Fatal(err)
	}
	if _, err := fs.AcceptFindings([]string{f.ID}); err != nil {
		t.Fatal(err)
	}
	fs.Close()
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	fs, err = NewCheckoutFindingsStore(dir, st, c)
	if err != nil {
		t.Fatal(err)
	}
	defer fs.Close()
	f.Accepted = false
	f.ID = "new-analysis-id"
	if err := fs.AddFinding(f); err != nil {
		t.Fatal(err)
	}
	actual, err := fs.GetFinding(f.ID)
	if err != nil || !actual.Accepted {
		t.Fatalf("disposition lost: %+v %v", actual, err)
	}
	other, err := NewCheckoutFindingsStore(filepath.Join(root, "other"), st, checkout.Info{ID: "checkout-two"})
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	f.Accepted = false
	if err := other.AddFinding(f); err != nil {
		t.Fatal(err)
	}
	actual, _ = other.GetFinding(f.ID)
	if actual.Accepted {
		t.Fatal("acceptance leaked to another checkout")
	}
}
