package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jmylchreest/aide/aide/pkg/config"
)

// isolateHome points HOME (and USERPROFILE on Windows) at a fresh temp dir so a
// test that triggers a config load sees an empty global ~/.aide/config/aide.json
// rather than the developer's real one. buildKoanf reads the global file on
// every Load/Resolve, so without this any such test would be non-hermetic.
func isolateHome(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", home)
	}
}

// newConfigProject returns a dbPath whose derived project root is an empty temp
// dir. The config subcommands never open the database — they read/write only
// .aide/config/aide.json — so no backend or memory.db is created here. It also
// isolates HOME so the resolved config never reads the developer's real global
// file (buildKoanf layers ~/.aide/config/aide.json on every load).
func newConfigProject(t *testing.T) (dbPath, root string) {
	t.Helper()
	isolateHome(t)
	root = t.TempDir()
	// projectRoot(dbPath) strips three path segments, so this dbPath maps back
	// to root: <root>/.aide/memory/memory.db.
	return filepath.Join(root, ".aide", "memory", "memory.db"), root
}

// readRawConfig returns the parsed aide.json for a project root, or nil when the
// file is absent.
func readRawConfig(t *testing.T, root string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(config.FilePath(root))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("reading config: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("parsing config: %v", err)
	}
	return m
}

func TestConfigSetScalarPtrBool(t *testing.T) {
	dbPath, root := newConfigProject(t)

	if err := cmdConfigSet(dbPath, []string{"share.decisions.export", "false"}); err != nil {
		t.Fatalf("set: %v", err)
	}

	// File must carry a real nested bool, not a string.
	m := readRawConfig(t, root)
	share := m["share"].(map[string]any)
	dec := share["decisions"].(map[string]any)
	if v, ok := dec["export"].(bool); !ok || v {
		t.Fatalf("export = %#v, want bool false", dec["export"])
	}

	// Reload through Load: the accessor must now report export OFF.
	cfg, err := config.Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Share.DecisionExportEnabled() {
		t.Error("after set false, DecisionExportEnabled() should be false")
	}
}

func TestConfigSetListMultiArgAndCommaIdentical(t *testing.T) {
	want := []string{"scope:global", "session:*"}

	for _, form := range [][]string{
		{"scope:global", "session:*"}, // multi-arg
		{"scope:global,session:*"},    // single comma-joined arg
	} {
		dbPath, root := newConfigProject(t)
		args := append([]string{"share.memories.export_filter.exclude"}, form...)
		if err := cmdConfigSet(dbPath, args); err != nil {
			t.Fatalf("set %v: %v", form, err)
		}

		// File must store a JSON array of strings.
		m := readRawConfig(t, root)
		exclude := dig(t, m, "share", "memories", "export_filter", "exclude")
		arr, ok := exclude.([]any)
		if !ok {
			t.Fatalf("exclude is %T, want []any", exclude)
		}
		got := make([]string, len(arr))
		for i, e := range arr {
			got[i] = e.(string)
		}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("form %v stored %v, want %v", form, got, want)
		}

		// Reload: resolved exclude must match.
		cfg, err := config.Load(root)
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		resolved := cfg.Share.MemoryExportFilter().Exclude
		if strings.Join(resolved, ",") != strings.Join(want, ",") {
			t.Errorf("form %v resolved exclude = %v, want %v", form, resolved, want)
		}
	}
}

func TestConfigSetListNoValuesClearsDefault(t *testing.T) {
	dbPath, root := newConfigProject(t)

	if err := cmdConfigSet(dbPath, []string{"share.memories.export_filter.exclude"}); err != nil {
		t.Fatalf("set: %v", err)
	}

	// File must store an explicit empty array.
	m := readRawConfig(t, root)
	exclude := dig(t, m, "share", "memories", "export_filter", "exclude")
	arr, ok := exclude.([]any)
	if !ok || len(arr) != 0 {
		t.Fatalf("exclude = %#v, want empty []any", exclude)
	}

	// Reload: the explicit [] clears the defaulted exclude list.
	cfg, err := config.Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := cfg.Share.MemoryExportFilter().Exclude; len(got) != 0 {
		t.Errorf("resolved exclude = %v, want empty", got)
	}
}

