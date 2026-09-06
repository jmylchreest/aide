package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadDashboardDevPath(t *testing.T) {
	dir := t.TempDir()
	pointer := filepath.Join(dir, "dashboard-dev.path")
	if path, err := readDashboardDevPath(pointer); path != "" || err != nil {
		t.Fatalf("absent pointer: %q, %v", path, err)
	}
	binary := filepath.Join(dir, "local build", "aide-web")
	if err := os.MkdirAll(filepath.Dir(binary), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(binary, []byte("build"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pointer, []byte(binary+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if path, err := readDashboardDevPath(pointer); path != binary || err != nil {
		t.Fatalf("local build: %q, %v", path, err)
	}
	for _, invalid := range []string{"", "relative/aide-web", dir, filepath.Join(dir, "missing")} {
		if err := os.WriteFile(pointer, []byte(invalid), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := readDashboardDevPath(pointer); err == nil {
			t.Errorf("accepted invalid dev path %q", invalid)
		}
	}
}

func TestDashboardDownloadProtectsSelectedBuild(t *testing.T) {
	dir := t.TempDir()
	pointer := filepath.Join(dir, "dashboard-dev.path")
	local := filepath.Join(dir, "aide-web")
	if err := os.WriteFile(local, []byte("local build"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pointer, []byte(local+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validateDashboardDownloadPath(local, pointer); err == nil {
		t.Fatal("download would overwrite the selected local build")
	}
	if err := validateDashboardDownloadPath(filepath.Join(dir, "published", "aide-web"), pointer); err != nil {
		t.Fatalf("separate published install: %v", err)
	}
	alias := filepath.Join(dir, "alias")
	if err := os.Symlink(local, alias); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := validateDashboardDownloadPath(alias, pointer); err == nil {
		t.Fatal("download through an alias would overwrite the selected local build")
	}
}
