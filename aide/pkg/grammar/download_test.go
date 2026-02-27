package grammar

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
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
			asset:    "aide-grammar-ruby-grammars-v1-linux-amd64.tar.gz",
			langName: "ruby",
			wantSub:  "releases/download/grammars-v1/aide-grammar-ruby",
		},
		{
			name:     "custom mirror with name",
			tmpl:     "https://mirror.example.com/{name}/{version}/{asset}",
			version:  "v2",
			asset:    "ruby.tar.gz",
			langName: "ruby",
			wantSub:  "mirror.example.com/ruby/v2/ruby.tar.gz",
		},
		{
			name:     "os and arch placeholders",
			tmpl:     "https://dl.example.com/{os}/{arch}/{asset}",
			version:  "v1",
			asset:    "grammar.tar.gz",
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
	tmpl := "https://static.example.com/grammar.tar.gz"
	got := resolveDownloadURL(tmpl, "v1", "grammar.tar.gz", "ruby")
	if got != tmpl {
		t.Errorf("resolveDownloadURL with no placeholders = %q; want %q", got, tmpl)
	}
}

// ---------------------------------------------------------------------------
// downloadToFile — HTTP download + SHA256
// ---------------------------------------------------------------------------

func TestDownloadToFile(t *testing.T) {
	body := []byte("fake-archive-content-for-test")
	expectedHash := sha256Hex(body)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(body)
	}))
	defer srv.Close()

	dir := t.TempDir()
	destPath := filepath.Join(dir, "grammars", "archive.tar.gz")

	got, err := downloadToFile(context.Background(), srv.URL+"/archive.tar.gz", destPath)
	if err != nil {
		t.Fatalf("downloadToFile: %v", err)
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
}

func TestDownloadToFile404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	dir := t.TempDir()
	destPath := filepath.Join(dir, "archive.tar.gz")

	_, err := downloadToFile(context.Background(), srv.URL+"/missing.tar.gz", destPath)
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
	if !strings.Contains(err.Error(), "HTTP 404") {
		t.Errorf("error = %q; want it to mention HTTP 404", err)
	}
}

func TestDownloadToFileCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	dir := t.TempDir()
	destPath := filepath.Join(dir, "archive.tar.gz")

	_, err := downloadToFile(ctx, srv.URL+"/archive.tar.gz", destPath)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestDownloadToFileCreatesDir(t *testing.T) {
	body := []byte("content")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(body)
	}))
	defer srv.Close()

	dir := t.TempDir()
	destPath := filepath.Join(dir, "deep", "nested", "archive.tar.gz")

	_, err := downloadToFile(context.Background(), srv.URL+"/archive.tar.gz", destPath)
	if err != nil {
		t.Fatalf("downloadToFile: %v", err)
	}

	if _, err := os.Stat(destPath); err != nil {
		t.Errorf("file should exist at nested path: %v", err)
	}
}

// ---------------------------------------------------------------------------
// extractTarGz — archive extraction with security checks
// ---------------------------------------------------------------------------

