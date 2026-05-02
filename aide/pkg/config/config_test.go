package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnvToKey(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"AIDE_FORCE_INIT", "force_init"},
		{"AIDE_PROJECT_ROOT", "project_root"},
		{"AIDE_MODE", "mode"},
		{"AIDE_INDEX_NON_VCS", "index_non_vcs"},
		{"AIDE_CODE_WATCH", "code.watch"},
		{"AIDE_CODE_WATCH_PATHS", "code.watch_paths"},
		{"AIDE_CODE_WATCH_DELAY", "code.watch_delay"},
		{"AIDE_CODE_STORE_DISABLE", "code.store_disable"},
		{"AIDE_CODE_STORE_SYNC", "code.store_sync"},
		{"AIDE_PPROF_ENABLE", "pprof.enable"},
		{"AIDE_PPROF_ADDR", "pprof.addr"},
		{"AIDE_GRAMMAR_URL", "grammar.url"},
		{"AIDE_GRAMMAR_AUTO_DOWNLOAD", "grammar.auto_download"},
		{"AIDE_SHARE_AUTO_IMPORT", "share.auto_import"},
		{"AIDE_MEMORY_SCORING_DISABLED", "memory.scoring_disabled"},
		{"AIDE_MEMORY_DECAY_DISABLED", "memory.decay_disabled"},
		{"NOT_AIDE_FOO", ""},
	}
	for _, c := range cases {
		got := envToKey(c.in)
		if got != c.want {
			t.Errorf("envToKey(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestLoad_FromEnv(t *testing.T) {
	t.Setenv("AIDE_FORCE_INIT", "1")
	t.Setenv("AIDE_PROJECT_ROOT", "/some/path")
	t.Setenv("AIDE_MODE", "autopilot")
	t.Setenv("AIDE_CODE_WATCH", "1")
	t.Setenv("AIDE_CODE_WATCH_PATHS", "src,pkg")
	t.Setenv("AIDE_CODE_WATCH_DELAY", "500ms")
	t.Setenv("AIDE_PPROF_ENABLE", "1")
	t.Setenv("AIDE_PPROF_ADDR", ":6060")
	t.Setenv("AIDE_INDEX_NON_VCS", "1")
	t.Setenv("AIDE_GRAMMAR_AUTO_DOWNLOAD", "yes")
	t.Setenv("AIDE_MEMORY_SCORING_ENABLED", "0")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.ForceInit {
		t.Error("ForceInit not set")
	}
	if cfg.ProjectRoot != "/some/path" {
		t.Errorf("ProjectRoot = %q", cfg.ProjectRoot)
	}
	if cfg.Mode != "autopilot" {
		t.Errorf("Mode = %q", cfg.Mode)
	}
	if !cfg.Code.Watch {
		t.Error("Code.Watch not set")
	}
	if cfg.Code.WatchPaths != "src,pkg" {
		t.Errorf("Code.WatchPaths = %q", cfg.Code.WatchPaths)
	}
	if got := cfg.Code.WatchPathList(); len(got) != 2 || got[0] != "src" || got[1] != "pkg" {
		t.Errorf("WatchPathList() = %v", got)
	}
	if got := cfg.Code.WatchDelayDuration(0); got.String() != "500ms" {
		t.Errorf("WatchDelayDuration = %v", got)
	}
	if !cfg.Pprof.Enable || cfg.Pprof.Addr != ":6060" {
		t.Errorf("Pprof = %+v", cfg.Pprof)
	}
	if !cfg.IndexNonVCS {
		t.Error("IndexNonVCS not set")
	}
	if cfg.Grammar.AutoDownload != "yes" {
		t.Errorf("Grammar.AutoDownload = %q", cfg.Grammar.AutoDownload)
	}
	if cfg.Memory.ScoringEnabled {
		t.Error("AIDE_MEMORY_SCORING_ENABLED=0 should produce ScoringEnabled=false")
	}
}

func TestLoad_LegacyDisabledEnvVarsInvert(t *testing.T) {
	t.Setenv("AIDE_MEMORY_SCORING_DISABLED", "1")
	t.Setenv("AIDE_MEMORY_DECAY_DISABLED", "1")
	t.Setenv("AIDE_CODE_STORE_DISABLE", "1")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Memory.ScoringEnabled {
		t.Error("AIDE_MEMORY_SCORING_DISABLED=1 should leave ScoringEnabled=false")
	}
	if cfg.Memory.DecayEnabled {
		t.Error("AIDE_MEMORY_DECAY_DISABLED=1 should leave DecayEnabled=false")
	}
	if cfg.Code.StoreEnabled {
		t.Error("AIDE_CODE_STORE_DISABLE=1 should leave StoreEnabled=false")
	}
}

func TestLoad_DefaultsAreOn(t *testing.T) {
	// Clear every disable/enable variant so defaults can show through.
	for _, name := range []string{
		"AIDE_MEMORY_SCORING_ENABLED", "AIDE_MEMORY_SCORING_DISABLED",
		"AIDE_MEMORY_DECAY_ENABLED", "AIDE_MEMORY_DECAY_DISABLED",
		"AIDE_CODE_STORE_ENABLED", "AIDE_CODE_STORE_DISABLE",
	} {
		t.Setenv(name, "")
	}

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.Memory.ScoringEnabled {
		t.Error("Memory.ScoringEnabled should default to true")
	}
	if !cfg.Memory.DecayEnabled {
		t.Error("Memory.DecayEnabled should default to true")
	}
	if !cfg.Code.StoreEnabled {
		t.Error("Code.StoreEnabled should default to true")
	}
}

func TestLoad_FileOverriddenByEnv(t *testing.T) {
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, ".aide", "config")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	contents := []byte(`{"mode":"plan","code":{"watch":false}}`)
	if err := os.WriteFile(filepath.Join(cfgDir, "aide.json"), contents, 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("AIDE_MODE", "autopilot")
	t.Setenv("AIDE_CODE_WATCH", "1")

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Mode != "autopilot" {
		t.Errorf("Mode should be overridden by env, got %q", cfg.Mode)
	}
	if !cfg.Code.Watch {
		t.Error("Code.Watch should be overridden by env")
	}
}

func TestGet_BeforeLoad(t *testing.T) {
	Set(nil)
	c := Get()
	if c == nil {
		t.Fatal("Get returned nil; expected zero-valued Config")
	}
	if c.Mode != "" {
		t.Errorf("zero-value Config should have empty Mode, got %q", c.Mode)
	}
}
