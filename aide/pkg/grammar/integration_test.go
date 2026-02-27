package grammar

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// ---------------------------------------------------------------------------
// Integration tests: exercise Download → Manifest → Load pipeline end-to-end
// using a local HTTP test server. These do NOT require real shared libraries;
// they verify the download, file-system, and manifest mechanics.
// ---------------------------------------------------------------------------

// makeGrammarArchive builds a .tar.gz archive in memory containing
// {name}/grammar{ext} with the given body and {name}/pack.json.
func makeGrammarArchive(t *testing.T, name string, body []byte) []byte {
	t.Helper()
	p := CurrentPlatform()
	var buf bytes.Buffer

	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	// Grammar binary entry.
	grammarEntry := name + "/grammar" + p.Ext
	hdr := &tar.Header{Name: grammarEntry, Size: int64(len(body)), Mode: 0o755, Typeflag: tar.TypeReg}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(body); err != nil {
		t.Fatal(err)
	}

	// pack.json entry — include c_symbol and source_repo so the pack looks
	// realistic when LoadFromDir overwrites the registry entry.
	cSymbol := "tree_sitter_" + strings.ReplaceAll(name, "-", "_")
	packContent := []byte(`{"schema_version": 1, "name": "` + name + `", "c_symbol": "` + cSymbol + `", "source_repo": "tree-sitter/tree-sitter-` + name + `", "meta": {"extensions": []}}`)
	phdr := &tar.Header{Name: name + "/pack.json", Size: int64(len(packContent)), Mode: 0o644, Typeflag: tar.TypeReg}
	if err := tw.WriteHeader(phdr); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(packContent); err != nil {
		t.Fatal(err)
	}

	tw.Close()
	gw.Close()
	return buf.Bytes()
}

// newTestArchiveServer returns a test HTTP server that serves .tar.gz archives
// containing a grammar for whatever language is requested. The grammar binary
// content comes from body. Returns a request counter.
func newTestArchiveServer(t *testing.T, body []byte) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	var count atomic.Int64
	// Pre-build archives for common test languages.
	archives := map[string][]byte{}
	for _, lang := range []string{"ruby", "php", "lua", "bash", "kotlin", "testlang"} {
		archives[lang] = makeGrammarArchive(t, lang, body)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count.Add(1)
		// Extract language name from the URL path (contains aide-grammar-{name}-...).
		path := r.URL.Path
		for lang, archive := range archives {
			if strings.Contains(path, "aide-grammar-"+lang+"-") {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(archive)
				return
			}
		}
		// Fallback: serve a generic archive using "unknown" prefix.
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(makeGrammarArchive(t, "unknown", body))
	}))
	t.Cleanup(srv.Close)
	return srv, &count
}

// newTest404Server returns a test HTTP server that always returns 404.
func newTest404Server(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// ---------------------------------------------------------------------------
// DynamicLoader.Download → manifest + file-on-disk
// ---------------------------------------------------------------------------

func TestIntegrationDownloadWritesFileAndManifest(t *testing.T) {
	body := []byte("fake-shared-library-bytes")
	srv, reqCount := newTestArchiveServer(t, body)

	dir := t.TempDir()
	dl := NewDynamicLoader(dir)
	dl.baseURL = srv.URL + "/{version}/{asset}"
	dl.version = "v0.1.0"

	ctx := context.Background()
	pack := &Pack{
		Name:       "ruby",
		SourceRepo: "tree-sitter/tree-sitter-ruby",
		CSymbol:    "tree_sitter_ruby",
	}

	if err := dl.Download(ctx, "ruby", pack); err != nil {
		t.Fatalf("Download: %v", err)
	}

	// Verify exactly one HTTP request was made.
	if got := reqCount.Load(); got != 1 {
		t.Errorf("expected 1 request, got %d", got)
	}

	// Verify the grammar binary exists on disk with correct content.
	expectedFilename := LibraryFilename("ruby")
	filePath := filepath.Join(dir, expectedFilename)
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("reading downloaded file: %v", err)
	}
	if string(data) != string(body) {
		t.Errorf("file content mismatch: got %d bytes, want %d", len(data), len(body))
	}

	// Verify pack.json was also extracted.
	packPath := filepath.Join(dir, "ruby", "pack.json")
	if _, err := os.Stat(packPath); err != nil {
		t.Errorf("pack.json should exist: %v", err)
	}

	// Verify the manifest was updated.
	entry := dl.manifest.get("ruby")
	if entry == nil {
		t.Fatal("manifest entry for ruby is nil after download")
	}
	if entry.Version != "v0.1.0" {
		t.Errorf("manifest version = %q; want %q", entry.Version, "v0.1.0")
	}
	if entry.File != expectedFilename {
		t.Errorf("manifest file = %q; want %q", entry.File, expectedFilename)
	}
	if entry.CSymbol != "tree_sitter_ruby" {
		t.Errorf("manifest c_symbol = %q; want %q", entry.CSymbol, "tree_sitter_ruby")
	}
	if entry.SHA256 == "" {
		t.Error("manifest sha256 should be set")
	}
	if !entry.HasPack {
		t.Error("manifest HasPack should be true")
	}

	// Verify the manifest was persisted to disk.
	manifestPath := filepath.Join(dir, "manifest.json")
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("reading manifest.json: %v", err)
	}
	var m Manifest
	if err := json.Unmarshal(manifestData, &m); err != nil {
		t.Fatalf("parsing manifest.json: %v", err)
	}
	if _, ok := m.Grammars["ruby"]; !ok {
		t.Error("manifest.json missing ruby entry")
	}
}

