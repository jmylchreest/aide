// Package config centralizes runtime configuration for the aide CLI and
// daemon. Values flow from (in increasing precedence): defaults → optional
// .aide/config/aide.json → AIDE_* environment variables → top-level CLI
// overrides written back into the env (e.g. --project sets AIDE_PROJECT_ROOT
// before Load is called).
//
// Callers ask for the singleton via Get(); the bootstrap path in main.go is
// responsible for invoking Load() exactly once at startup.
package config

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/knadh/koanf/parsers/json"
	"github.com/knadh/koanf/providers/confmap"
	"github.com/knadh/koanf/providers/env/v2"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

// EnvPrefix is the prefix every aide-recognised environment variable carries.
const EnvPrefix = "AIDE_"

// Config holds every tunable read from env or file. Fields are organised by
// subsystem but kept in a single struct because koanf merges all sources into
// one tree and the consumer count is small enough that a flat global is
// simpler than passing a Config down through every call site.
type Config struct {
	ForceInit   bool   `koanf:"force_init"`
	ProjectRoot string `koanf:"project_root"`
	Mode        string `koanf:"mode"`
	// IndexNonVCS is kept top-level so the legacy AIDE_INDEX_NON_VCS env var
	// (no AIDE_CODE_ prefix) still maps without bespoke rewrites.
	IndexNonVCS bool `koanf:"index_non_vcs"`

	Code    CodeConfig    `koanf:"code"`
	Pprof   PprofConfig   `koanf:"pprof"`
	Grammar GrammarConfig `koanf:"grammar"`
	Share   ShareConfig   `koanf:"share"`
	Memory  MemoryConfig  `koanf:"memory"`
}

// CodeConfig groups indexer / watcher tunables (AIDE_CODE_*).
type CodeConfig struct {
	Watch      bool   `koanf:"watch"`
	WatchPaths string `koanf:"watch_paths"`
	WatchDelay string `koanf:"watch_delay"`
	// StoreEnabled defaults to true. Set AIDE_CODE_STORE_ENABLED=0 (or
	// the legacy AIDE_CODE_STORE_DISABLE=1, which the loader inverts) to
	// keep the daemon from opening a code store.
	StoreEnabled bool `koanf:"store_enabled"`
	// StoreSync makes code-store opening synchronous (default false =
	// lazy-init in the background). AIDE_CODE_STORE_SYNC=1.
	StoreSync bool `koanf:"store_sync"`
}

// PprofConfig groups pprof http server tunables (AIDE_PPROF_*).
type PprofConfig struct {
	Enable bool   `koanf:"enable"`
	Addr   string `koanf:"addr"`
}

// GrammarConfig groups grammar-download tunables (AIDE_GRAMMAR_*).
type GrammarConfig struct {
	URL          string `koanf:"url"`
	AutoDownload string `koanf:"auto_download"`
}

// ShareConfig groups memory share-import tunables (AIDE_SHARE_*).
type ShareConfig struct {
	AutoImport bool `koanf:"auto_import"`
}

// MemoryConfig groups memory-scoring tunables (AIDE_MEMORY_*).
type MemoryConfig struct {
	// ScoringEnabled defaults to true. AIDE_MEMORY_SCORING_ENABLED=0 (or
	// the legacy AIDE_MEMORY_SCORING_DISABLED=1, inverted) kills scoring
	// and falls back to chronological ULID order.
	ScoringEnabled bool `koanf:"scoring_enabled"`
	// DecayEnabled defaults to true. AIDE_MEMORY_DECAY_ENABLED=0 (or
	// the legacy AIDE_MEMORY_DECAY_DISABLED=1, inverted) leaves scoring
	// running but pins the recency factor at 1.0 regardless of age.
	DecayEnabled bool `koanf:"decay_enabled"`
}

