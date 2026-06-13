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

func TestShareConfigDefaults(t *testing.T) {
	// A zero-value ShareConfig (every *bool nil, every filter empty) must
	// resolve to the documented type defaults.
	var sc ShareConfig

	if !sc.DecisionExportEnabled() {
		t.Error("decisions should default to export ON")
	}
	if !sc.DecisionImportEnabled() {
		t.Error("decisions should default to import ON")
	}
	if sc.MemoryExportEnabled() {
		t.Error("memories should default to export OFF")
	}
	if sc.MemoryImportEnabled() {
		t.Error("memories should default to import OFF")
	}

	if df := sc.DecisionExportFilter(); len(df.Include) != 1 || df.Include[0] != "*" || len(df.Exclude) != 0 {
		t.Errorf("decision export filter default = %+v, want include [*] exclude []", df)
	}
	mf := sc.MemoryExportFilter()
	if len(mf.Include) != 1 || mf.Include[0] != "*" {
		t.Errorf("memory export filter include default = %v, want [*]", mf.Include)
	}
	wantExclude := []string{"scope:global", "session:*"}
	if len(mf.Exclude) != len(wantExclude) {
		t.Fatalf("memory export filter exclude default = %v, want %v", mf.Exclude, wantExclude)
	}
	for i := range wantExclude {
		if mf.Exclude[i] != wantExclude[i] {
			t.Errorf("memory export exclude[%d] = %q, want %q", i, mf.Exclude[i], wantExclude[i])
		}
	}
	if mif := sc.MemoryImportFilter(); len(mif.Exclude) != len(wantExclude) {
		t.Errorf("memory import filter exclude default = %v, want %v", mif.Exclude, wantExclude)
	}
}

func TestShareConfigExplicitFalseOverridesDefault(t *testing.T) {
	no := false
	yes := true
	sc := ShareConfig{
		Decisions: ShareTypePolicy{Export: &no},  // override the true default
		Memories:  ShareTypePolicy{Import: &yes}, // override the false default
	}
	if sc.DecisionExportEnabled() {
		t.Error("explicit export=false should override the decision default of true")
	}
	if !sc.DecisionImportEnabled() {
		t.Error("decision import left unset should stay at its true default")
	}
	if !sc.MemoryImportEnabled() {
		t.Error("explicit memory import=true should override the default of false")
	}
	if sc.MemoryExportEnabled() {
		t.Error("memory export left unset should stay at its false default")
	}
}

func TestShareFilterEmptyIncludeMatchesAll(t *testing.T) {
	// An explicitly empty include must resolve to ["*"] (match all), while a
	// user-set include is preserved verbatim.
	sc := ShareConfig{
		Decisions: ShareTypePolicy{
			ExportFilter: ShareFilter{Include: nil, Exclude: []string{"decided_by:blueprint:*"}},
		},
	}
	df := sc.DecisionExportFilter()
	if len(df.Include) != 1 || df.Include[0] != "*" {
		t.Errorf("empty include should resolve to [*], got %v", df.Include)
	}
	if len(df.Exclude) != 1 || df.Exclude[0] != "decided_by:blueprint:*" {
		t.Errorf("explicit exclude should be preserved, got %v", df.Exclude)
	}

	custom := ShareConfig{
		Memories: ShareTypePolicy{ImportFilter: ShareFilter{Include: []string{"team:*"}}},
	}
	if got := custom.MemoryImportFilter().Include; len(got) != 1 || got[0] != "team:*" {
		t.Errorf("explicit include should be preserved, got %v", got)
	}
}

func TestShareConfigUnmarshalsBoolPointers(t *testing.T) {
	// koanf must leave an absent *bool as nil (so the type default applies) and
	// populate an explicit one. A file sets decisions.export=false; memories is
	// untouched.
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, ".aide", "config")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	contents := []byte(`{"share":{"decisions":{"export":false},"memories":{"export":true}}}`)
	if err := os.WriteFile(filepath.Join(cfgDir, "aide.json"), contents, 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Share.Decisions.Export == nil {
		t.Fatal("decisions.export should unmarshal to a non-nil *bool")
	}
	if *cfg.Share.Decisions.Export {
		t.Error("decisions.export=false should unmarshal to false")
	}
	if cfg.Share.Decisions.Import != nil {
		t.Error("absent decisions.import should stay nil so the default applies")
	}
	if !cfg.Share.DecisionImportEnabled() {
		t.Error("absent decisions.import should resolve to the true default")
	}
	if cfg.Share.Decisions.Export != nil && cfg.Share.DecisionExportEnabled() {
		t.Error("decisions.export=false should make DecisionExportEnabled() false")
	}
	if cfg.Share.Memories.Export == nil || !*cfg.Share.Memories.Export {
		t.Error("memories.export=true should unmarshal to true")
	}
	if !cfg.Share.MemoryExportEnabled() {
		t.Error("memories.export=true should make MemoryExportEnabled() true")
	}
}

func TestShareConfigExplicitEmptyExcludeClearsDefault(t *testing.T) {
	// An explicit JSON "[]" must clear the default memory exclusions (so a team
	// can opt to share scope:global / session:* too), while an absent key still
	// inherits the default. This relies on koanf/mapstructure unmarshalling an
	// absent slice to nil but an explicit "[]" to a non-nil empty slice; guard
	// that contract here so a parser change can't silently break the override.
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, ".aide", "config")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	contents := []byte(`{"share":{"memories":{"export":true,"export_filter":{"exclude":[]}}}}`)
	if err := os.WriteFile(filepath.Join(cfgDir, "aide.json"), contents, 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if ex := cfg.Share.MemoryExportFilter().Exclude; len(ex) != 0 {
		t.Errorf("explicit exclude:[] should clear the default, got %v", ex)
	}
	// Import filter was left absent, so it must still inherit the default.
	if ex := cfg.Share.MemoryImportFilter().Exclude; len(ex) != 2 {
		t.Errorf("absent import exclude should inherit the 2-entry default, got %v", ex)
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
