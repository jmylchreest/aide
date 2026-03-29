// Package main provides the dashboard command for managing the aide-web binary.
package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/jmylchreest/aide/aide/internal/version"
)

func cmdDashboard(args []string) error {
	if hasFlag(args, "--help") || hasFlag(args, "-h") {
		printDashboardUsage()
		return nil
	}

	// Determine subcommand; default is "run".
	sub := "run"
	subArgs := args
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		sub = args[0]
		subArgs = args[1:]
	}

	switch sub {
	case "run":
		return dashboardRun(subArgs)
	case "download":
		return dashboardDownload(subArgs)
	case "upgrade":
		return dashboardUpgrade(subArgs)
	case "version":
		return dashboardVersion()
	default:
		return fmt.Errorf("unknown dashboard subcommand: %s", sub)
	}
}

// dashboardRun ensures aide-web is installed and executes it, passing through
// any remaining flags (--port, --addr, --open, etc.).
func dashboardRun(args []string) error {
	webPath, err := aideWebPath()
	if err != nil {
		return err
	}

	// Auto-download if not present.
	if _, err := os.Stat(webPath); os.IsNotExist(err) {
		fmt.Println("aide-web not found, downloading...")
		if err := downloadAideWeb("", webPath); err != nil {
			return fmt.Errorf("auto-download failed: %w", err)
		}
	}

	// Execute aide-web with the provided arguments.
	cmd := exec.Command(webPath, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		// Propagate child exit code without printing a redundant error.
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.ExitCode())
		}
		return err
	}
	return nil
}

// dashboardDownload downloads a specific (or current) version of aide-web.
func dashboardDownload(args []string) error {
	if hasFlag(args, "--help") || hasFlag(args, "-h") {
		fmt.Println(`aide dashboard download - Download the aide-web binary

Usage:
  aide dashboard download [VERSION]

Arguments:
  VERSION    Version to download (default: current aide version)`)
		return nil
	}

	// Parse optional version argument.
	targetVersion := ""
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			targetVersion = arg
			break
		}
	}

	webPath, err := aideWebPath()
	if err != nil {
		return err
	}

	return downloadAideWeb(targetVersion, webPath)
}

// dashboardUpgrade checks if a newer aide-web is available and downloads it.
func dashboardUpgrade(args []string) error {
	if hasFlag(args, "--help") || hasFlag(args, "-h") {
		fmt.Println(`aide dashboard upgrade - Upgrade aide-web to the latest version

Usage:
  aide dashboard upgrade [options]

Options:
  --force    Force download even if already up to date`)
		return nil
	}

	forceDownload := hasFlag(args, "--force")

	webPath, err := aideWebPath()
	if err != nil {
		return err
	}

	installedVersion := getInstalledAideWebVersion(webPath)
	if installedVersion != "" {
		fmt.Printf("Installed aide-web version: %s\n", installedVersion)
	} else {
		fmt.Println("aide-web is not installed.")
	}

	release, err := getLatestRelease()
	if err != nil {
		return fmt.Errorf("failed to check for updates: %w", err)
	}

	latestVersion := release.TagName
	fmt.Printf("Latest version:              %s\n", latestVersion)

	if !forceDownload && installedVersion != "" {
		installed := strings.TrimPrefix(installedVersion, "v")
		latest := strings.TrimPrefix(latestVersion, "v")
		if compareVersions(installed, latest) >= 0 {
			fmt.Println("\naide-web is already up to date!")
			return nil
		}
	}

	fmt.Println()
	return downloadAideWeb("", webPath)
}

// dashboardVersion prints the installed aide-web version.
func dashboardVersion() error {
	webPath, err := aideWebPath()
	if err != nil {
		return err
	}

	ver := getInstalledAideWebVersion(webPath)
	if ver == "" {
		fmt.Println("aide-web: not installed")
	} else {
		fmt.Printf("aide-web: %s\n", ver)
	}
	return nil
}

// aideWebPath returns the expected path for the aide-web binary.
// It looks next to the running aide binary first. If aide is running from a
// temp directory (e.g. via `go run`), it falls back to .aide/bin/ in the
// project root so the download persists across runs.
func aideWebPath() (string, error) {
	name := "aide-web"
	if runtime.GOOS == "windows" {
		name = "aide-web.exe"
	}

	self, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("failed to get executable path: %w", err)
	}
	self, err = filepath.EvalSymlinks(self)
	if err != nil {
		return "", fmt.Errorf("failed to resolve executable path: %w", err)
	}

	dir := filepath.Dir(self)

	// If the binary is in a temp directory (go run), use .aide/bin/ instead
	// so the download persists.
	if strings.HasPrefix(dir, os.TempDir()) {
		projectRoot, hasMarker := findProjectRoot()
		if hasMarker {
			binDir := filepath.Join(projectRoot, ".aide", "bin")
			_ = os.MkdirAll(binDir, 0o755)
			return filepath.Join(binDir, name), nil
		}
	}

	return filepath.Join(dir, name), nil
}