// ---------------------------------------------------------------------------
// DynamicLoader.Download with version fallback chain
// ---------------------------------------------------------------------------

func TestIntegrationDownloadVersionFallback(t *testing.T) {
	var requestedURL atomic.Value
	archiveBytes := makeGrammarArchive(t, "testlang", []byte("lib"))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedURL.Store(r.URL.Path)
		_, _ = w.Write(archiveBytes)
	}))
	t.Cleanup(srv.Close)

	tests := []struct {
		name      string
		dlVersion string // DynamicLoader.version
		wantInURL string // substring expected in the request URL
	}{
		{
			name:      "loader version used when set",
			dlVersion: "v0.2.0",
			wantInURL: "v0.2.0",
		},
		{
			name:      "falls back to snapshot when empty",
			dlVersion: "",
			wantInURL: "snapshot",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			dl := NewDynamicLoader(dir)
			dl.baseURL = srv.URL + "/{version}/{asset}"
			dl.version = tt.dlVersion

			pack := &Pack{
				Name:       "testlang",
				SourceRepo: "test/test-grammar",
				CSymbol:    "tree_sitter_test",
			}

			if err := dl.Download(context.Background(), "testlang", pack); err != nil {
				t.Fatalf("Download: %v", err)
			}

			got, _ := requestedURL.Load().(string)
			if !strings.Contains(got, tt.wantInURL) {
				t.Errorf("request URL %q does not contain %q", got, tt.wantInURL)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Download + Remove round-trip
// ---------------------------------------------------------------------------

func TestIntegrationDownloadThenRemove(t *testing.T) {
	body := []byte("removable-library")
	srv, _ := newTestArchiveServer(t, body)

	dir := t.TempDir()
	dl := NewDynamicLoader(dir)
	dl.baseURL = srv.URL + "/{version}/{asset}"
	dl.version = "v1.0.0"

	pack := &Pack{
		Name:       "php",
		SourceRepo: "tree-sitter/tree-sitter-php",
		CSymbol:    "tree_sitter_php",
	}

	// Download
	if err := dl.Download(context.Background(), "php", pack); err != nil {
		t.Fatalf("Download: %v", err)
	}

	// Verify installed.
	infos := dl.Installed()
	if len(infos) != 1 {
		t.Fatalf("Installed after download: got %d, want 1", len(infos))
	}
	if infos[0].Name != "php" {
		t.Errorf("installed name = %q; want %q", infos[0].Name, "php")
	}

	// Remove
	if err := dl.Remove("php"); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	// Verify no longer installed.
	infos = dl.Installed()
	if len(infos) != 0 {
		t.Errorf("Installed after remove: got %d, want 0", len(infos))
	}

	// Verify the grammar subdirectory is gone from disk.
	grammarDir := filepath.Join(dir, "php")
	if _, err := os.Stat(grammarDir); !os.IsNotExist(err) {
		t.Error("grammar directory should be deleted after Remove")
	}

	// Verify manifest entry is gone.
	if entry := dl.manifest.get("php"); entry != nil {
		t.Error("manifest entry should be nil after Remove")
	}
}

// ---------------------------------------------------------------------------
// Download multiple grammars
// ---------------------------------------------------------------------------

func TestIntegrationDownloadMultiple(t *testing.T) {
	body := []byte("grammar-lib")
	srv, reqCount := newTestArchiveServer(t, body)

	dir := t.TempDir()
	dl := NewDynamicLoader(dir)
	dl.baseURL = srv.URL + "/{version}/{asset}"
	dl.version = "v0.5.0"

	grammars := map[string]*Pack{
		"ruby": {Name: "ruby", SourceRepo: "tree-sitter/tree-sitter-ruby", CSymbol: "tree_sitter_ruby"},
		"php":  {Name: "php", SourceRepo: "tree-sitter/tree-sitter-php", CSymbol: "tree_sitter_php"},
		"lua":  {Name: "lua", SourceRepo: "tree-sitter-grammars/tree-sitter-lua", CSymbol: "tree_sitter_lua"},
	}

	for name, pack := range grammars {
		if err := dl.Download(context.Background(), name, pack); err != nil {
			t.Fatalf("Download(%s): %v", name, err)
		}
	}

	// Verify 3 HTTP requests.
	if got := reqCount.Load(); got != 3 {
		t.Errorf("expected 3 requests, got %d", got)
	}

	// Verify all installed.
	infos := dl.Installed()
	if len(infos) != 3 {
		t.Fatalf("Installed: got %d, want 3", len(infos))
	}

	installed := make(map[string]bool)
	for _, info := range infos {
		installed[info.Name] = true
	}
	for name := range grammars {
		if !installed[name] {
			t.Errorf("grammar %q not in Installed() result", name)
		}
	}

	// Verify manifest persistence — reload from disk.
	ms2 := newManifestStore(dir)
	if err := ms2.load(); err != nil {
		t.Fatalf("reload manifest: %v", err)
	}
	entries := ms2.entries()
	if len(entries) != 3 {
		t.Errorf("reloaded manifest has %d entries, want 3", len(entries))
	}
}

// ---------------------------------------------------------------------------
// CompositeLoader.Install via HTTP test server
// ---------------------------------------------------------------------------

func TestIntegrationCompositeLoaderInstall(t *testing.T) {
	body := []byte("composite-grammar-lib")
	srv, _ := newTestArchiveServer(t, body)

	dir := t.TempDir()
	cl := NewCompositeLoader(
		WithGrammarDir(dir),
		WithBaseURL(srv.URL+"/{version}/{asset}"),
		WithVersion("v0.1.0"),
		WithAutoDownload(false),
	)

	// Install a known dynamic grammar.
	if err := cl.Install(context.Background(), "ruby"); err != nil {
		t.Fatalf("Install: %v", err)
	}

	// Verify it appears in Installed().
	installed := cl.Installed()
	found := false
	for _, info := range installed {
		if info.Name == "ruby" && !info.BuiltIn {
			found = true
			if info.Version != "v0.1.0" {
				t.Errorf("installed version = %q; want %q", info.Version, "v0.1.0")
			}
			break
		}
	}
	if !found {
		t.Error("ruby not found in Installed() after Install")
	}

	// Total installed should be 9 builtins + 1 dynamic.
	expectedCount := len(expectedBuiltins) + 1
	if len(installed) != expectedCount {
		t.Errorf("Installed() count = %d; want %d", len(installed), expectedCount)
	}
}

// ---------------------------------------------------------------------------
// CompositeLoader.Install — download failure propagation
// ---------------------------------------------------------------------------

func TestIntegrationCompositeLoaderInstallFailure(t *testing.T) {
	srv := newTest404Server(t)

	dir := t.TempDir()
	cl := NewCompositeLoader(
		WithGrammarDir(dir),
		WithBaseURL(srv.URL+"/{version}/{asset}"),
		WithVersion("v0.1.0"),
		WithAutoDownload(false),
	)

	err := cl.Install(context.Background(), "ruby")
	if err == nil {
		t.Fatal("expected error from 404 server")
	}
	if _, ok := err.(*DownloadFailedError); !ok {
		t.Errorf("error type = %T; want *DownloadFailedError", err)
	}

	// Verify nothing was installed.
	for _, info := range cl.Installed() {
		if info.Name == "ruby" {
			t.Error("ruby should not appear in Installed() after failed download")
		}
	}
}

// ---------------------------------------------------------------------------
// CompositeLoader auto-download on Load
// ---------------------------------------------------------------------------

func TestIntegrationCompositeLoaderAutoDownloadOnLoad(t *testing.T) {
	// We can't actually load a fake .so via purego, so the Load will download
	// the file but then fail at Dlopen. We verify auto-download was triggered
	// and the Dlopen error is propagated (not masked as GrammarNotFoundError).
	body := []byte("not-a-real-shared-library")
	srv, reqCount := newTestArchiveServer(t, body)

	dir := t.TempDir()
	cl := NewCompositeLoader(
		WithGrammarDir(dir),
		WithBaseURL(srv.URL+"/{version}/{asset}"),
		WithVersion("v0.1.0"),
		WithAutoDownload(true),
	)

	_, err := cl.Load(context.Background(), "bash")
	// Load will error because we can't Dlopen a fake file — that's expected.
	if err == nil {
		t.Fatal("expected error loading fake .so file")
	}

	// The key assertion: auto-download was triggered (HTTP request made).
	if reqCount.Load() == 0 {
		t.Error("expected HTTP request for auto-download, got none")
	}

	// The error should be from Dlopen, NOT GrammarNotFoundError — the download
	// succeeded and the error should reflect the actual loading failure.
	if _, ok := err.(*GrammarNotFoundError); ok {
		t.Error("got GrammarNotFoundError — should propagate Dlopen error instead")
	}

	// The grammar binary should exist on disk — download succeeded even though Load failed.
	filename := LibraryFilename("bash")
	if _, statErr := os.Stat(filepath.Join(dir, filename)); statErr != nil {
		t.Errorf("downloaded file should exist: %v", statErr)
	}

	// Manifest should have the entry.
	entry := cl.dynamic.manifest.get("bash")
	if entry == nil {
		t.Error("manifest should have bash entry after auto-download")
	}
}

// ---------------------------------------------------------------------------
// CompositeLoader.InstallFromLock with HTTP download
// ---------------------------------------------------------------------------

func TestIntegrationCompositeLoaderInstallFromLock(t *testing.T) {
	body := []byte("lock-grammar-lib")
	srv, reqCount := newTestArchiveServer(t, body)

	dir := t.TempDir()
	cl := NewCompositeLoader(
		WithGrammarDir(dir),
		WithBaseURL(srv.URL+"/{version}/{asset}"),
		WithVersion("v0.1.0"),
		WithAutoDownload(false),
	)

	// Create a lock file requesting ruby and lua (dynamic) plus go (builtin).
	lf := &LockFile{
		Grammars: map[string]*LockEntry{
			"go":   {Version: "v0.1.0", CSymbol: "tree_sitter_go"},
			"ruby": {Version: "v0.1.0", CSymbol: "tree_sitter_ruby"},
			"lua":  {Version: "v0.1.0", CSymbol: "tree_sitter_lua"},
		},
	}

	installed, err := cl.InstallFromLock(context.Background(), lf)
	if err != nil {
		t.Fatalf("InstallFromLock: %v", err)
	}

	// "go" is builtin — should be skipped. "ruby" and "lua" should be installed.
	if len(installed) != 2 {
		t.Errorf("installed count = %d; want 2 (ruby, lua)", len(installed))
	}

	installedSet := make(map[string]bool)
	for _, name := range installed {
		installedSet[name] = true
	}
	if !installedSet["ruby"] {
		t.Error("ruby should be in installed list")
	}
	if !installedSet["lua"] {
		t.Error("lua should be in installed list")
	}
	if installedSet["go"] {
		t.Error("go (builtin) should NOT be in installed list")
	}

	// Verify 2 HTTP requests (not 3 — go is skipped).
	if got := reqCount.Load(); got != 2 {
		t.Errorf("expected 2 HTTP requests, got %d", got)
	}
}

// ---------------------------------------------------------------------------
// GenerateLockFile after installation
// ---------------------------------------------------------------------------

func TestIntegrationGenerateLockFileAfterInstall(t *testing.T) {
	body := []byte("lockfile-test-lib")
	srv, _ := newTestArchiveServer(t, body)

	dir := t.TempDir()
	cl := NewCompositeLoader(
		WithGrammarDir(dir),
		WithBaseURL(srv.URL+"/{version}/{asset}"),
		WithVersion("v0.2.0"),
		WithAutoDownload(false),
	)

	// Install two grammars.
	for _, name := range []string{"ruby", "php"} {
		if err := cl.Install(context.Background(), name); err != nil {
			t.Fatalf("Install(%s): %v", name, err)
		}
	}

	// Generate lock file.
	lf := cl.GenerateLockFile()
	if len(lf.Grammars) != 2 {
		t.Fatalf("LockFile grammars = %d; want 2", len(lf.Grammars))
	}

	for _, name := range []string{"ruby", "php"} {
		entry, ok := lf.Grammars[name]
		if !ok {
			t.Errorf("LockFile missing %q", name)
			continue
		}
		if entry.Version != "v0.2.0" {
			t.Errorf("LockFile[%s].Version = %q; want %q", name, entry.Version, "v0.2.0")
		}
		// SHA256 is of the archive, not the raw body — just check it's non-empty.
		if entry.SHA256 == "" {
			t.Errorf("LockFile[%s].SHA256 should be set", name)
		}
	}
}

// ---------------------------------------------------------------------------
// Download overwrites existing grammar with newer version
// ---------------------------------------------------------------------------

func TestIntegrationDownloadOverwritesExisting(t *testing.T) {
	v1Body := []byte("version-1-library")
	v2Body := []byte("version-2-library-updated")

	var currentArchive atomic.Value
	currentArchive.Store(makeGrammarArchive(t, "ruby", v1Body))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		archive, _ := currentArchive.Load().([]byte)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(archive)
	}))
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	dl := NewDynamicLoader(dir)
	dl.baseURL = srv.URL + "/{version}/{asset}"
	dl.version = "v1.0.0"

	def := &Pack{
		Name:       "ruby",
		SourceRepo: "tree-sitter/tree-sitter-ruby",
		CSymbol:    "tree_sitter_ruby",
	}

	// Download v1
	if err := dl.Download(context.Background(), "ruby", def); err != nil {
		t.Fatalf("Download v1: %v", err)
	}

	entryV1 := dl.manifest.get("ruby")
	if entryV1 == nil {
		t.Fatal("manifest entry nil after v1 download")
	}
	sha1 := entryV1.SHA256

	// Download v2 (different body, same version tag to simulate overwrite)
	currentArchive.Store(makeGrammarArchive(t, "ruby", v2Body))
	if err := dl.Download(context.Background(), "ruby", def); err != nil {
		t.Fatalf("Download v2: %v", err)
	}

	entryV2 := dl.manifest.get("ruby")
	if entryV2 == nil {
		t.Fatal("manifest entry nil after v2 download")
	}

	// SHA should change because archive content changed.
	if entryV2.SHA256 == sha1 {
		t.Error("SHA256 should differ after overwrite with different content")
	}

	// File on disk should have v2 content.
	filePath := filepath.Join(dir, entryV2.File)
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("reading overwritten file: %v", err)
	}
	if string(data) != string(v2Body) {
		t.Error("file content should be v2 after overwrite")
	}
}

