package grammar

import (
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

// LibraryFilename returns the expected filename for a grammar shared library.
// Format: aide-grammar-{lang}-{version}-{os}-{arch}.{ext}
func LibraryFilename(name, version string) string {
	p := CurrentPlatform()
	return "aide-grammar-" + name + "-" + version + "-" + p.OS + "-" + p.Arch + p.Ext
}

// LibraryAssetName returns the GitHub release asset name for a grammar.
func LibraryAssetName(name, version string) string {
	return LibraryFilename(name, version)
}
