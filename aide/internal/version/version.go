// Package version provides build-time version information for aide.
//
// Build-time variables are injected via ldflags:
//
//	go build -ldflags "
//	  -X github.com/jmylchreest/aide/aide/internal/version.Version=x.y.z
//	  -X github.com/jmylchreest/aide/aide/internal/version.Commit=$(git rev-parse HEAD)
//	  -X github.com/jmylchreest/aide/aide/internal/version.Date=$(date -u +%Y-%m-%dT%H:%M:%SZ)
//	"
package version

import (
	"encoding/json"
	"fmt"
	"runtime"
	"runtime/debug"
	"strings"
)

// Build-time variables injected via ldflags.
var (
	// Version is the semantic version following SemVer 2.0.0.
	// Release format: "1.2.3"
	// Dev format: "1.2.3-dev.N+HASH" (next patch + dev + commits since release + short SHA)
	// Default: "0.0.0" for local/development builds
	Version = "0.0.0"

	// Commit is the full git commit SHA.
	Commit = "unknown"

	// Date is the build timestamp in RFC3339 format.
	Date = "unknown"
)

func init() {
	// If ldflags weren't provided, try to get VCS info from build info
	if Commit == "unknown" {
		if info, ok := debug.ReadBuildInfo(); ok {
			for _, setting := range info.Settings {
				switch setting.Key {
				case "vcs.revision":
					Commit = setting.Value
				case "vcs.time":
					Date = setting.Value
				}
			}
		}
	}
}

// ApplicationName is the canonical name of this application.
const ApplicationName = "aide"

// Info contains structured version information.
type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	CommitSHA string `json:"commit_sha,omitempty"` // Short SHA for display
	Date      string `json:"date"`
	GoVersion string `json:"go_version"`
	Platform  string `json:"platform"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
}

// GetInfo returns all version information as a structured type.
func GetInfo() Info {
	commitSHA := ""
	if Commit != "unknown" && len(Commit) >= 8 {
		commitSHA = Commit[:8]
	}

	return Info{
		Version:   Version,
		Commit:    Commit,
		CommitSHA: commitSHA,
		Date:      Date,
		GoVersion: runtime.Version(),
		Platform:  fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
	}
}

// String returns a human-readable version string.
func String() string {
	info := GetInfo()
	if Commit != "unknown" && len(Commit) >= 8 {
		return fmt.Sprintf("%s version %s (commit: %s, built: %s, %s, %s)",
			ApplicationName, info.Version, info.CommitSHA, info.Date, info.GoVersion, info.Platform)
	}
	return fmt.Sprintf("%s version %s (%s, %s)", ApplicationName, info.Version, info.GoVersion, info.Platform)
}

// Short returns a short version string suitable for CLI --version output.
func Short() string {
	if Commit != "unknown" && len(Commit) >= 8 {
		return fmt.Sprintf("%s (%s)", Version, Commit[:8])
	}
	return Version
}

// JSON returns the version info as a JSON string for machine parsing.
func JSON() string {
	info := GetInfo()
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return fmt.Sprintf(`{"error": %q}`, err.Error())
	}
	return string(data)
}

// IsSnapshot returns true if this is a snapshot/prerelease build.
func IsSnapshot() bool {
	return Version == "0.0.0" || Version == "dev" || strings.Contains(Version, "-dev.")
}

// IsRelease returns true if this is a tagged release build.
func IsRelease() bool {
	return !IsSnapshot() && Version != "0.0.0" && Version != "dev"
}