// ---------------------------------------------------------------------------
// Manifest persistence survives DynamicLoader recreation
// ---------------------------------------------------------------------------

func TestIntegrationManifestPersistence(t *testing.T) {
	body := []byte("persistent-grammar")
	srv, _ := newTestArchiveServer(t, body)

	dir := t.TempDir()

	// First loader: download a grammar.
	dl1 := NewDynamicLoader(dir)
	dl1.baseURL = srv.URL + "/{version}/{asset}"
	dl1.version = "v1.0.0"

	def := &Pack{
		Name:       "ruby",
		SourceRepo: "tree-sitter/tree-sitter-ruby",
		CSymbol:    "tree_sitter_ruby",
	}
	if err := dl1.Download(context.Background(), "ruby", def); err != nil {
		t.Fatalf("Download: %v", err)
	}

	// Second loader: creates a new DynamicLoader pointing to the same dir.
	dl2 := NewDynamicLoader(dir)

	// Should find the grammar via manifest loaded from disk.
	entry := dl2.manifest.get("ruby")
	if entry == nil {
		t.Fatal("new DynamicLoader should load manifest from disk")
	}
	if entry.Version != "v1.0.0" {
		t.Errorf("persisted version = %q; want %q", entry.Version, "v1.0.0")
	}
	if entry.CSymbol != "tree_sitter_ruby" {
		t.Errorf("persisted c_symbol = %q; want %q", entry.CSymbol, "tree_sitter_ruby")
	}

	// Should appear in Installed().
	infos := dl2.Installed()
	if len(infos) != 1 || infos[0].Name != "ruby" {
		t.Errorf("Installed on new loader: got %v; want [ruby]", infos)
	}
}

