package grammar

import (
	"runtime"
	"strings"
	"testing"
)

func TestCurrentPlatform(t *testing.T) {
	p := CurrentPlatform()

	if p.OS != runtime.GOOS {
		t.Errorf("OS: got %q, want %q", p.OS, runtime.GOOS)
	}
	if p.Arch != runtime.GOARCH {
		t.Errorf("Arch: got %q, want %q", p.Arch, runtime.GOARCH)
	}

	// Verify the extension matches the OS.
	switch runtime.GOOS {
	case "darwin":
		if p.Ext != ".dylib" {
			t.Errorf("Ext on darwin: got %q, want %q", p.Ext, ".dylib")
		}
	case "windows":
		if p.Ext != ".dll" {
			t.Errorf("Ext on windows: got %q, want %q", p.Ext, ".dll")
		}
	default:
		if p.Ext != ".so" {
			t.Errorf("Ext on %s: got %q, want %q", runtime.GOOS, p.Ext, ".so")
		}
	}
}

func TestLibraryFilename(t *testing.T) {
	tests := []struct {
		name    string
		version string
		wantPfx string // prefix before the platform-specific parts
	}{
		{name: "ruby", version: "v0.23.1", wantPfx: "aide-grammar-ruby-v0.23.1-"},
		{name: "kotlin", version: "latest", wantPfx: "aide-grammar-kotlin-latest-"},
		{name: "csharp", version: "grammars-v1", wantPfx: "aide-grammar-csharp-grammars-v1-"},
	}

	p := CurrentPlatform()
	for _, tt := range tests {
		t.Run(tt.name+"/"+tt.version, func(t *testing.T) {
			got := LibraryFilename(tt.name, tt.version)
			if !strings.HasPrefix(got, tt.wantPfx) {
				t.Errorf("LibraryFilename(%q, %q) = %q; want prefix %q", tt.name, tt.version, got, tt.wantPfx)
			}
			wantSuffix := p.OS + "-" + p.Arch + p.Ext
			if !strings.HasSuffix(got, wantSuffix) {
				t.Errorf("LibraryFilename(%q, %q) = %q; want suffix %q", tt.name, tt.version, got, wantSuffix)
			}
		})
	}
}

func TestLibraryAssetName(t *testing.T) {
	// LibraryAssetName is an alias for LibraryFilename — verify they match.
	got := LibraryAssetName("ruby", "v1.0")
	want := LibraryFilename("ruby", "v1.0")
	if got != want {
		t.Errorf("LibraryAssetName = %q; LibraryFilename = %q; should match", got, want)
	}
}
