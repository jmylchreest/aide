package grammar

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// resolveDownloadURL — pure string template logic
// ---------------------------------------------------------------------------

func TestResolveDownloadURL(t *testing.T) {
	tests := []struct {
		name     string
		tmpl     string
		version  string
		asset    string
		langName string
		wantSub  string // substring the result must contain
	}{
		{
			name:     "default template",
			tmpl:     DefaultGrammarURL,
			version:  "grammars-v1",
			asset:    "aide-grammar-ruby-grammars-v1-linux-amd64.so",
			langName: "ruby",
			wantSub:  "releases/download/grammars-v1/aide-grammar-ruby",
		},
		{
			name:     "custom mirror with name",
			tmpl:     "https://mirror.example.com/{name}/{version}/{asset}",
			version:  "v2",
			asset:    "ruby.so",
			langName: "ruby",
			wantSub:  "mirror.example.com/ruby/v2/ruby.so",
		},
		{
			name:     "os and arch placeholders",
			tmpl:     "https://dl.example.com/{os}/{arch}/{asset}",
			version:  "v1",
			asset:    "grammar.so",
			langName: "test",
			wantSub:  CurrentPlatform().OS + "/" + CurrentPlatform().Arch,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveDownloadURL(tt.tmpl, tt.version, tt.asset, tt.langName)
			if !strings.Contains(got, tt.wantSub) {
				t.Errorf("resolveDownloadURL() = %q; want substring %q", got, tt.wantSub)
			}
		})
	}
}

func TestResolveDownloadURLNoPlaceholders(t *testing.T) {
	// A template with no placeholders should be returned as-is.
	tmpl := "https://static.example.com/grammar.so"
	got := resolveDownloadURL(tmpl, "v1", "grammar.so", "ruby")
	if got != tmpl {
		t.Errorf("resolveDownloadURL with no placeholders = %q; want %q", got, tmpl)
	}
}

// ---------------------------------------------------------------------------
// downloadGrammarAsset — HTTP download + SHA256 + atomic rename
// ---------------------------------------------------------------------------

func TestDownloadGrammarAsset(t *testing.T) {
	body := []byte("fake-shared-library-content-for-test")
	expectedHash := sha256Hex(body)

	// Start a test HTTP server that serves the body.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(body)
	}))
	defer srv.Close()

	dir := t.TempDir()
	destPath := filepath.Join(dir, "grammars", "aide-grammar-ruby-v1-linux-amd64.so")

	// Use the test server URL directly as the template (no placeholders needed).
	got, err := downloadGrammarAsset(context.Background(), srv.URL+"/{asset}", "ruby", "v1", destPath)
	if err != nil {
		t.Fatalf("downloadGrammarAsset: %v", err)
	}

	if got != expectedHash {
		t.Errorf("sha256: got %q, want %q", got, expectedHash)
	}

	// Verify the file exists with correct content.
	data, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("reading downloaded file: %v", err)
	}
	if string(data) != string(body) {
		t.Errorf("file content mismatch: got %d bytes, want %d", len(data), len(body))
	}

	// Verify the file is executable.
	info, err := os.Stat(destPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o100 == 0 {
		t.Errorf("file should be executable, got permissions %o", info.Mode().Perm())
	}
}

func TestDownloadGrammarAsset404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	dir := t.TempDir()
	destPath := filepath.Join(dir, "grammar.so")

	_, err := downloadGrammarAsset(context.Background(), srv.URL+"/{asset}", "ruby", "v1", destPath)
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
	if !strings.Contains(err.Error(), "HTTP 404") {
		t.Errorf("error = %q; want it to mention HTTP 404", err)
	}

	// Verify no file was left behind.
	if _, err := os.Stat(destPath); !os.IsNotExist(err) {
		t.Error("destination file should not exist after 404")
	}
}

func TestDownloadGrammarAssetCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Intentionally slow — but context should cancel before we respond.
		select {
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	dir := t.TempDir()
	destPath := filepath.Join(dir, "grammar.so")

	_, err := downloadGrammarAsset(ctx, srv.URL+"/{asset}", "ruby", "v1", destPath)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestDownloadGrammarAssetCreatesDir(t *testing.T) {
	body := []byte("content")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(body)
	}))
	defer srv.Close()

	dir := t.TempDir()
	// Nested directory that doesn't exist yet.
	destPath := filepath.Join(dir, "deep", "nested", "grammar.so")

	_, err := downloadGrammarAsset(context.Background(), srv.URL+"/{asset}", "test", "v1", destPath)
	if err != nil {
		t.Fatalf("downloadGrammarAsset: %v", err)
	}

	if _, err := os.Stat(destPath); err != nil {
		t.Errorf("file should exist at nested path: %v", err)
	}
}

// ---------------------------------------------------------------------------
// DefaultGrammarURL constant
// ---------------------------------------------------------------------------

func TestDefaultGrammarURL(t *testing.T) {
	if !strings.Contains(DefaultGrammarURL, "{version}") {
		t.Errorf("DefaultGrammarURL missing {version} placeholder: %s", DefaultGrammarURL)
	}
	if !strings.Contains(DefaultGrammarURL, "{asset}") {
		t.Errorf("DefaultGrammarURL missing {asset} placeholder: %s", DefaultGrammarURL)
	}
	if !strings.HasPrefix(DefaultGrammarURL, "https://") {
		t.Errorf("DefaultGrammarURL should use HTTPS: %s", DefaultGrammarURL)
	}
}

// sha256Hex returns the lowercase hex sha256 digest of data.
func sha256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return fmt.Sprintf("%s", hex.EncodeToString(h[:]))
}
