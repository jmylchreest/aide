package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/jmylchreest/aide/aide/internal/version"
)

// =============================================================================
// projectRoot / grammarDir
// =============================================================================

func TestProjectRoot(t *testing.T) {
	// dbPath is <root>/.aide/memory/memory.db — three Dir() calls to reach <root>.
	tests := []struct {
		name   string
		dbPath string
		want   string
	}{
		{
			name:   "standard layout",
			dbPath: "/home/user/myproject/.aide/memory/memory.db",
			want:   "/home/user/myproject",
		},
		{
			name:   "nested project",
			dbPath: "/a/b/c/project/.aide/memory/memory.db",
			want:   "/a/b/c/project",
		},
		{
			name:   "root-level project",
			dbPath: "/project/.aide/memory/memory.db",
			want:   "/project",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := projectRoot(tt.dbPath)
			if got != tt.want {
				t.Errorf("projectRoot(%q) = %q, want %q", tt.dbPath, got, tt.want)
			}
		})
	}
}

func TestGrammarDir(t *testing.T) {
	dbPath := "/home/user/myproject/.aide/memory/memory.db"
	want := "/home/user/myproject/.aide/grammars"
	got := grammarDir(dbPath)
	if got != want {
		t.Errorf("grammarDir(%q) = %q, want %q", dbPath, got, want)
	}
}

// =============================================================================
// loadAideConfig
// =============================================================================

func TestLoadAideConfigValid(t *testing.T) {
	tmpDir := t.TempDir()

	configDir := filepath.Join(tmpDir, ".aide", "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}

	cfg := aideJSON{
		Grammars: grammarsConfig{
			URL: "https://example.com/{version}/{asset}",
		},
	}
	data, _ := json.Marshal(cfg)
	if err := os.WriteFile(filepath.Join(configDir, "aide.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	got := loadAideConfig(tmpDir)
	if got.Grammars.URL != "https://example.com/{version}/{asset}" {
		t.Errorf("URL = %q, want %q", got.Grammars.URL, "https://example.com/{version}/{asset}")
	}
}

func TestLoadAideConfigAutoDownloadExplicit(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".aide", "config")
	os.MkdirAll(configDir, 0o755)

	val := false
	cfg := aideJSON{
		Grammars: grammarsConfig{
			AutoDownload: &val,
		},
	}
	data, _ := json.Marshal(cfg)
	os.WriteFile(filepath.Join(configDir, "aide.json"), data, 0o644)

	got := loadAideConfig(tmpDir)
	if got.Grammars.AutoDownload == nil {
		t.Fatal("AutoDownload should not be nil")
	}
	if *got.Grammars.AutoDownload != false {
		t.Errorf("AutoDownload = %v, want false", *got.Grammars.AutoDownload)
	}
}

func TestLoadAideConfigMissing(t *testing.T) {
	tmpDir := t.TempDir()
	got := loadAideConfig(tmpDir)
	// Should return zero-valued config without error.
	if got.Grammars.URL != "" {
		t.Errorf("URL should be empty, got %q", got.Grammars.URL)
	}
	if got.Grammars.AutoDownload != nil {
		t.Errorf("AutoDownload should be nil, got %v", *got.Grammars.AutoDownload)
	}
}

func TestLoadAideConfigInvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".aide", "config")
	os.MkdirAll(configDir, 0o755)
	os.WriteFile(filepath.Join(configDir, "aide.json"), []byte("not json"), 0o644)

	got := loadAideConfig(tmpDir)
	// Should return zero-valued config on parse error.
	if got.Grammars.URL != "" {
		t.Errorf("URL should be empty on invalid JSON, got %q", got.Grammars.URL)
	}
}

// =============================================================================
// loadGrammarsConfig — env var overrides
// =============================================================================

func TestLoadGrammarsConfigEnvURL(t *testing.T) {
	tmpDir := t.TempDir()

	// Set env var to override URL.
	t.Setenv("AIDE_GRAMMAR_URL", "https://custom.example.com/{version}/{asset}")

	got := loadGrammarsConfig(tmpDir)
	if got.URL != "https://custom.example.com/{version}/{asset}" {
		t.Errorf("URL = %q, want env override", got.URL)
	}
}

func TestLoadGrammarsConfigEnvURLOverridesFile(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".aide", "config")
	os.MkdirAll(configDir, 0o755)

	cfg := aideJSON{Grammars: grammarsConfig{URL: "https://from-file.example.com/{asset}"}}
	data, _ := json.Marshal(cfg)
	os.WriteFile(filepath.Join(configDir, "aide.json"), data, 0o644)

	t.Setenv("AIDE_GRAMMAR_URL", "https://from-env.example.com/{asset}")

	got := loadGrammarsConfig(tmpDir)
	if got.URL != "https://from-env.example.com/{asset}" {
		t.Errorf("URL = %q, want env to override file", got.URL)
	}
}

