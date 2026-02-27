package grammar

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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

// newTestServer returns a test HTTP server that serves the given body for any
// request, and a counter of how many requests were made.
func newTestServer(t *testing.T, body []byte) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	var count atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count.Add(1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
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

func sha256Sum(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// ---------------------------------------------------------------------------
// DynamicLoader.Download → manifest + file-on-disk
// ---------------------------------------------------------------------------

func TestIntegrationDownloadWritesFileAndManifest(t *testing.T) {
	body := []byte("fake-shared-library-bytes")
	srv, reqCount := newTestServer(t, body)

	dir := t.TempDir()
	dl := NewDynamicLoader(dir)
	dl.baseURL = srv.URL + "/{version}/{asset}"
	dl.version = "v0.1.0"

	ctx := context.Background()
	def := &DynamicGrammarDef{
		SourceRepo: "tree-sitter/tree-sitter-ruby",
		CSymbol:    "tree_sitter_ruby",
	}

	if err := dl.Download(ctx, "ruby", def); err != nil {
		t.Fatalf("Download: %v", err)
	}

	// Verify exactly one HTTP request was made.
	if got := reqCount.Load(); got != 1 {
		t.Errorf("expected 1 request, got %d", got)
	}

	// Verify the file exists on disk with correct content.
	expectedFilename := LibraryFilename("ruby", "v0.1.0")
	filePath := filepath.Join(dir, expectedFilename)
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("reading downloaded file: %v", err)
	}
	if string(data) != string(body) {
		t.Errorf("file content mismatch: got %d bytes, want %d", len(data), len(body))
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
	if entry.SHA256 != sha256Sum(body) {
		t.Errorf("manifest sha256 = %q; want %q", entry.SHA256, sha256Sum(body))
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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedURL.Store(r.URL.Path)
		_, _ = w.Write([]byte("lib"))
	}))
	t.Cleanup(srv.Close)

	tests := []struct {
		name       string
		dlVersion  string // DynamicLoader.version
		defVersion string // DynamicGrammarDef.LatestVersion
		wantInURL  string // substring expected in the request URL
	}{
		{
			name:       "loader version takes priority",
			dlVersion:  "v0.2.0",
			defVersion: "v0.1.0",
			wantInURL:  "v0.2.0",
		},
		{
			name:       "falls back to def version",
			dlVersion:  "",
			defVersion: "v0.3.0",
			wantInURL:  "v0.3.0",
		},
		{
			name:       "falls back to snapshot",
			dlVersion:  "",
			defVersion: "",
			wantInURL:  "snapshot",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			dl := NewDynamicLoader(dir)
			dl.baseURL = srv.URL + "/{version}/{asset}"
			dl.version = tt.dlVersion

			def := &DynamicGrammarDef{
				SourceRepo:    "test/test-grammar",
				CSymbol:       "tree_sitter_test",
				LatestVersion: tt.defVersion,
			}

			if err := dl.Download(context.Background(), "testlang", def); err != nil {
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
	srv, _ := newTestServer(t, body)

	dir := t.TempDir()
	dl := NewDynamicLoader(dir)
	dl.baseURL = srv.URL + "/{version}/{asset}"
	dl.version = "v1.0.0"

	def := &DynamicGrammarDef{
		SourceRepo: "tree-sitter/tree-sitter-php",
		CSymbol:    "tree_sitter_php",
	}

	// Download
	if err := dl.Download(context.Background(), "php", def); err != nil {
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

	// Verify file is gone from disk.
	filename := LibraryFilename("php", "v1.0.0")
	if _, err := os.Stat(filepath.Join(dir, filename)); !os.IsNotExist(err) {
		t.Error("library file should be deleted after Remove")
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
	srv, reqCount := newTestServer(t, body)

	dir := t.TempDir()
	dl := NewDynamicLoader(dir)
	dl.baseURL = srv.URL + "/{version}/{asset}"
	dl.version = "v0.5.0"

	grammars := map[string]*DynamicGrammarDef{
		"ruby": {SourceRepo: "tree-sitter/tree-sitter-ruby", CSymbol: "tree_sitter_ruby"},
		"php":  {SourceRepo: "tree-sitter/tree-sitter-php", CSymbol: "tree_sitter_php"},
		"lua":  {SourceRepo: "tree-sitter-grammars/tree-sitter-lua", CSymbol: "tree_sitter_lua"},
	}

	for name, def := range grammars {
		if err := dl.Download(context.Background(), name, def); err != nil {
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
	srv, _ := newTestServer(t, body)

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
	srv, reqCount := newTestServer(t, body)

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

	// The file should exist on disk — download succeeded even though Load failed.
	filename := LibraryFilename("bash", "v0.1.0")
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
	srv, reqCount := newTestServer(t, body)

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
	srv, _ := newTestServer(t, body)

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
		if entry.SHA256 != sha256Sum(body) {
			t.Errorf("LockFile[%s].SHA256 = %q; want %q", name, entry.SHA256, sha256Sum(body))
		}
	}
}

// ---------------------------------------------------------------------------
// Download overwrites existing grammar with newer version
// ---------------------------------------------------------------------------

func TestIntegrationDownloadOverwritesExisting(t *testing.T) {
	v1Body := []byte("version-1-library")
	v2Body := []byte("version-2-library-updated")

	var currentBody atomic.Value
	currentBody.Store(v1Body)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := currentBody.Load().([]byte)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	dl := NewDynamicLoader(dir)
	dl.baseURL = srv.URL + "/{version}/{asset}"
	dl.version = "v1.0.0"

	def := &DynamicGrammarDef{
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
	currentBody.Store(v2Body)
	if err := dl.Download(context.Background(), "ruby", def); err != nil {
		t.Fatalf("Download v2: %v", err)
	}

	entryV2 := dl.manifest.get("ruby")
	if entryV2 == nil {
		t.Fatal("manifest entry nil after v2 download")
	}

	// SHA should change because file content changed.
	if entryV2.SHA256 == sha1 {
		t.Error("SHA256 should differ after overwrite with different content")
	}
	if entryV2.SHA256 != sha256Sum(v2Body) {
		t.Errorf("SHA256 after overwrite = %q; want %q", entryV2.SHA256, sha256Sum(v2Body))
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
	srv, _ := newTestServer(t, body)

	dir := t.TempDir()

	// First loader: download a grammar.
	dl1 := NewDynamicLoader(dir)
	dl1.baseURL = srv.URL + "/{version}/{asset}"
	dl1.version = "v1.0.0"

	def := &DynamicGrammarDef{
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

	def := &DynamicGrammarDef{
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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath.Store(r.URL.Path)
		_, _ = w.Write([]byte("lib"))
	}))
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	dl := NewDynamicLoader(dir)
	// Template: /{version}/{name}/{os}/{arch}/{asset}
	dl.baseURL = srv.URL + "/{version}/{name}/{os}/{arch}/{asset}"
	dl.version = "v0.3.0"

	def := &DynamicGrammarDef{
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