// ---------------------------------------------------------------------------
// Context cancellation during download
// ---------------------------------------------------------------------------

func TestIntegrationDownloadContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Wait for client to cancel.
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	dl := NewDynamicLoader(dir)
	dl.baseURL = srv.URL + "/{version}/{asset}"
	dl.version = "v1.0.0"

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	def := &Pack{
		Name:       "ruby",
		SourceRepo: "tree-sitter/tree-sitter-ruby",
		CSymbol:    "tree_sitter_ruby",
	}

	err := dl.Download(ctx, "ruby", def)
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
	if _, ok := err.(*DownloadFailedError); !ok {
		t.Errorf("error type = %T; want *DownloadFailedError", err)
	}

	// Manifest should remain empty.
	if entry := dl.manifest.get("ruby"); entry != nil {
		t.Error("manifest should be empty after failed download")
	}
}

// ---------------------------------------------------------------------------
// URL template gets correct version, name, platform values
// ---------------------------------------------------------------------------

func TestIntegrationDownloadURLTemplateExpansion(t *testing.T) {
	var capturedPath atomic.Value
	archiveBytes := makeGrammarArchive(t, "ruby", []byte("lib"))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath.Store(r.URL.Path)
		_, _ = w.Write(archiveBytes)
	}))
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	dl := NewDynamicLoader(dir)
	// Template: /{version}/{name}/{os}/{arch}/{asset}
	dl.baseURL = srv.URL + "/{version}/{name}/{os}/{arch}/{asset}"
	dl.version = "v0.3.0"

	def := &Pack{
		Name:       "ruby",
		SourceRepo: "tree-sitter/tree-sitter-ruby",
		CSymbol:    "tree_sitter_ruby",
	}

	if err := dl.Download(context.Background(), "ruby", def); err != nil {
		t.Fatalf("Download: %v", err)
	}

	got, _ := capturedPath.Load().(string)
	p := CurrentPlatform()

	// Check all template placeholders were expanded.
	if !strings.Contains(got, "/v0.3.0/") {
		t.Errorf("URL path %q missing version", got)
	}
	if !strings.Contains(got, "/ruby/") {
		t.Errorf("URL path %q missing name", got)
	}
	if !strings.Contains(got, "/"+p.OS+"/") {
		t.Errorf("URL path %q missing OS %q", got, p.OS)
	}
	if !strings.Contains(got, "/"+p.Arch+"/") {
		t.Errorf("URL path %q missing arch %q", got, p.Arch)
	}
	if !strings.Contains(got, "aide-grammar-ruby-") {
		t.Errorf("URL path %q missing asset prefix", got)
	}
}