// WatchPathList parses Code.WatchPaths as a comma-separated list, returning
// an empty slice when the value is unset. Trims whitespace around entries.
func (c CodeConfig) WatchPathList() []string {
	if c.WatchPaths == "" {
		return nil
	}
	raw := strings.Split(c.WatchPaths, ",")
	out := make([]string, 0, len(raw))
	for _, s := range raw {
		if t := strings.TrimSpace(s); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// WatchDelayDuration parses Code.WatchDelay as a Go duration. Returns the
// supplied fallback when the value is empty or unparseable so callers don't
// each have to repeat the same defaulting dance.
func (c CodeConfig) WatchDelayDuration(fallback time.Duration) time.Duration {
	if c.WatchDelay == "" {
		return fallback
	}
	d, err := time.ParseDuration(c.WatchDelay)
	if err != nil {
		return fallback
	}
	return d
}

// envSections lists the koanf top-level keys that correspond to a
// double-underscore-bearing env-var section. AIDE_<SECTION>_<rest> is
// rewritten as <section>.<rest_with_underscores_kept>.
var envSections = map[string]struct{}{
	"code":    {},
	"pprof":   {},
	"grammar": {},
	"share":   {},
	"memory":  {},
}

// defaults are loaded as the lowest-precedence koanf source so that
// boolean fields whose semantic default is "on" (scoring, decay,
// store) keep that meaning when neither the file nor the env overrides
// them. Without this, Go's zero-value bool (false) would silently
// disable features users expect on by default.
var defaults = map[string]any{
	"code.store_enabled":     true,
	"memory.scoring_enabled": true,
	"memory.decay_enabled":   true,
}

// legacyDisabledVars maps each AIDE_*_DISABLED / AIDE_CODE_STORE_DISABLE
// env var (the names callers set before the rename) to its inverted
// koanf key. When the loader sees the legacy var, it negates the value
// and stores it under the new positive key. New names like
// AIDE_MEMORY_SCORING_ENABLED still flow through the normal envToKey
// path and override the legacy entry if both are set (env order).
var legacyDisabledVars = map[string]string{
	"AIDE_MEMORY_SCORING_DISABLED": "memory.scoring_enabled",
	"AIDE_MEMORY_DECAY_DISABLED":   "memory.decay_enabled",
	"AIDE_CODE_STORE_DISABLE":      "code.store_enabled",
}

// truthy reports whether an env-var value should be treated as "on".
// Matches what every other AIDE_*=1 var has historically accepted.
func truthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// envToKey converts a raw environment variable name into the dot-path koanf
// uses internally. The mapping preserves backwards compatibility with the
// pre-koanf env-var names so users don't see any behaviour change.
//
// Examples:
//
//	AIDE_CODE_WATCH        -> code.watch
//	AIDE_CODE_WATCH_PATHS  -> code.watch_paths
//	AIDE_FORCE_INIT        -> force_init
//	AIDE_INDEX_NON_VCS     -> index_non_vcs
func envToKey(name string) string {
	if !strings.HasPrefix(name, EnvPrefix) {
		return ""
	}
	tail := strings.ToLower(strings.TrimPrefix(name, EnvPrefix))
	parts := strings.SplitN(tail, "_", 2)
	if len(parts) == 2 {
		if _, ok := envSections[parts[0]]; ok {
			return parts[0] + "." + parts[1]
		}
	}
	return tail
}

var (
	mu     sync.RWMutex
	cached *Config
)

// Load reads configuration in this precedence order (each source overrides
// the previous): hard-coded defaults → optional .aide/config/aide.json →
// AIDE_* environment variables. The result is unmarshalled into a typed
// Config and stashed for retrieval via Get. Calling Load again replaces
// the previous value — useful in tests; production callers invoke it once
// at startup.
func Load(projectRoot string) (*Config, error) {
	k := koanf.New(".")

	if err := k.Load(confmap.Provider(defaults, "."), nil); err != nil {
		return nil, fmt.Errorf("loading defaults: %w", err)
	}

	if projectRoot != "" {
		path := projectRoot + "/.aide/config/aide.json"
		if err := k.Load(file.Provider(path), json.Parser()); err != nil {
			// Missing file is fine; only surface real parse errors.
			if !strings.Contains(err.Error(), "no such file") {
				return nil, fmt.Errorf("loading %s: %w", path, err)
			}
		}
	}

	if err := k.Load(env.Provider(".", env.Opt{
		Prefix: EnvPrefix,
		TransformFunc: func(name, value string) (string, any) {
			// Legacy AIDE_*_DISABLED vars are negated into the new
			// positive AIDE_*_ENABLED keys. The new env-var names map
			// through the normal path below.
			if key, ok := legacyDisabledVars[name]; ok {
				return key, !truthy(value)
			}
			key := envToKey(name)
			if key == "" {
				return "", nil
			}
			return key, value
		},
	}), nil); err != nil {
		return nil, fmt.Errorf("loading env vars: %w", err)
	}

	cfg := &Config{}
	if err := k.Unmarshal("", cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	mu.Lock()
	cached = cfg
	mu.Unlock()
	return cfg, nil
}

// Get returns the loaded Config. If Load hasn't been called yet, it returns
// a zero-valued Config so callers don't have to nil-check. Tests may also
// inject a value via Set.
func Get() *Config {
	mu.RLock()
	defer mu.RUnlock()
	if cached == nil {
		return &Config{}
	}
	return cached
}

// Set replaces the cached config. Intended for tests.
func Set(c *Config) {
	mu.Lock()
	cached = c
	mu.Unlock()
}
