package grammar

import (
	"path/filepath"
	"runtime"
)

// PlatformInfo holds the current platform details for grammar file naming.
type PlatformInfo struct {
	OS   string // "linux", "darwin", "windows"
	Arch string // "amd64", "arm64"
	Ext  string // ".so", ".dylib", ".dll"
}

// CurrentPlatform returns the platform info for the running system.
func CurrentPlatform() PlatformInfo {
	p := PlatformInfo{
		OS:   runtime.GOOS,
		Arch: runtime.GOARCH,
	}

	switch p.OS {
	case "darwin":
		p.Ext = ".dylib"
	case "windows":
		p.Ext = ".dll"
	default: // linux, freebsd, etc.
		p.Ext = ".so"
	}

	return p
}

// LibraryFilename returns the expected filename for a grammar shared library
// within the grammar cache directory. Since Phase 3, grammars are stored in
// per-language subdirectories: {name}/grammar{ext}
func LibraryFilename(name string) string {
	p := CurrentPlatform()
	return filepath.Join(name, "grammar"+p.Ext)
}

// PackArchiveFilename returns the GitHub release asset name for a grammar pack
// archive. Format: aide-grammar-{name}-{version}-{os}-{arch}.tar.gz
func PackArchiveFilename(name, version string) string {
	p := CurrentPlatform()
	return "aide-grammar-" + name + "-" + version + "-" + p.OS + "-" + p.Arch + ".tar.gz"
}
