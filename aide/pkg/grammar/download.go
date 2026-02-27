package grammar

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/httputil"
)

const (
	// DefaultGrammarURL is the default URL template for downloading grammar assets.
	// Supported placeholders:
	//   {version} — grammar release version tag (e.g., "v0.1.0", "snapshot")
	//   {asset}   — full asset filename (e.g., "aide-grammar-ruby-v0.1.0-linux-amd64.tar.gz")
	//   {name}    — grammar name (e.g., "ruby")
	//   {os}      — operating system (e.g., "linux", "darwin", "windows")
	//   {arch}    — architecture (e.g., "amd64", "arm64")
	DefaultGrammarURL = "https://github.com/jmylchreest/aide/releases/download/{version}/{asset}"

	// maxArchiveSize is a safety limit for grammar pack archives (50 MiB).
	maxArchiveSize = 50 << 20

	// maxFileSize is a safety limit for individual files extracted from a
	// grammar pack archive (10 MiB). This is lower than maxArchiveSize to
	// prevent a single entry from consuming the entire budget.
	maxFileSize = 10 << 20

	// maxArchiveEntries limits the number of entries extracted from a tar to
	// prevent zip-bomb style attacks.
	maxArchiveEntries = 20

	// httpTimeout is the maximum time for a grammar download HTTP request.
	httpTimeout = 2 * time.Minute
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

// downloadAndExtractGrammarPack downloads a grammar pack archive (.tar.gz)
// and extracts it to destDir/{name}/. The archive is expected to contain
// files prefixed with {name}/ (e.g., "ruby/grammar.so", "ruby/pack.json").
//
// Returns the SHA256 checksum of the downloaded archive and whether a
// pack.json was found in the archive.
func downloadAndExtractGrammarPack(ctx context.Context, urlTemplate, name, version, destDir string) (sha256sum string, hasPack bool, err error) {
	// Ensure destination directory exists.
	grammarDir := filepath.Join(destDir, name)
	if mkErr := os.MkdirAll(grammarDir, 0o755); mkErr != nil {
		return "", false, fmt.Errorf("creating grammar directory: %w", mkErr)
	}

	// Build the download URL from the template.
	assetName := PackArchiveFilename(name, version)
	url := resolveDownloadURL(urlTemplate, version, assetName, name)

	// Download to a temporary file.
	tmpPath := filepath.Join(destDir, assetName+".tmp")
	archiveSHA, dlErr := downloadToFile(ctx, url, tmpPath)
	if dlErr != nil {
		return "", false, dlErr
	}
	defer os.Remove(tmpPath) // Clean up the temp archive after extraction.

	// Extract the archive into destDir.
	hasPack, extractErr := extractTarGz(tmpPath, destDir, name)
	if extractErr != nil {
		// Clean up partial extraction.
		os.RemoveAll(grammarDir)
		return "", false, fmt.Errorf("extracting grammar pack: %w", extractErr)
	}

	return archiveSHA, hasPack, nil
}

// grammarHTTPClient is the shared retry-capable client for grammar downloads.
var grammarHTTPClient = httputil.NewClient(
	httputil.WithHTTPTimeout(httpTimeout),
)

// downloadToFile downloads a URL to a local file, computing SHA256 on the fly.
// Returns the hex-encoded SHA256 checksum. Retries transient failures (429,
// 502, 503, connection resets) with exponential backoff.
func downloadToFile(ctx context.Context, url, destPath string) (string, error) {
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return "", fmt.Errorf("creating directory: %w", err)
	}

	resp, err := grammarHTTPClient.Get(ctx, url)
	if err != nil {
		return "", fmt.Errorf("downloading %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download returned HTTP %d for %s", resp.StatusCode, url)
	}

	f, err := os.Create(destPath)
	if err != nil {
		return "", fmt.Errorf("creating temp file: %w", err)
	}

	hasher := sha256.New()
	writer := io.MultiWriter(f, hasher)

	// Limit read size to prevent downloading excessively large files.
	limited := io.LimitReader(resp.Body, maxArchiveSize+1)
	n, copyErr := io.Copy(writer, limited)
	if copyErr != nil {
		f.Close()
		os.Remove(destPath)
		return "", fmt.Errorf("writing file: %w", copyErr)
	}
	if n > maxArchiveSize {
		f.Close()
		os.Remove(destPath)
		return "", fmt.Errorf("archive exceeds maximum size of %d bytes", maxArchiveSize)
	}

	if err := f.Close(); err != nil {
		os.Remove(destPath)
		return "", fmt.Errorf("closing file: %w", err)
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// extractTarGz extracts a .tar.gz archive to destDir. Only files under the
// expected {name}/ prefix are extracted. Returns whether a pack.json was found.
func extractTarGz(archivePath, destDir, name string) (bool, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return false, err
	}
	defer f.Close()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		return false, fmt.Errorf("opening gzip: %w", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	hasPack := false
	entries := 0
	expectedPrefix := name + "/"

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return false, fmt.Errorf("reading tar entry: %w", err)
		}

		entries++
		if entries > maxArchiveEntries {
			return false, fmt.Errorf("archive has too many entries (max %d)", maxArchiveEntries)
		}

		// Only process regular files.
		if hdr.Typeflag != tar.TypeReg {
			continue
		}

		// Normalise to forward slashes for consistent prefix matching.
		entryName := filepath.ToSlash(filepath.Clean(hdr.Name))

		// Validate the path: must be under {name}/ prefix.
		if !strings.HasPrefix(entryName, expectedPrefix) {
			continue // Skip files not under expected prefix.
		}

		// Prevent path traversal.
		relPath := strings.TrimPrefix(entryName, expectedPrefix)
		if relPath == "" || strings.Contains(relPath, "..") || filepath.IsAbs(relPath) {
			continue
		}

		// Only allow expected files: grammar binary or pack.json.
		baseName := filepath.Base(relPath)
		if baseName == "pack.json" {
			hasPack = true
		} else if !strings.HasPrefix(baseName, "grammar") {
			continue // Skip unexpected files.
		}

		destPath := filepath.Join(destDir, name, relPath)
		if mkErr := os.MkdirAll(filepath.Dir(destPath), 0o755); mkErr != nil {
			return false, fmt.Errorf("creating directory for %s: %w", relPath, mkErr)
		}

		outFile, createErr := os.Create(destPath)
		if createErr != nil {
			return false, fmt.Errorf("creating %s: %w", destPath, createErr)
		}

		// Limit extraction size per file.
		limited := io.LimitReader(tr, maxFileSize)
		if _, copyErr := io.Copy(outFile, limited); copyErr != nil {
			outFile.Close()
			return false, fmt.Errorf("extracting %s: %w", relPath, copyErr)
		}

		if closeErr := outFile.Close(); closeErr != nil {
			return false, fmt.Errorf("closing %s: %w", destPath, closeErr)
		}

		// Make binaries executable.
		if baseName != "pack.json" {
			_ = os.Chmod(destPath, 0o755)
		}
	}

	return hasPack, nil
}