// ---------------------------------------------------------------------------
// Staleness detection: DynamicLoader.Load returns GrammarStaleError when the
// installed grammar version differs from the loader's version.
// ---------------------------------------------------------------------------

func TestIntegrationLoadReturnsStaleError(t *testing.T) {
	body := []byte("stale-grammar-lib")
	srv, _ := newTestArchiveServer(t, body)

	dir := t.TempDir()
	dl := NewDynamicLoader(dir)
	dl.baseURL = srv.URL + "/{version}/{asset}"
	dl.version = "v0.1.0"

	def := &Pack{
		Name:       "ruby",
		SourceRepo: "tree-sitter/tree-sitter-ruby",
		CSymbol:    "tree_sitter_ruby",
	}

	// Install grammar at v0.1.0.
	if err := dl.Download(context.Background(), "ruby", def); err != nil {
		t.Fatalf("Download: %v", err)
	}

	// Simulate an aide upgrade: create a new loader at v0.2.0 pointing
	// to the same directory, so it picks up the v0.1.0 manifest.
	dl2 := NewDynamicLoader(dir)
	dl2.version = "v0.2.0"

	_, err := dl2.Load("ruby")
	if err == nil {
		t.Fatal("expected GrammarStaleError, got nil")
	}

	var staleErr *GrammarStaleError
	if !errors.As(err, &staleErr) {
		t.Fatalf("expected GrammarStaleError, got %T: %v", err, err)
	}
	if staleErr.Name != "ruby" {
		t.Errorf("staleErr.Name = %q; want %q", staleErr.Name, "ruby")
	}
	if staleErr.InstalledVersion != "v0.1.0" {
		t.Errorf("staleErr.InstalledVersion = %q; want %q", staleErr.InstalledVersion, "v0.1.0")
	}
	if staleErr.WantVersion != "v0.2.0" {
		t.Errorf("staleErr.WantVersion = %q; want %q", staleErr.WantVersion, "v0.2.0")
	}
}