// makeTarGz creates a .tar.gz archive file from a set of entries.
func makeTarGz(t *testing.T, destPath string, entries map[string][]byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(destPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	for name, data := range entries {
		hdr := &tar.Header{
			Name: name,
			Size: int64(len(data)),
			Mode: 0o644,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(data); err != nil {
			t.Fatal(err)
		}
	}
}

// makeTarGzBytes creates a .tar.gz archive in memory from a set of entries.
func makeTarGzBytes(t *testing.T, entries map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer

	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	for name, data := range entries {
		hdr := &tar.Header{
			Name:     name,
			Size:     int64(len(data)),
			Mode:     0o644,
			Typeflag: tar.TypeReg,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(data); err != nil {
			t.Fatal(err)
		}
	}

	tw.Close()
	gw.Close()
	return buf.Bytes()
}

func TestExtractTarGz(t *testing.T) {
	dir := t.TempDir()
	archivePath := filepath.Join(dir, "test.tar.gz")

	grammarData := []byte("fake-grammar-binary")
	packData := []byte(`{"schema_version": 1, "name": "ruby"}`)

	makeTarGz(t, archivePath, map[string][]byte{
		"ruby/grammar.so": grammarData,
		"ruby/pack.json":  packData,
	})

	hasPack, err := extractTarGz(archivePath, dir, "ruby")
	if err != nil {
		t.Fatalf("extractTarGz: %v", err)
	}
	if !hasPack {
		t.Error("expected hasPack = true")
	}

	// Verify extracted files.
	data, err := os.ReadFile(filepath.Join(dir, "ruby", "grammar.so"))
	if err != nil {
		t.Fatalf("reading grammar: %v", err)
	}
	if string(data) != string(grammarData) {
		t.Errorf("grammar content mismatch")
	}

	data, err = os.ReadFile(filepath.Join(dir, "ruby", "pack.json"))
	if err != nil {
		t.Fatalf("reading pack.json: %v", err)
	}
	if string(data) != string(packData) {
		t.Errorf("pack.json content mismatch")
	}
}

func TestExtractTarGzWithoutPackJSON(t *testing.T) {
	dir := t.TempDir()
	archivePath := filepath.Join(dir, "test.tar.gz")

	makeTarGz(t, archivePath, map[string][]byte{
		"ruby/grammar.so": []byte("binary"),
	})

	hasPack, err := extractTarGz(archivePath, dir, "ruby")
	if err != nil {
		t.Fatalf("extractTarGz: %v", err)
	}
	if hasPack {
		t.Error("expected hasPack = false when no pack.json in archive")
	}
}

func TestExtractTarGzSkipsWrongPrefix(t *testing.T) {
	dir := t.TempDir()
	archivePath := filepath.Join(dir, "test.tar.gz")

	// Archive contains files under "evil/" which don't match expected "ruby/" prefix.
	makeTarGz(t, archivePath, map[string][]byte{
		"evil/grammar.so": []byte("malicious-binary"),
		"ruby/grammar.so": []byte("real-binary"),
	})

	_, err := extractTarGz(archivePath, dir, "ruby")
	if err != nil {
		t.Fatalf("extractTarGz: %v", err)
	}

	// evil directory should NOT have been created.
	if _, err := os.Stat(filepath.Join(dir, "evil")); !os.IsNotExist(err) {
		t.Error("files outside expected prefix should not be extracted")
	}

	// ruby directory should exist.
	data, err := os.ReadFile(filepath.Join(dir, "ruby", "grammar.so"))
	if err != nil {
		t.Fatalf("ruby/grammar.so should exist: %v", err)
	}
	if string(data) != "real-binary" {
		t.Errorf("wrong content for ruby/grammar.so")
	}
}

func TestExtractTarGzSkipsUnexpectedFiles(t *testing.T) {
	dir := t.TempDir()
	archivePath := filepath.Join(dir, "test.tar.gz")

	// Archive contains an unexpected file (not grammar* or pack.json).
	makeTarGz(t, archivePath, map[string][]byte{
		"ruby/grammar.so": []byte("binary"),
		"ruby/README.md":  []byte("should be skipped"),
	})

	_, err := extractTarGz(archivePath, dir, "ruby")
	if err != nil {
		t.Fatalf("extractTarGz: %v", err)
	}

	// README.md should NOT be extracted.
	if _, err := os.Stat(filepath.Join(dir, "ruby", "README.md")); !os.IsNotExist(err) {
		t.Error("unexpected files should not be extracted")
	}
}

func TestExtractTarGzGrammarExecutable(t *testing.T) {
	dir := t.TempDir()
	archivePath := filepath.Join(dir, "test.tar.gz")

	makeTarGz(t, archivePath, map[string][]byte{
		"ruby/grammar.so": []byte("binary"),
		"ruby/pack.json":  []byte(`{}`),
	})

	_, err := extractTarGz(archivePath, dir, "ruby")
	if err != nil {
		t.Fatalf("extractTarGz: %v", err)
	}

	// Grammar binary should be executable.
	info, err := os.Stat(filepath.Join(dir, "ruby", "grammar.so"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o100 == 0 {
		t.Errorf("grammar binary should be executable, got permissions %o", info.Mode().Perm())
	}

	// pack.json should NOT be executable.
	info, err = os.Stat(filepath.Join(dir, "ruby", "pack.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o100 != 0 {
		t.Errorf("pack.json should not be executable, got permissions %o", info.Mode().Perm())
	}
}

// ---------------------------------------------------------------------------
// downloadAndExtractGrammarPack — full round-trip
// ---------------------------------------------------------------------------

func TestDownloadAndExtractGrammarPack(t *testing.T) {
	grammarData := []byte("fake-grammar-binary-for-download-test")
	packData := []byte(`{"schema_version": 1, "name": "ruby"}`)

	p := CurrentPlatform()
	archiveBytes := makeTarGzBytes(t, map[string][]byte{
		"ruby/grammar" + p.Ext: grammarData,
		"ruby/pack.json":       packData,
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(archiveBytes)
	}))
	defer srv.Close()

	dir := t.TempDir()
	sha, hasPack, err := downloadAndExtractGrammarPack(
		context.Background(),
		srv.URL+"/{version}/{asset}",
		"ruby", "v1.0.0", dir,
	)
	if err != nil {
		t.Fatalf("downloadAndExtractGrammarPack: %v", err)
	}

	if !hasPack {
		t.Error("expected hasPack = true")
	}

	expectedSHA := sha256Hex(archiveBytes)
	if sha != expectedSHA {
		t.Errorf("sha256: got %q, want %q", sha, expectedSHA)
	}

	// Verify extracted files.
	data, err := os.ReadFile(filepath.Join(dir, "ruby", "grammar"+p.Ext))
	if err != nil {
		t.Fatalf("reading grammar: %v", err)
	}
	if string(data) != string(grammarData) {
		t.Error("grammar content mismatch")
	}

	data, err = os.ReadFile(filepath.Join(dir, "ruby", "pack.json"))
	if err != nil {
		t.Fatalf("reading pack.json: %v", err)
	}
	if string(data) != string(packData) {
		t.Error("pack.json content mismatch")
	}
}

func TestDownloadAndExtractGrammarPack404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	dir := t.TempDir()
	_, _, err := downloadAndExtractGrammarPack(
		context.Background(),
		srv.URL+"/{version}/{asset}",
		"ruby", "v1.0.0", dir,
	)
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
	if !strings.Contains(err.Error(), "HTTP 404") {
		t.Errorf("error = %q; want it to mention HTTP 404", err)
	}
}

func TestDownloadAndExtractGrammarPackCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	dir := t.TempDir()
	_, _, err := downloadAndExtractGrammarPack(ctx, srv.URL+"/{version}/{asset}", "ruby", "v1.0.0", dir)
	if err == nil {
		t.Fatal("expected error for cancelled context")
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
	return hex.EncodeToString(h[:])
}