// getAideWebBinaryName returns the platform-specific asset name for aide-web
// releases (e.g. "aide-web-linux-amd64").
func getAideWebBinaryName() string {
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	return fmt.Sprintf("aide-web-%s-%s%s", runtime.GOOS, runtime.GOARCH, ext)
}

// downloadAideWeb downloads the aide-web binary for the given version (or
// the current aide version when targetVersion is empty) and installs it to
// destPath.
func downloadAideWeb(targetVersion, destPath string) error {
	var release *GitHubRelease
	var err error

	if targetVersion != "" {
		release, err = getRelease(targetVersion)
		if err != nil {
			return fmt.Errorf("failed to get release %s: %w", targetVersion, err)
		}
	} else if version.IsRelease() {
		// Use the matching release for this aide version.
		release, err = getRelease("v" + version.Version)
		if err != nil {
			fmt.Printf("Release v%s not found, falling back to latest...\n", version.Version)
			release, err = getLatestRelease()
			if err != nil {
				return fmt.Errorf("failed to get latest release: %w", err)
			}
		}
	} else {
		// Dev/snapshot build — use latest release.
		fmt.Println("Dev build detected, fetching latest release...")
		release, err = getLatestRelease()
		if err != nil {
			return fmt.Errorf("failed to get latest release: %w", err)
		}
	}

	releaseTag := release.TagName
	assetName := getAideWebBinaryName()

	// Find the asset in the release.
	var downloadURL string
	for _, asset := range release.Assets {
		if asset.Name == assetName {
			downloadURL = asset.BrowserDownloadURL
			break
		}
	}

	if downloadURL == "" {
		return fmt.Errorf("no aide-web binary available for %s/%s in release %s", runtime.GOOS, runtime.GOARCH, releaseTag)
	}

	fmt.Printf("Downloading %s from %s...\n", assetName, releaseTag)

	// Download to temp file.
	tmpFile, err := downloadFile(downloadURL)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer os.Remove(tmpFile)

	// Verify checksum.
	checksumURL := fmt.Sprintf("%s/%s/checksums.txt", githubBaseURL, releaseTag)
	if err := verifyDownloadChecksum(tmpFile, assetName, checksumURL); err != nil {
		return fmt.Errorf("checksum verification failed: %w", err)
	}
	fmt.Println("Checksum verified.")

	// Install: if an existing binary is present, remove it first.
	if _, err := os.Stat(destPath); err == nil {
		oldPath := destPath + ".old"
		os.Remove(oldPath)
		if err := os.Rename(destPath, oldPath); err != nil {
			return fmt.Errorf("failed to move old aide-web binary: %w", err)
		}
		defer os.Remove(oldPath)
	}

	if err := copyFile(tmpFile, destPath); err != nil {
		return fmt.Errorf("failed to install aide-web binary: %w", err)
	}

	if err := os.Chmod(destPath, 0755); err != nil {
		return fmt.Errorf("failed to set permissions: %w", err)
	}

	fmt.Printf("Installed aide-web %s to %s\n", releaseTag, destPath)
	return nil
}

// getInstalledAideWebVersion runs "aide-web version" and returns the version
// string, or "" if aide-web is not installed or the command fails.
func getInstalledAideWebVersion(webPath string) string {
	if _, err := os.Stat(webPath); os.IsNotExist(err) {
		return ""
	}

	cmd := exec.Command(webPath, "version")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}

	ver := strings.TrimSpace(string(out))
	// The output might be multi-line or prefixed; take the first line.
	if idx := strings.IndexByte(ver, '\n'); idx >= 0 {
		ver = ver[:idx]
	}
	return ver
}

func printDashboardUsage() {
	fmt.Println(`aide dashboard - Manage aide-web dashboard (run, download, upgrade)

Usage:
  aide dashboard [command] [options]

Commands:
  run        Start the aide-web dashboard (default if no command given)
  download   Download the aide-web binary
  upgrade    Upgrade aide-web to the latest version
  version    Show installed aide-web version

Run options (passed through to aide-web):
  --port PORT    Port to listen on (default: 8080)
  --addr ADDR    Address to bind to (default: 127.0.0.1)
  --open         Open browser automatically
  --dev          Enable development mode

Examples:
  aide dashboard                     # Start the dashboard
  aide dashboard run --port 3000     # Start on a specific port
  aide dashboard download            # Download aide-web
  aide dashboard download v0.0.50    # Download a specific version
  aide dashboard upgrade             # Upgrade to latest version
  aide dashboard upgrade --force     # Force re-download
  aide dashboard version             # Show installed version`)
}