func TestConfigUnsetRevertsToDefault(t *testing.T) {
	dbPath, root := newConfigProject(t)

	if err := cmdConfigSet(dbPath, []string{"share.decisions.export", "false"}); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := cmdConfigUnset(dbPath, []string{"share.decisions.export"}); err != nil {
		t.Fatalf("unset: %v", err)
	}

	// The leaf and its now-empty parents must be pruned from the file.
	m := readRawConfig(t, root)
	if _, ok := m["share"]; ok {
		t.Errorf("share section should be pruned after unset, got %#v", m["share"])
	}

	// Reload: the accessor reverts to its true default.
	cfg, err := config.Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.Share.DecisionExportEnabled() {
		t.Error("after unset, DecisionExportEnabled() should revert to the true default")
	}
}

func TestConfigGetResolvedValue(t *testing.T) {
	dbPath, root := newConfigProject(t)
	if err := cmdConfigSet(dbPath, []string{"mode", "autopilot"}); err != nil {
		t.Fatalf("set: %v", err)
	}
	out := captureStdout(t, func() {
		if err := cmdConfigGet(dbPath, []string{"mode"}); err != nil {
			t.Fatalf("get: %v", err)
		}
	})
	if strings.TrimSpace(out) != "autopilot" {
		t.Errorf("get mode = %q, want autopilot", strings.TrimSpace(out))
	}
	_ = root
}

func TestConfigGetPtrBoolDefault(t *testing.T) {
	dbPath, _ := newConfigProject(t)
	// Nothing set: decisions.export defaults to true via the accessor, even
	// though it never appears in any source.
	out := captureStdout(t, func() {
		if err := cmdConfigGet(dbPath, []string{"share.decisions.export"}); err != nil {
			t.Fatalf("get: %v", err)
		}
	})
	if strings.TrimSpace(out) != "true" {
		t.Errorf("get share.decisions.export = %q, want true", strings.TrimSpace(out))
	}
}

func TestConfigShowAllHumanAndJSON(t *testing.T) {
	dbPath, root := newConfigProject(t)
	if err := cmdConfigSet(dbPath, []string{"share.decisions.export", "false"}); err != nil {
		t.Fatalf("set: %v", err)
	}

	// Human --all: representative keys with values and a source column.
	human := captureStdout(t, func() {
		if err := cmdConfigShow(dbPath, []string{"--all"}); err != nil {
			t.Fatalf("show --all: %v", err)
		}
	})
	for _, want := range []string{"share.decisions.export", "cleanup.observe_max_age", "index_workers", "mode"} {
		if !strings.Contains(human, want) {
			t.Errorf("show --all output missing key %q\n%s", want, human)
		}
	}
	if !strings.Contains(human, "file") {
		t.Error("show --all should mark the set key's source as file")
	}

	// JSON --all parses into the stable view shape.
	jsonOut := captureStdout(t, func() {
		if err := cmdConfigShow(dbPath, []string{"--all", "--json"}); err != nil {
			t.Fatalf("show --all --json: %v", err)
		}
	})
	var views []configKeyView
	if err := json.Unmarshal([]byte(jsonOut), &views); err != nil {
		t.Fatalf("--all --json not valid JSON: %v\n%s", err, jsonOut)
	}
	found := false
	for _, v := range views {
		if v.Key == "share.decisions.export" {
			found = true
			if v.Source != "file" {
				t.Errorf("share.decisions.export source = %q, want file", v.Source)
			}
			if b, ok := v.Value.(bool); !ok || b {
				t.Errorf("share.decisions.export value = %#v, want false", v.Value)
			}
		}
	}
	if !found {
		t.Error("--all --json missing share.decisions.export")
	}
	_ = root
}

func TestConfigShowJSONMarshalsResolvedConfig(t *testing.T) {
	dbPath, _ := newConfigProject(t)
	out := captureStdout(t, func() {
		if err := cmdConfigShow(dbPath, []string{"--json"}); err != nil {
			t.Fatalf("show --json: %v", err)
		}
	})
	var cfg config.Config
	if err := json.Unmarshal([]byte(out), &cfg); err != nil {
		t.Fatalf("show --json not valid Config JSON: %v\n%s", err, out)
	}
}

