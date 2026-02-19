package main

import "testing"

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
	}

	for _, tt := range tests {
		got := compareVersions(tt.a, tt.b)
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