func TestLoadGrammarsConfigEnvAutoDownloadDisable(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		envVal string
		want   bool
	}{
		{"0", false},
		{"false", false},
		{"FALSE", false},
		{"False", false},
		{"1", true},
		{"true", true},
		{"yes", true},
	}

	for _, tt := range tests {
		t.Run(tt.envVal, func(t *testing.T) {
			t.Setenv("AIDE_GRAMMAR_AUTO_DOWNLOAD", tt.envVal)

			got := loadGrammarsConfig(tmpDir)
			if got.AutoDownload == nil {
				t.Fatal("AutoDownload should not be nil when env is set")
			}
			if *got.AutoDownload != tt.want {
				t.Errorf("AIDE_GRAMMAR_AUTO_DOWNLOAD=%q: got %v, want %v", tt.envVal, *got.AutoDownload, tt.want)
			}
		})
	}
}

func TestLoadGrammarsConfigNoEnv(t *testing.T) {
	tmpDir := t.TempDir()
	// Ensure env vars are unset.
	t.Setenv("AIDE_GRAMMAR_URL", "")
	t.Setenv("AIDE_GRAMMAR_AUTO_DOWNLOAD", "")

	got := loadGrammarsConfig(tmpDir)
	// With empty env vars: URL empty string should not override (getenv returns ""),
	// but our code checks envURL != "" so it should not set it.
	// However we set it to "" explicitly — Setenv("", "") still means os.Getenv returns "".
	if got.URL != "" {
		t.Errorf("URL should be empty with no config, got %q", got.URL)
	}
	// AutoDownload: empty string means getenv returns "", which is != "",
	// BUT we set it to "". The code checks envAuto != "". os.Getenv of "" key returns ""?
	// Actually t.Setenv("AIDE_GRAMMAR_AUTO_DOWNLOAD", "") sets the value to empty string.
	// os.Getenv returns "" which fails the != "" check. So AutoDownload stays nil.
	if got.AutoDownload != nil {
		t.Errorf("AutoDownload should be nil with no config and empty env, got %v", *got.AutoDownload)
	}
}

// =============================================================================
// newGrammarLoader / newGrammarLoaderNoAuto
// =============================================================================

func TestNewGrammarLoader(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, ".aide", "memory", "memory.db")

	// Ensure env vars don't interfere.
	t.Setenv("AIDE_GRAMMAR_URL", "")
	t.Setenv("AIDE_GRAMMAR_AUTO_DOWNLOAD", "")

	loader := newGrammarLoader(dbPath)
	if loader == nil {
		t.Fatal("newGrammarLoader returned nil")
	}
	// Should have builtins available.
	avail := loader.Available()
	if len(avail) == 0 {
		t.Error("expected available grammars from loader")
	}
}

func TestNewGrammarLoaderWithConfig(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, ".aide", "memory", "memory.db")

	// Create config file.
	configDir := filepath.Join(tmpDir, ".aide", "config")
	os.MkdirAll(configDir, 0o755)

	val := false
	cfg := aideJSON{
		Grammars: grammarsConfig{
			URL:          "https://custom.example.com/{version}/{asset}",
			AutoDownload: &val,
		},
	}
	data, _ := json.Marshal(cfg)
	os.WriteFile(filepath.Join(configDir, "aide.json"), data, 0o644)

	t.Setenv("AIDE_GRAMMAR_URL", "")
	t.Setenv("AIDE_GRAMMAR_AUTO_DOWNLOAD", "")

	loader := newGrammarLoader(dbPath)
	if loader == nil {
		t.Fatal("newGrammarLoader returned nil")
	}
}

func TestNewGrammarLoaderNoAuto(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, ".aide", "memory", "memory.db")

	t.Setenv("AIDE_GRAMMAR_URL", "")
	t.Setenv("AIDE_GRAMMAR_AUTO_DOWNLOAD", "")

	loader := newGrammarLoaderNoAuto(dbPath)
	if loader == nil {
		t.Fatal("newGrammarLoaderNoAuto returned nil")
	}
}

// =============================================================================
// grammarVersion
// =============================================================================

func TestGrammarVersionRelease(t *testing.T) {
	orig := version.Version
	defer func() { version.Version = orig }()

	version.Version = "0.0.39"
	got := grammarVersion()
	if got != "v0.0.39" {
		t.Errorf("grammarVersion() = %q; want %q", got, "v0.0.39")
	}
}