// ---------------------------------------------------------------------------
// Staleness detection: snapshot versions always skip the staleness check.
// ---------------------------------------------------------------------------

func TestIntegrationSnapshotSkipsStalenessCheck(t *testing.T) {
	body := []byte("snapshot-grammar-lib")
	srv, _ := newTestArchiveServer(t, body)

	dir := t.TempDir()
	dl := NewDynamicLoader(dir)
	dl.baseURL = srv.URL + "/{version}/{asset}"

	def := &Pack{
		Name:       "ruby",
		SourceRepo: "tree-sitter/tree-sitter-ruby",
		CSymbol:    "tree_sitter_ruby",
	}

	// Sub-test: installed version is "snapshot", loader version is a release.
	// No staleness should be reported.
	t.Run("installed_snapshot", func(t *testing.T) {
		d := t.TempDir()
		dl := NewDynamicLoader(d)
		dl.baseURL = srv.URL + "/{version}/{asset}"
		dl.version = "snapshot"

		if err := dl.Download(context.Background(), "ruby", def); err != nil {
			t.Fatalf("Download: %v", err)
		}

		// Now set loader to a release version — staleness should be skipped
		// because installed version is "snapshot".
		dl2 := NewDynamicLoader(d)
		dl2.version = "v0.5.0"

		// Load will fail because the .so isn't a real library, but the error
		// should NOT be GrammarStaleError.
		_, err := dl2.Load("ruby")
		var staleErr *GrammarStaleError
		if errors.As(err, &staleErr) {
			t.Error("snapshot installed version should not trigger staleness")
		}
	})

	// Sub-test: installed version is a release, loader version is "snapshot".
	// No staleness should be reported.
	t.Run("loader_snapshot", func(t *testing.T) {
		d := t.TempDir()
		dl := NewDynamicLoader(d)
		dl.baseURL = srv.URL + "/{version}/{asset}"
		dl.version = "v0.3.0"

		if err := dl.Download(context.Background(), "ruby", def); err != nil {
			t.Fatalf("Download: %v", err)
		}

		// Loader running as snapshot — should skip staleness.
		dl2 := NewDynamicLoader(d)
		dl2.version = "snapshot"

		_, err := dl2.Load("ruby")
		var staleErr *GrammarStaleError
		if errors.As(err, &staleErr) {
			t.Error("snapshot loader version should not trigger staleness")
		}
	})

	// Sub-test: both versions are empty — no staleness.
	t.Run("both_empty", func(t *testing.T) {
		d := t.TempDir()
		dl := NewDynamicLoader(d)
		dl.baseURL = srv.URL + "/{version}/{asset}"
		dl.version = ""

		if err := dl.Download(context.Background(), "ruby", def); err != nil {
			t.Fatalf("Download: %v", err)
		}

		dl2 := NewDynamicLoader(d)
		dl2.version = ""

		_, err := dl2.Load("ruby")
		var staleErr *GrammarStaleError
		if errors.As(err, &staleErr) {
			t.Error("empty versions should not trigger staleness")
		}
	})
}

// ---------------------------------------------------------------------------
// Staleness: Download cleans up old library file when version changes
// ---------------------------------------------------------------------------

