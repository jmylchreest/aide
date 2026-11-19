package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInstallUpgradeBinary(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "aide")
	src := filepath.Join(t.TempDir(), "download")
	if err := os.WriteFile(dst, []byte("old executable"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := installUpgradeBinary(src, dst); err == nil {
		t.Fatal("missing download should fail")
	}
	if data, _ := os.ReadFile(dst); string(data) != "old executable" {
		t.Fatal("failed upgrade damaged the installed binary")
	}
	if err := os.WriteFile(src, []byte("new executable"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := installUpgradeBinary(src, dst); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(dst); string(data) != "new executable" {
		t.Fatal("replacement was not installed")
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("staging files left behind: %v, %v", entries, err)
	}
}

func TestInstallUpgradeBinaryPreservesDestinationOnRenameFailure(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "occupied")
	if err := os.Mkdir(dst, 0755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(t.TempDir(), "download")
	if err := os.WriteFile(src, []byte("new executable"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := installUpgradeBinary(src, dst); err == nil {
		t.Fatal("replacing a directory should fail")
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 || !entries[0].IsDir() {
		t.Fatalf("destination changed or staging files leaked: %v, %v", entries, err)
	}
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"0.0.1", "0.0.2", -1},
		{"0.0.2", "0.0.1", 1},
		{"0.0.1", "0.0.1", 0},
		{"v0.0.29", "v0.0.29", 0},
		{"0.0.0", "v0.0.29", -1},
		{"v0.1.0", "v0.0.29", 1},
		{"1.0.0", "0.99.99", 1},
		// Prerelease
		{"1.0.0-dev.1", "1.0.0", -1},
		{"1.0.0", "1.0.0-dev.1", 1},
		{"1.0.0-dev.1", "1.0.0-dev.2", -1},
		{"1.0.0-dev.10", "1.0.0-dev.2", 1},
		{"1.0.0+build.1", "1.0.0+build.2", 0},
		{"999999999999999999999.0.0", "1.0.0", 1},
	}

	for _, tt := range tests {
		got, err := compareVersions(tt.a, tt.b)
		if err != nil {
			t.Fatal(err)
		}
		if got != tt.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestGetBinaryName(t *testing.T) {
	name := getBinaryName()
	if name == "" {
		t.Error("getBinaryName returned empty string")
	}
	// Should contain the OS and arch
	if len(name) < 10 {
		t.Errorf("getBinaryName too short: %q", name)
	}
}

func TestCompareVersionsRejectsMalformedComponents(t *testing.T) {
	for _, version := range []string{"1.two.3", "1.2.-3", "", "1.02.3", "1.2.3-01", "dev"} {
		if _, err := compareVersions(version, "1.2.3"); err == nil {
			t.Errorf("accepted version %q", version)
		}
		if _, err := compareVersions("1.2.3", version); err == nil {
			t.Errorf("accepted target %q", version)
		}
	}
}
