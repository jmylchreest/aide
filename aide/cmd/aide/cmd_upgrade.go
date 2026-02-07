// Package main provides the upgrade command for aide.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/jmylchreest/aide/aide/internal/version"
)

// GitHubRelease represents a GitHub release API response.
type GitHubRelease struct {
	TagName string        `json:"tag_name"`
	Name    string        `json:"name"`
	Assets  []GitHubAsset `json:"assets"`
}

// GitHubAsset represents a release asset.
type GitHubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

const (
	githubRepo    = "jmylchreest/aide"
	githubAPIURL  = "https://api.github.com/repos/" + githubRepo + "/releases"
	githubBaseURL = "https://github.com/" + githubRepo + "/releases/download"
)

func cmdUpgrade(args []string) error {
	if hasFlag(args, "--help") || hasFlag(args, "-h") {
		printUpgradeUsage()
		return nil
	}

	checkOnly := hasFlag(args, "--check")
	forceDownload := hasFlag(args, "--force")

	// Parse target version (optional)
	targetVersion := ""
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			targetVersion = arg
			break
		}
	}

	// Get current version
	currentVersion := version.Short()
	fmt.Printf("Current version: %s\n", currentVersion)

	// Get release info
	var release *GitHubRelease
	var err error

	if targetVersion != "" {
		// Specific version requested
		release, err = getRelease(targetVersion)
		if err != nil {
			return fmt.Errorf("failed to get release %s: %w", targetVersion, err)
		}
	} else {
		// Get latest release
		release, err = getLatestRelease()
		if err != nil {
			return fmt.Errorf("failed to check for updates: %w", err)
		}
	}

	latestVersion := release.TagName
	fmt.Printf("Latest version:  %s\n", latestVersion)

	// Compare versions
	needsUpdate := forceDownload || compareVersions(currentVersion, latestVersion) < 0

	if !needsUpdate {
		fmt.Println("\nYou're already on the latest version!")
		return nil
	}

	if checkOnly {
		fmt.Printf("\nUpdate available: %s -> %s\n", currentVersion, latestVersion)
		fmt.Println("Run 'aide upgrade' to install the update.")
		return nil
	}

	// Find the right asset for this platform
	assetName := getBinaryName()
	var downloadURL string
	for _, asset := range release.Assets {
		if asset.Name == assetName {
			downloadURL = asset.BrowserDownloadURL
			break
		}
	}

	if downloadURL == "" {
		return fmt.Errorf("no binary available for %s/%s in release %s", runtime.GOOS, runtime.GOARCH, latestVersion)
	}

	fmt.Printf("\nDownloading %s...\n", assetName)

	// Download to temp file
	tmpFile, err := downloadFile(downloadURL)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer os.Remove(tmpFile)

	// Get current executable path
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return fmt.Errorf("failed to resolve executable path: %w", err)
	}

	// Replace current binary
	fmt.Printf("Installing to %s...\n", execPath)

	// On Windows, we need to rename first
	if runtime.GOOS == "windows" {
		oldPath := execPath + ".old"
		os.Remove(oldPath)
		if err := os.Rename(execPath, oldPath); err != nil {
			return fmt.Errorf("failed to backup old binary: %w", err)
		}
		defer os.Remove(oldPath)
	}

	// Copy new binary
	if err := copyFile(tmpFile, execPath); err != nil {
		return fmt.Errorf("failed to install new binary: %w", err)
	}

	// Make executable
	if err := os.Chmod(execPath, 0755); err != nil {
		return fmt.Errorf("failed to set permissions: %w", err)
	}

	// Write version file
	versionFile := filepath.Join(filepath.Dir(execPath), ".aide-version")
	if err := os.WriteFile(versionFile, []byte(latestVersion), 0644); err != nil {
		// Non-fatal
		fmt.Printf("Warning: failed to write version file: %v\n", err)
	}

	fmt.Printf("\nSuccessfully upgraded to %s!\n", latestVersion)
	return nil
}

func getLatestRelease() (*GitHubRelease, error) {
	resp, err := http.Get(githubAPIURL + "/latest")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	var release GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, err
	}

	return &release, nil
}

func getRelease(version string) (*GitHubRelease, error) {
	// Ensure version has 'v' prefix
	if !strings.HasPrefix(version, "v") {
		version = "v" + version
	}

	url := fmt.Sprintf("%s/tags/%s", githubAPIURL, version)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("version %s not found", version)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	var release GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, err
	}

	return &release, nil
}

func getBinaryName() string {
	goos := runtime.GOOS
	goarch := runtime.GOARCH

	// Normalize arch names to match release artifacts
	if goarch == "amd64" {
		goarch = "amd64"
	}

	ext := ""
	if goos == "windows" {
		ext = ".exe"
	}

	return fmt.Sprintf("aide-%s-%s%s", goos, goarch, ext)
}

func downloadFile(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download returned %d", resp.StatusCode)
	}

	tmpFile, err := os.CreateTemp("", "aide-upgrade-*")
	if err != nil {
		return "", err
	}
	defer tmpFile.Close()

	_, err = io.Copy(tmpFile, resp.Body)
	if err != nil {
		os.Remove(tmpFile.Name())
		return "", err
	}

	return tmpFile.Name(), nil
}

func copyFile(src, dst string) error {
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	dest, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dest.Close()

	_, err = io.Copy(dest, source)
	return err
}

func printUpgradeUsage() {
	fmt.Println(`aide upgrade - Check for updates and upgrade to latest version

Usage:
  aide upgrade [VERSION] [options]

Arguments:
  VERSION            Target version to install (default: latest)

Options:
  --check            Check for updates without installing
  --force            Force download even if already up to date

Examples:
  aide upgrade                  # Upgrade to latest
  aide upgrade --check          # Check for updates only
  aide upgrade v0.0.5           # Install specific version
  aide upgrade --force          # Force re-download`)
}

// compareVersions compares two semver-like versions.
// Returns -1 if a < b, 0 if a == b, 1 if a > b
func compareVersions(a, b string) int {
	// Strip 'v' prefix
	a = strings.TrimPrefix(a, "v")
	b = strings.TrimPrefix(b, "v")

	// Handle dev versions (e.g., "0.1.0-dev.5")
	aParts := strings.Split(a, "-")
	bParts := strings.Split(b, "-")

	// Compare major.minor.patch
	aVer := strings.Split(aParts[0], ".")
	bVer := strings.Split(bParts[0], ".")

	for i := 0; i < 3; i++ {
		var aNum, bNum int
		if i < len(aVer) {
			fmt.Sscanf(aVer[i], "%d", &aNum)
		}
		if i < len(bVer) {
			fmt.Sscanf(bVer[i], "%d", &bNum)
		}

		if aNum < bNum {
			return -1
		}
		if aNum > bNum {
			return 1
		}
	}

	// Same base version, check prerelease
	// A version without prerelease is greater than one with prerelease
	if len(aParts) == 1 && len(bParts) > 1 {
		return 1 // a is release, b is prerelease
	}
	if len(aParts) > 1 && len(bParts) == 1 {
		return -1 // a is prerelease, b is release
	}

	// Both have prerelease, compare lexically
	if len(aParts) > 1 && len(bParts) > 1 {
		if aParts[1] < bParts[1] {
			return -1
		}
		if aParts[1] > bParts[1] {
			return 1
		}
	}

	return 0
}