func TestIntegrationDownloadCleansUpOldVersionFile(t *testing.T) {
	v1Body := []byte("v1-lib-data")
	v2Body := []byte("v2-lib-data-longer")

	var currentArchive atomic.Value
	currentArchive.Store(makeGrammarArchive(t, "ruby", v1Body))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		archive, _ := currentArchive.Load().([]byte)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(archive)
	}))
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	dl := NewDynamicLoader(dir)
	dl.baseURL = srv.URL + "/{version}/{asset}"
	dl.version = "v0.1.0"

	def := &Pack{
		Name:       "ruby",
		SourceRepo: "tree-sitter/tree-sitter-ruby",
		CSymbol:    "tree_sitter_ruby",
	}

	// Download v0.1.0
	if err := dl.Download(context.Background(), "ruby", def); err != nil {
		t.Fatalf("Download v1: %v", err)
	}

	v1Path := filepath.Join(dir, LibraryFilename("ruby"))
	if _, err := os.Stat(v1Path); err != nil {
		t.Fatalf("v1 file should exist: %v", err)
	}

	// Verify v1 content.
	v1Data, _ := os.ReadFile(v1Path)
	if string(v1Data) != string(v1Body) {
		t.Error("v1 file content mismatch")
	}

	// Download v0.2.0 — library filename is the same but content differs.
	currentArchive.Store(makeGrammarArchive(t, "ruby", v2Body))
	dl.version = "v0.2.0"
	if err := dl.Download(context.Background(), "ruby", def); err != nil {
		t.Fatalf("Download v2: %v", err)
	}

	// File should exist with v2 content (same path, overwritten).
	v2Data, err := os.ReadFile(v1Path)
	if err != nil {
		t.Fatalf("v2 file should exist: %v", err)
	}
	if string(v2Data) != string(v2Body) {
		t.Error("file content should be v2 after overwrite")
	}

	// Manifest version should be updated.
	if got := dl.manifest.get("ruby").Version; got != "v0.2.0" {
		t.Errorf("manifest version = %q; want %q", got, "v0.2.0")
	}
}

// ---------------------------------------------------------------------------
// Staleness: Manifest.AideVersion gets populated after download
// ---------------------------------------------------------------------------

func TestIntegrationManifestAideVersionSet(t *testing.T) {
	body := []byte("aide-version-grammar")
	srv, _ := newTestArchiveServer(t, body)

	dir := t.TempDir()
	dl := NewDynamicLoader(dir)
	dl.baseURL = srv.URL + "/{version}/{asset}"
	dl.version = "v0.5.0"

	def := &Pack{
		Name:       "ruby",
		SourceRepo: "tree-sitter/tree-sitter-ruby",
		CSymbol:    "tree_sitter_ruby",
	}

	if err := dl.Download(context.Background(), "ruby", def); err != nil {
		t.Fatalf("Download: %v", err)
	}

	// Check AideVersion in manifest
	if got := dl.manifest.data.AideVersion; got != "v0.5.0" {
		t.Errorf("manifest AideVersion = %q; want %q", got, "v0.5.0")
	}

	// Verify persistence: create a new loader pointing to the same dir
	// and check that AideVersion was persisted to disk.
	dl2 := NewDynamicLoader(dir)
	if got := dl2.manifest.data.AideVersion; got != "v0.5.0" {
		t.Errorf("persisted AideVersion = %q; want %q", got, "v0.5.0")
	}
}

// ---------------------------------------------------------------------------
// Staleness: in-memory cache short-circuits staleness check
// ---------------------------------------------------------------------------

func TestIntegrationCachedGrammarSkipsStalenessCheck(t *testing.T) {
	body := []byte("cached-grammar-lib")
	srv, _ := newTestArchiveServer(t, body)

	dir := t.TempDir()
	dl := NewDynamicLoader(dir)
	dl.baseURL = srv.URL + "/{version}/{asset}"
	dl.version = "v0.1.0"

	def := &Pack{
		Name:       "ruby",
		SourceRepo: "tree-sitter/tree-sitter-ruby",
		CSymbol:    "tree_sitter_ruby",
	}

	// Download at v0.1.0
	if err := dl.Download(context.Background(), "ruby", def); err != nil {
		t.Fatalf("Download: %v", err)
	}

	// Manually inject a fake language into the loaded cache to simulate
	// a grammar that was already loaded (the real .so isn't loadable in tests).
	dl.mu.Lock()
	dl.loaded["ruby"] = nil // nil *Language is fine for this test
	dl.mu.Unlock()

	// Even though we change the version, the in-memory cache hit should
	// return immediately without checking staleness.
	dl.version = "v999.0.0"

	// Load should return the cached nil language without error.
	lang, err := dl.Load("ruby")
	if err != nil {
		t.Fatalf("expected cached hit, got error: %v", err)
	}
	if lang != nil {
		t.Error("expected nil (cached) language")
	}
}

// ---------------------------------------------------------------------------
// Staleness: CompositeLoader.Load auto-redownloads stale grammars
// ---------------------------------------------------------------------------

