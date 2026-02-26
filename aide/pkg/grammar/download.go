package grammar

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const (
	// DefaultGrammarURL is the default URL template for downloading grammar assets.
	// Supported placeholders:
	//   {version} — grammar release version tag (e.g., "grammars-v1", "grammars-latest")
	//   {asset}   — full asset filename (e.g., "aide-grammar-ruby-grammars-v1-linux-amd64.so")
	//   {name}    — grammar name (e.g., "ruby")
	//   {os}      — operating system (e.g., "linux", "darwin", "windows")
	//   {arch}    — architecture (e.g., "amd64", "arm64")
	DefaultGrammarURL = "https://github.com/jmylchreest/aide/releases/download/{version}/{asset}"
)

// resolveDownloadURL expands a URL template with the given values.
func resolveDownloadURL(urlTemplate, version, asset, name string) string {
	p := CurrentPlatform()
	r := strings.NewReplacer(
		"{version}", version,
		"{asset}", asset,
		"{name}", name,
		"{os}", p.OS,
		"{arch}", p.Arch,
	)
	return r.Replace(urlTemplate)
}

// downloadGrammarAsset downloads a grammar shared library using the given URL template.
// Returns the SHA256 checksum of the downloaded file.
func downloadGrammarAsset(ctx context.Context, urlTemplate, name, version, destPath string) (string, error) {
	// Ensure destination directory exists.
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return "", fmt.Errorf("creating grammar directory: %w", err)
	}

	// Build the download URL from the template.
	assetName := filepath.Base(destPath)
	url := resolveDownloadURL(urlTemplate, version, assetName, name)

	// Download
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("downloading %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download returned HTTP %d for %s", resp.StatusCode, url)
	}

	// Write to a temporary file first, then rename for atomicity
	tmpPath := destPath + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return "", fmt.Errorf("creating temp file: %w", err)
	}

	hasher := sha256.New()
	writer := io.MultiWriter(f, hasher)

	if _, err := io.Copy(writer, resp.Body); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return "", fmt.Errorf("writing grammar file: %w", err)
	}

	if err := f.Close(); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("closing grammar file: %w", err)
	}

	// Make the library executable (needed on some platforms)
	if err := os.Chmod(tmpPath, 0o755); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("setting permissions: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpPath, destPath); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("renaming grammar file: %w", err)
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}
