package grammar

import (
	"path/filepath"
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
	p := CurrentPlatform()

	tests := []struct {
		name string
		want string
	}{
		{name: "ruby", want: filepath.Join("ruby", "grammar"+p.Ext)},
		{name: "kotlin", want: filepath.Join("kotlin", "grammar"+p.Ext)},
		{name: "csharp", want: filepath.Join("csharp", "grammar"+p.Ext)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := LibraryFilename(tt.name)
			if got != tt.want {
				t.Errorf("LibraryFilename(%q) = %q; want %q", tt.name, got, tt.want)
			}
		})
	}
}

func TestPackArchiveFilename(t *testing.T) {
	tests := []struct {
		name    string
		version string
		wantPfx string
	}{
		{name: "ruby", version: "v0.1.0", wantPfx: "aide-grammar-ruby-v0.1.0-"},
		{name: "kotlin", version: "snapshot", wantPfx: "aide-grammar-kotlin-snapshot-"},
	}

	p := CurrentPlatform()
	for _, tt := range tests {
		t.Run(tt.name+"/"+tt.version, func(t *testing.T) {
			got := PackArchiveFilename(tt.name, tt.version)
			if !strings.HasPrefix(got, tt.wantPfx) {
				t.Errorf("PackArchiveFilename(%q, %q) = %q; want prefix %q", tt.name, tt.version, got, tt.wantPfx)
			}
			wantSuffix := p.OS + "-" + p.Arch + ".tar.gz"
			if !strings.HasSuffix(got, wantSuffix) {
				t.Errorf("PackArchiveFilename(%q, %q) = %q; want suffix %q", tt.name, tt.version, got, wantSuffix)
			}
		})
	}
}