func TestGrammarVersionSnapshot(t *testing.T) {
	orig := version.Version
	defer func() { version.Version = orig }()

	version.Version = "0.0.40-dev.10+bf0bfe2"
	got := grammarVersion()
	if got != "snapshot" {
		t.Errorf("grammarVersion() = %q; want %q", got, "snapshot")
	}
}

func TestGrammarVersionDev(t *testing.T) {
	orig := version.Version
	defer func() { version.Version = orig }()

	version.Version = "0.0.0"
	got := grammarVersion()
	if got != "snapshot" {
		t.Errorf("grammarVersion() = %q; want %q", got, "snapshot")
	}
}

// =============================================================================
// Pre-existing helpers: truncate, parseFlag, hasFlag, titleCase
// =============================================================================

func TestTruncate(t *testing.T) {
	tests := []struct {
		s    string
		n    int
		want string
	}{
		{"hello world", 20, "hello world"},
		{"hello world", 11, "hello world"},
		{"hello world", 8, "hello..."},
		{"hello world", 5, "he..."},
		{"hi", 3, "hi"}, // n < 4, returns s unchanged
		{"hi", 2, "hi"}, // n < 4, returns s unchanged
		{"abcd", 4, "abcd"},
	}
	for _, tt := range tests {
		got := truncate(tt.s, tt.n)
		if got != tt.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", tt.s, tt.n, got, tt.want)
		}
	}
}

func TestParseFlag(t *testing.T) {
	tests := []struct {
		name   string
		args   []string
		prefix string
		want   string
	}{
		{
			name:   "exact match",
			args:   []string{"--analyzer=complexity"},
			prefix: "--analyzer=",
			want:   "complexity",
		},
		{
			name:   "British alias",
			args:   []string{"--analyser=coupling"},
			prefix: "--analyzer=",
			want:   "coupling",
		},
		{
			name:   "not found",
			args:   []string{"--other=value"},
			prefix: "--analyzer=",
			want:   "",
		},
		{
			name:   "multiple args",
			args:   []string{"--foo=bar", "--analyzer=secrets", "--baz=qux"},
			prefix: "--analyzer=",
			want:   "secrets",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseFlag(tt.args, tt.prefix)
			if got != tt.want {
				t.Errorf("parseFlag(%v, %q) = %q, want %q", tt.args, tt.prefix, got, tt.want)
			}
		})
	}
}

func TestHasFlag(t *testing.T) {
	args := []string{"--verbose", "--no-lock", "install"}
	if !hasFlag(args, "--verbose") {
		t.Error("expected --verbose to be found")
	}
	if !hasFlag(args, "--no-lock") {
		t.Error("expected --no-lock to be found")
	}
	if hasFlag(args, "--missing") {
		t.Error("expected --missing to not be found")
	}
}

func TestTitleCase(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello world", "Hello World"},
		{"", ""},
		{"single", "Single"},
		{"already Title", "Already Title"},
		{"a b c", "A B C"},
	}
	for _, tt := range tests {
		got := titleCase(tt.input)
		if got != tt.want {
			t.Errorf("titleCase(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// =============================================================================
// loadFindingsConfig
// =============================================================================

func TestLoadFindingsConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".aide", "config")
	os.MkdirAll(configDir, 0o755)

	cfg := aideJSON{
		Findings: findingsConfig{},
	}
	cfg.Findings.Complexity.Threshold = 15
	cfg.Findings.Coupling.FanOut = 20
	cfg.Findings.Clones.MinLines = 10
	data, _ := json.Marshal(cfg)
	os.WriteFile(filepath.Join(configDir, "aide.json"), data, 0o644)

	got := loadFindingsConfig(tmpDir)
	if got.Complexity.Threshold != 15 {
		t.Errorf("Complexity.Threshold = %d, want 15", got.Complexity.Threshold)
	}
	if got.Coupling.FanOut != 20 {
		t.Errorf("Coupling.FanOut = %d, want 20", got.Coupling.FanOut)
	}
	if got.Clones.MinLines != 10 {
		t.Errorf("Clones.MinLines = %d, want 10", got.Clones.MinLines)
	}
}

func TestLoadFindingsConfigMissing(t *testing.T) {
	tmpDir := t.TempDir()
	got := loadFindingsConfig(tmpDir)
	// All zero values.
	if got.Complexity.Threshold != 0 {
		t.Errorf("expected zero threshold, got %d", got.Complexity.Threshold)
	}
}