func TestIntegrationCompositeLoaderRedownloadsStale(t *testing.T) {
	v1Body := []byte("v1-composite-lib")
	v2Body := []byte("v2-composite-lib")

	var currentArchive atomic.Value
	currentArchive.Store(makeGrammarArchive(t, "ruby", v1Body))

	var reqCount atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqCount.Add(1)
		archive, _ := currentArchive.Load().([]byte)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(archive)
	}))
	t.Cleanup(srv.Close)

	dir := t.TempDir()

	// Phase 1: install grammar at v0.1.0 using a DynamicLoader directly
	dl := NewDynamicLoader(dir)
	dl.baseURL = srv.URL + "/{version}/{asset}"
	dl.version = "v0.1.0"

	def := &Pack{
		Name:       "ruby",
		SourceRepo: "tree-sitter/tree-sitter-ruby",
		CSymbol:    "tree_sitter_ruby",
	}
	if err := dl.Download(context.Background(), "ruby", def); err != nil {
		t.Fatalf("initial download: %v", err)
	}
	initialReqs := reqCount.Load()

	// Phase 2: Create a CompositeLoader at v0.2.0 with autoLoad.
	// Loading "ruby" should detect staleness and re-download.
	currentArchive.Store(makeGrammarArchive(t, "ruby", v2Body))

	cl := NewCompositeLoader(
		WithGrammarDir(dir),
		WithBaseURL(srv.URL+"/{version}/{asset}"),
		WithVersion("v0.2.0"),
		WithAutoDownload(true),
	)

	// CompositeLoader.Load will:
	// 1. Miss built-in
	// 2. DynamicLoader.Load returns GrammarStaleError
	// 3. autoLoad triggers Install → Download at v0.2.0
	// 4. DynamicLoader.Load again — now the library file exists but isn't
	//    a real .so, so openAndLoadLanguage will fail. That's fine; we're
	//    testing the download was triggered.
	_, err := cl.Load(context.Background(), "ruby")

	// We expect an error from the actual dlopen, NOT from staleness or not-found.
	if err == nil {
		// If somehow it loaded (shouldn't with fake .so), that's also acceptable.
		return
	}

	var staleErr *GrammarStaleError
	var notFoundErr *GrammarNotFoundError
	if errors.As(err, &staleErr) {
		t.Fatalf("should not get GrammarStaleError after re-download, got: %v", err)
	}
	if errors.As(err, &notFoundErr) {
		t.Fatalf("should not get GrammarNotFoundError after re-download, got: %v", err)
	}

	// Verify a second download request was made (the re-download).
	if got := reqCount.Load(); got <= initialReqs {
		t.Errorf("expected re-download request; total requests = %d, initial = %d", got, initialReqs)
	}

	// Verify the manifest was updated to v0.2.0.
	entry := cl.dynamic.manifest.get("ruby")
	if entry == nil {
		t.Fatal("manifest entry should exist after re-download")
	}
	if entry.Version != "v0.2.0" {
		t.Errorf("manifest version = %q; want %q", entry.Version, "v0.2.0")
	}

	// Verify the v2 file content is on disk.
	v2Path := filepath.Join(dir, entry.File)
	data, err := os.ReadFile(v2Path)
	if err != nil {
		t.Fatalf("reading re-downloaded file: %v", err)
	}
	if string(data) != string(v2Body) {
		t.Error("file content should be v2 after re-download")
	}

	// Verify AideVersion was set.
	if got := cl.dynamic.manifest.data.AideVersion; got != "v0.2.0" {
		t.Errorf("manifest AideVersion = %q; want %q", got, "v0.2.0")
	}
}

// ---------------------------------------------------------------------------
// Staleness: CompositeLoader.Load does NOT redownload when autoLoad is false
// ---------------------------------------------------------------------------

func TestIntegrationCompositeLoaderNoAutoRedownloadWhenDisabled(t *testing.T) {
	body := []byte("no-auto-lib")
	srv, reqCount := newTestArchiveServer(t, body)

	dir := t.TempDir()

	// Install grammar at v0.1.0.
	dl := NewDynamicLoader(dir)
	dl.baseURL = srv.URL + "/{version}/{asset}"
	dl.version = "v0.1.0"

	def := &Pack{
		Name:       "ruby",
		SourceRepo: "tree-sitter/tree-sitter-ruby",
		CSymbol:    "tree_sitter_ruby",
	}
	if err := dl.Download(context.Background(), "ruby", def); err != nil {
		t.Fatalf("Download: %v", err)
	}
	initialReqs := reqCount.Load()

	// Create CompositeLoader at v0.2.0 with autoLoad DISABLED.
	cl := NewCompositeLoader(
		WithGrammarDir(dir),
		WithBaseURL(srv.URL+"/{version}/{asset}"),
		WithVersion("v0.2.0"),
		WithAutoDownload(false),
	)

	_, err := cl.Load(context.Background(), "ruby")
	if err == nil {
		t.Fatal("expected error when autoLoad is disabled and grammar is stale")
	}

	// Should get GrammarNotFoundError (the fallback at end of Load method).
	var notFoundErr *GrammarNotFoundError
	if !errors.As(err, &notFoundErr) {
		t.Errorf("expected GrammarNotFoundError, got %T: %v", err, err)
	}

	// No additional requests should have been made.
	if got := reqCount.Load(); got != initialReqs {
		t.Errorf("expected no re-download; requests = %d, initial = %d", got, initialReqs)
	}
}