func TestConfigPathPrintsFile(t *testing.T) {
	dbPath, root := newConfigProject(t)
	out := captureStdout(t, func() {
		if err := cmdConfigPath(dbPath, nil); err != nil {
			t.Fatalf("path: %v", err)
		}
	})
	want := config.FilePath(root)
	if strings.TrimSpace(out) != want {
		t.Errorf("path = %q, want %q", strings.TrimSpace(out), want)
	}
}

func TestConfigUnknownKeyErrors(t *testing.T) {
	dbPath, _ := newConfigProject(t)
	for _, fn := range []func() error{
		func() error { return cmdConfigSet(dbPath, []string{"nope.key", "x"}) },
		func() error { return cmdConfigGet(dbPath, []string{"nope.key"}) },
		func() error { return cmdConfigUnset(dbPath, []string{"nope.key"}) },
	} {
		if err := fn(); err == nil {
			t.Error("expected error for unknown key")
		} else if !strings.Contains(err.Error(), "unknown config key") {
			t.Errorf("error = %v, want unknown config key message", err)
		}
	}
}

func TestConfigSetBadBoolErrors(t *testing.T) {
	dbPath, _ := newConfigProject(t)
	err := cmdConfigSet(dbPath, []string{"share.decisions.export", "maybe"})
	if err == nil {
		t.Fatal("expected error for non-bool value")
	}
	if !strings.Contains(err.Error(), "invalid bool") {
		t.Errorf("error = %v, want invalid bool message", err)
	}
}

func TestConfigSetCreatesDirAndFile(t *testing.T) {
	dbPath, root := newConfigProject(t)
	// Confirm the config file does not exist yet.
	if _, err := os.Stat(config.FilePath(root)); !os.IsNotExist(err) {
		t.Fatalf("config file should not exist yet, stat err = %v", err)
	}
	if err := cmdConfigSet(dbPath, []string{"mode", "plan"}); err != nil {
		t.Fatalf("set: %v", err)
	}
	if _, err := os.Stat(config.FilePath(root)); err != nil {
		t.Fatalf("config file should exist after set: %v", err)
	}
	if m := readRawConfig(t, root); m["mode"] != "plan" {
		t.Errorf("mode = %#v, want plan", m["mode"])
	}
}

func TestConfigSetEnvOverrideNote(t *testing.T) {
	dbPath, _ := newConfigProject(t)
	t.Setenv("AIDE_MODE", "autopilot")
	out := captureStdout(t, func() {
		if err := cmdConfigSet(dbPath, []string{"mode", "plan"}); err != nil {
			t.Fatalf("set: %v", err)
		}
	})
	if !strings.Contains(out, "overridden by env AIDE_MODE") {
		t.Errorf("set output should note the env override, got %q", out)
	}
}

// readGlobalConfig returns the parsed global ~/.aide/config/aide.json (under the
// isolated HOME), or nil when it is absent.
func readGlobalConfig(t *testing.T) map[string]any {
	t.Helper()
	path := config.GlobalFilePath()
	if path == "" {
		t.Fatal("GlobalFilePath() empty; isolateHome must run first")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("reading global config: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("parsing global config: %v", err)
	}
	return m
}

func TestConfigSetGlobalWritesGlobalNotProject(t *testing.T) {
	dbPath, root := newConfigProject(t)

	if err := cmdConfigSet(dbPath, []string{"--global", "mode", "plan"}); err != nil {
		t.Fatalf("set --global: %v", err)
	}

	// The global file carries the value...
	gm := readGlobalConfig(t)
	if gm == nil || gm["mode"] != "plan" {
		t.Fatalf("global mode = %#v, want plan", gm["mode"])
	}
	// ...and the project file is untouched (never created).
	if _, err := os.Stat(config.FilePath(root)); !os.IsNotExist(err) {
		t.Errorf("project config should not exist after set --global, stat err = %v", err)
	}

	// Resolved Load reflects the global value (no project override, no env).
	cfg, err := config.Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Mode != "plan" {
		t.Errorf("resolved mode = %q, want plan from global", cfg.Mode)
	}
}

func TestConfigUnsetGlobalRemovesFromGlobal(t *testing.T) {
	dbPath, _ := newConfigProject(t)

	if err := cmdConfigSet(dbPath, []string{"--global", "mode", "plan"}); err != nil {
		t.Fatalf("set --global: %v", err)
	}
	if err := cmdConfigUnset(dbPath, []string{"--global", "mode"}); err != nil {
		t.Fatalf("unset --global: %v", err)
	}

	gm := readGlobalConfig(t)
	if gm != nil {
		if _, ok := gm["mode"]; ok {
			t.Errorf("mode should be removed from global file, got %#v", gm)
		}
	}
}

func TestConfigPathGlobalPrintsGlobal(t *testing.T) {
	dbPath, root := newConfigProject(t)

	// Without --global: project path.
	projOut := captureStdout(t, func() {
		if err := cmdConfigPath(dbPath, nil); err != nil {
			t.Fatalf("path: %v", err)
		}
	})
	if got, want := strings.TrimSpace(projOut), config.FilePath(root); got != want {
		t.Errorf("path = %q, want project path %q", got, want)
	}

	// With --global: global path.
	globOut := captureStdout(t, func() {
		if err := cmdConfigPath(dbPath, []string{"--global"}); err != nil {
			t.Fatalf("path --global: %v", err)
		}
	})
	if got, want := strings.TrimSpace(globOut), config.GlobalFilePath(); got != want {
		t.Errorf("path --global = %q, want global path %q", got, want)
	}
}

// TestConfigShowAllSourceLayers exercises the four source labels in their
// resolution-precedence order: env > project file > global file > default.
func TestConfigShowAllSourceLayers(t *testing.T) {
	dbPath, root := newConfigProject(t)

	// global-only key: cleanup.interval set in the global file.
	if err := cmdConfigSet(dbPath, []string{"--global", "cleanup.interval", "30m"}); err != nil {
		t.Fatalf("set --global cleanup.interval: %v", err)
	}
	// project-file key: mode set in the project file.
	if err := cmdConfigSet(dbPath, []string{"mode", "plan"}); err != nil {
		t.Fatalf("set mode: %v", err)
	}
	// env key: cleanup.enabled overridden by env.
	t.Setenv("AIDE_CLEANUP_ENABLED", "0")

	views, err := buildConfigKeyViews(root, mustLoad(t, root))
	if err != nil {
		t.Fatalf("buildConfigKeyViews: %v", err)
	}
	src := map[string]string{}
	for _, v := range views {
		src[v.Key] = v.Source
	}

	if got := src["cleanup.interval"]; got != "global" {
		t.Errorf("cleanup.interval source = %q, want global", got)
	}
	if got := src["mode"]; got != "file" {
		t.Errorf("mode source = %q, want file", got)
	}
	if got := src["cleanup.enabled"]; !strings.HasPrefix(got, "env") {
		t.Errorf("cleanup.enabled source = %q, want env (...)", got)
	}
	// A key set in no source at all reports default.
	if got := src["index_workers"]; got != "default" {
		t.Errorf("index_workers source = %q, want default", got)
	}
}

// mustLoad loads the resolved config for root or fails the test.
func mustLoad(t *testing.T, root string) *config.Config {
	t.Helper()
	cfg, err := config.Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return cfg
}

// dig walks a parsed JSON map down a chain of map keys, failing the test if any
// intermediate node is missing or not a map.
func dig(t *testing.T, m map[string]any, path ...string) any {
	t.Helper()
	var cur any = m
	for i, key := range path {
		mp, ok := cur.(map[string]any)
		if !ok {
			t.Fatalf("dig: %q is not a map (at %v)", key, path[:i])
		}
		cur, ok = mp[key]
		if !ok {
			t.Fatalf("dig: key %q missing (at %v)", key, path[:i+1])
		}
	}
	return cur
}
