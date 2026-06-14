// Package config centralizes runtime configuration for the aide CLI and
// daemon. Values flow from (in increasing precedence): defaults → optional
// ~/.aide/config/aide.json global file → optional project .aide/config/aide.json
// → AIDE_* environment variables → top-level CLI overrides written back into the
// env (e.g. --project-root sets AIDE_PROJECT_ROOT before Load is called). The
// global file is config only; data and the database stay project-scoped.
//
// Callers ask for the singleton via Get(); the bootstrap path in main.go is
// responsible for invoking Load() exactly once at startup.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
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
	// IndexWorkers caps the number of parallel tree-sitter parser
	// goroutines the Index handler spawns. Zero or unset => runtime.NumCPU().
	// Negative values are treated as zero. Values above 32 are clamped.
	// Use IndexWorkerCount() to read the resolved value.
	IndexWorkers int `koanf:"index_workers"`

	Code        CodeConfig        `koanf:"code"`
	Pprof       PprofConfig       `koanf:"pprof"`
	Grammar     GrammarConfig     `koanf:"grammar"`
	Share       ShareConfig       `koanf:"share"`
	Memory      MemoryConfig      `koanf:"memory"`
	Reflect     ReflectConfig     `koanf:"reflect"`
	Cleanup     CleanupConfig     `koanf:"cleanup"`
	Maintenance MaintenanceConfig `koanf:"maintenance"`
}

// MaintenanceConfig controls on-disk upkeep of the bolt stores. bbolt never
// returns freed pages to the OS, so a store file only shrinks when rewritten;
// CompactOnExit does that rewrite when a long-lived store owner (the daemon or
// the MCP server) shuts down. Default true; disable with
// maintenance.compact_on_exit=false or AIDE_MAINTENANCE_COMPACT_ON_EXIT=0.
type MaintenanceConfig struct {
	CompactOnExit bool `koanf:"compact_on_exit"`
}

// CleanupConfig controls the daemon's background bucket-pruning loop.
// All durations are Go duration strings ("15m", "168h"). Empty values fall
// back to the defaults in the accessor methods (one year). A value of "0"
// disables pruning for that bucket entirely — its records are retained forever.
// Disable the whole loop with AIDE_CLEANUP_ENABLED=0 or cleanup.enabled=false.
type CleanupConfig struct {
	Enabled       bool   `koanf:"enabled"`         // default true; AIDE_CLEANUP_ENABLED=0 to disable
	Interval      string `koanf:"interval"`        // default "15m"
	StateMaxAge   string `koanf:"state_max_age"`   // default "8760h" (365d) — agent-specific state only
	ObserveMaxAge string `koanf:"observe_max_age"` // default "8760h" (365d)
	TaskMaxAge    string `koanf:"task_max_age"`    // default "8760h" (365d) — done tasks only; pending/claimed/blocked never pruned
	TokenMaxAge   string `koanf:"token_max_age"`   // default "8760h" (365d) — token events back the token-intelligence page
}

// defaultBucketMaxAge is the fallback TTL for the time-based cleanup buckets
// (observe events, agent state, done tasks, token events) used when the matching
// cleanup.*_max_age value is empty. Deliberately long — a full year — so nothing
// is dropped under normal use; tune any bucket down via config, or set it to "0"
// to disable pruning for that bucket entirely.
const defaultBucketMaxAge = 365 * 24 * time.Hour

// IntervalDuration returns the tick interval with a default fallback.
func (c CleanupConfig) IntervalDuration() time.Duration {
	return parseDur(c.Interval, 15*time.Minute)
}

// StateMaxAgeDuration returns the state TTL with a default fallback.
func (c CleanupConfig) StateMaxAgeDuration() time.Duration {
	return parseDur(c.StateMaxAge, defaultBucketMaxAge)
}

// ObserveMaxAgeDuration returns the observe-event TTL with a default fallback.
func (c CleanupConfig) ObserveMaxAgeDuration() time.Duration {
	return parseDur(c.ObserveMaxAge, defaultBucketMaxAge)
}

// TaskMaxAgeDuration returns the done-task TTL with a default fallback.
func (c CleanupConfig) TaskMaxAgeDuration() time.Duration {
	return parseDur(c.TaskMaxAge, defaultBucketMaxAge)
}

// TokenMaxAgeDuration returns the token-event TTL with a default fallback.
// Token events back the "Last 30 / 60 / 90 days" trend filters on the token
// intelligence page, so the default keeps a full year of history.
func (c CleanupConfig) TokenMaxAgeDuration() time.Duration {
	return parseDur(c.TokenMaxAge, defaultBucketMaxAge)
}

func parseDur(s string, fallback time.Duration) time.Duration {
	if s == "" {
		return fallback
	}
	if d, err := time.ParseDuration(s); err == nil {
		return d
	}
	return fallback
}

// ReflectConfig groups instinct-extraction (reflect) tunables. The env var
// AIDE_REFLECT (truthy values: 1/true/on/yes) takes precedence over the
// file-set value at read time — see ResolveReflectEnabled.
type ReflectConfig struct {
	Enabled    bool                    `koanf:"enabled"`
	Repetition ReflectRepetitionConfig `koanf:"repetition"`
}

// ReflectRepetitionConfig is the user-tunable subset of the repetition
// detector. Empty / zero-value fields fall back to package defaults
// (see instinct.DefaultRepetitionConfig); set fields override entirely.
type ReflectRepetitionConfig struct {
	// MinCount fires a proposal when a command's signature appears at
	// least this many times. Zero → default (4).
	MinCount int `koanf:"min_count"`
	// WindowMinutes constrains the densest run to this span. Zero → default (30).
	WindowMinutes int `koanf:"window_minutes"`
	// IgnoreCommands is a full replacement of the default ignore list
	// (`git status`, `git add`, `ls`, `pwd`). Nil → defaults; non-nil →
	// exactly this list. Signatures match what instinct.normaliseBash
	// produces — for git/docker/etc, the subcommand is kept
	// (e.g. "git status" not "git").
	IgnoreCommands []string `koanf:"ignore_commands"`
}

// ResolveReflectEnabled returns the effective reflect-enabled state given
// the loaded config. Precedence: env (truthy/falsy) overrides config.
// When env is unset or unrecognised, the config value wins (default false).
func ResolveReflectEnabled(cfg *Config) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("AIDE_REFLECT")))
	switch v {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}
	if cfg == nil {
		return false
	}
	return cfg.Reflect.Enabled
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

// ShareConfig groups the symmetric export/import sharing policy. AutoImport is
// the AIDE_SHARE_AUTO_IMPORT toggle that opts a session into importing shared
// data at init; Decisions and Memories carry per-type export/import gates and
// include/exclude filters. The per-type *bool fields distinguish "unset"
// (apply the type default) from an explicit false — see the resolved accessors.
type ShareConfig struct {
	AutoImport bool            `koanf:"auto_import"`
	Decisions  ShareTypePolicy `koanf:"decisions"`
	Memories   ShareTypePolicy `koanf:"memories"`
}

// ShareTypePolicy is the export/import policy for one record type (decisions or
// memories). Export and Import are *bool so an unset value (nil) falls back to
// the type default rather than to Go's zero-value false — decisions default to
// export AND import on, which a plain bool could not express.
type ShareTypePolicy struct {
	Export       *bool       `koanf:"export"` // nil = type default
	Import       *bool       `koanf:"import"` // nil = type default
	ExportFilter ShareFilter `koanf:"export_filter"`
	ImportFilter ShareFilter `koanf:"import_filter"`
}

// ShareFilter is a generic include/exclude glob filter over a record's tokens.
// It mirrors contextshare.Filter but lives here so config stays self-contained;
// the cmd layer maps one onto the other.
type ShareFilter struct {
	Include []string `koanf:"include"`
	Exclude []string `koanf:"exclude"`
}

// shareTypeDefaults captures the type defaults applied when a policy leaves a
// field unset: decisions flow both ways with no filtering, memories are opt-in
// both ways and exclude personal/ephemeral records by default.
type shareTypeDefaults struct {
	export        bool
	importEnabled bool
	exclude       []string
}

var (
	decisionDefaults = shareTypeDefaults{export: true, importEnabled: true, exclude: nil}
	memoryDefaults   = shareTypeDefaults{export: false, importEnabled: false, exclude: []string{"scope:global", "session:*"}}
)

// resolveBool returns *p when set, otherwise def.
func resolveBool(p *bool, def bool) bool {
	if p != nil {
		return *p
	}
	return def
}

// resolveFilter normalises a configured filter against its type default: an
// empty Include means ["*"] (match all); an unset (nil) Exclude inherits the
// type's default exclude list. koanf/mapstructure unmarshal an absent key to a
// nil slice but an explicit JSON "[]" to a non-nil empty slice, so a user can
// clear the default memory exclusions (publish scope:global / session:* too) by
// setting "exclude": [] explicitly; only a fully unset Exclude inherits the
// default. This matches the design's stated defaults.
func resolveFilter(f ShareFilter, def shareTypeDefaults) ShareFilter {
	include := f.Include
	if len(include) == 0 {
		include = []string{"*"}
	}
	exclude := f.Exclude
	if exclude == nil {
		exclude = def.exclude
	}
	return ShareFilter{Include: include, Exclude: exclude}
}

// DecisionExportEnabled reports whether decisions are published (default true).
func (c ShareConfig) DecisionExportEnabled() bool {
	return resolveBool(c.Decisions.Export, decisionDefaults.export)
}

// DecisionImportEnabled reports whether decisions are consumed (default true).
func (c ShareConfig) DecisionImportEnabled() bool {
	return resolveBool(c.Decisions.Import, decisionDefaults.importEnabled)
}

// MemoryExportEnabled reports whether memories are published (default false).
func (c ShareConfig) MemoryExportEnabled() bool {
	return resolveBool(c.Memories.Export, memoryDefaults.export)
}

// MemoryImportEnabled reports whether memories are consumed (default false).
func (c ShareConfig) MemoryImportEnabled() bool {
	return resolveBool(c.Memories.Import, memoryDefaults.importEnabled)
}

// DecisionExportFilter resolves the decision export filter with defaults.
func (c ShareConfig) DecisionExportFilter() ShareFilter {
	return resolveFilter(c.Decisions.ExportFilter, decisionDefaults)
}

// DecisionImportFilter resolves the decision import filter with defaults.
func (c ShareConfig) DecisionImportFilter() ShareFilter {
	return resolveFilter(c.Decisions.ImportFilter, decisionDefaults)
}

// MemoryExportFilter resolves the memory export filter with defaults
// (default exclude: scope:global, session:*).
func (c ShareConfig) MemoryExportFilter() ShareFilter {
	return resolveFilter(c.Memories.ExportFilter, memoryDefaults)
}

// MemoryImportFilter resolves the memory import filter with defaults
// (default exclude: scope:global, session:*).
func (c ShareConfig) MemoryImportFilter() ShareFilter {
	return resolveFilter(c.Memories.ImportFilter, memoryDefaults)
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
	// InjectionTokenBudget caps the estimated tokens of memory content
	// injected at session start. Memories are taken in score order (highest
	// first) until the budget is reached; lower-ranked memories beyond it are
	// dropped whole — never truncated mid-memory. 0 disables the budget (inject
	// all, prior behaviour). Set via AIDE_MEMORY_INJECTION_TOKEN_BUDGET.
	InjectionTokenBudget int `koanf:"injection_token_budget"`
}

// IndexWorkerCount resolves Config.IndexWorkers into a positive worker
// count: zero or negative falls back to runtime.NumCPU(); values above
// the upper bound clamp down. Past ~16-32 the indexer is bottlenecked
// elsewhere (single bbolt write tx, Bleve batch apply) so larger
// settings just consume more memory for no throughput gain.
func (c *Config) IndexWorkerCount() int {
	const maxWorkers = 32
	n := c.IndexWorkers
	if n <= 0 {
		n = runtime.NumCPU()
	}
	if n > maxWorkers {
		n = maxWorkers
	}
	return n
}

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
	"code":        {},
	"pprof":       {},
	"grammar":     {},
	"share":       {},
	"memory":      {},
	"reflect":     {},
	"cleanup":     {},
	"maintenance": {},
}

// envBareKey maps AIDE_<NAME> (no underscore tail) to a non-default koanf
// key when the section name on its own should pin a specific scalar field.
// Used for ergonomic shortcuts: AIDE_REFLECT=1 means reflect.enabled=true.
var envBareKey = map[string]string{
	"reflect": "reflect.enabled",
}

// defaults are loaded as the lowest-precedence koanf source so that
// boolean fields whose semantic default is "on" (scoring, decay,
// store) keep that meaning when neither the file nor the env overrides
// them. Without this, Go's zero-value bool (false) would silently
// disable features users expect on by default.
var defaults = map[string]any{
	"code.store_enabled":            true,
	"memory.scoring_enabled":        true,
	"memory.decay_enabled":          true,
	"memory.injection_token_budget": 8000,
	"cleanup.enabled":               true,
	"maintenance.compact_on_exit":   true,
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
	if mapped, ok := envBareKey[tail]; ok {
		return mapped
	}
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

// ConfigFileName is the project-relative path of the JSON config file the
// loader reads and the `aide config` command writes. Exported so the cmd layer
// can derive the same path without re-hardcoding it.
const ConfigFileName = ".aide/config/aide.json"

// FilePath returns the absolute path of the config file for a project root.
// An empty projectRoot yields the bare relative path.
func FilePath(projectRoot string) string {
	if projectRoot == "" {
		return ConfigFileName
	}
	return projectRoot + "/" + ConfigFileName
}

// GlobalFilePath returns the absolute path of the user-global config file,
// ~/.aide/config/aide.json. This is config only — never data or a database. It
// sits between the hard-coded defaults and the per-project file in precedence,
// so a value set here applies to every project unless that project overrides it.
// Returns "" when the home directory cannot be resolved, in which case the
// loader simply skips the global layer. The leaf name is taken from
// ConfigFileName so the global and project files always share a basename.
func GlobalFilePath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".aide", "config", filepath.Base(ConfigFileName))
}

// loadJSONFile merges a JSON config file into k. A missing file is tolerated
// (config files are optional at every layer); only real parse errors surface.
func loadJSONFile(k *koanf.Koanf, path string) error {
	if err := k.Load(file.Provider(path), json.Parser()); err != nil {
		// Missing file is fine; only surface real parse errors.
		if !strings.Contains(err.Error(), "no such file") {
			return fmt.Errorf("loading %s: %w", path, err)
		}
	}
	return nil
}

// buildKoanf layers the four configuration sources (hard-coded defaults →
// optional ~/.aide/config/aide.json global file → optional project
// .aide/config/aide.json → AIDE_* environment variables) into a koanf instance
// in the order that gives each source precedence over the previous. Both Load
// (which unmarshals into a typed Config and caches it) and the read-only
// `aide config` command resolve values through this single helper so the two
// never drift apart.
func buildKoanf(projectRoot string) (*koanf.Koanf, error) {
	k := koanf.New(".")

	if err := k.Load(confmap.Provider(defaults, "."), nil); err != nil {
		return nil, fmt.Errorf("loading defaults: %w", err)
	}

	// Global user config sits above the defaults and below the project file, so
	// a user-wide preference applies everywhere unless a project overrides it.
	if global := GlobalFilePath(); global != "" {
		if err := loadJSONFile(k, global); err != nil {
			return nil, err
		}
	}

	if projectRoot != "" {
		if err := loadJSONFile(k, FilePath(projectRoot)); err != nil {
			return nil, err
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

	return k, nil
}

// Load reads configuration in this precedence order (each source overrides the
// previous): hard-coded defaults → optional ~/.aide/config/aide.json global file
// → optional project .aide/config/aide.json → AIDE_* environment variables. The
// result is unmarshalled into a typed Config and stashed for retrieval via Get.
// Calling Load again replaces the previous value — useful in tests; production
// callers invoke it once at startup.
func Load(projectRoot string) (*Config, error) {
	k, err := buildKoanf(projectRoot)
	if err != nil {
		return nil, err
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

// Resolve layers the same sources as Load (defaults → global file → project
// file → env) and returns the resulting koanf tree without caching or mutating
// package state.
// The `aide config` command uses it to read resolved values and enumerate keys
// for `show` without disturbing the singleton that the rest of the process
// relies on.
func Resolve(projectRoot string) (*koanf.Koanf, error) {
	return buildKoanf(projectRoot)
}

// FieldKind classifies a config leaf field by the Go type the JSON writer must
// coerce a string argument to. It is the bridge between the dotted koanf keys a
// user types and the typed values stored in aide.json.
type FieldKind int

const (
	// KindBool is a plain bool leaf (e.g. cleanup.enabled).
	KindBool FieldKind = iota
	// KindPtrBool is a *bool leaf whose nil distinguishes "unset" from an
	// explicit false (e.g. share.decisions.export).
	KindPtrBool
	// KindString is a string leaf (e.g. mode, cleanup.observe_max_age).
	KindString
	// KindInt is an int leaf (e.g. index_workers).
	KindInt
	// KindStringSlice is a []string leaf (e.g. share.memories.export_filter.exclude).
	KindStringSlice
)

// String renders a FieldKind for help and error messages.
func (k FieldKind) String() string {
	switch k {
	case KindBool:
		return "bool"
	case KindPtrBool:
		return "bool"
	case KindString:
		return "string"
	case KindInt:
		return "int"
	case KindStringSlice:
		return "list"
	default:
		return "unknown"
	}
}

// FieldInfo describes one settable leaf in the Config tree: the koanf dot-path
// a user types and the Go kind the value coerces to.
type FieldInfo struct {
	Key  string
	Kind FieldKind
}

// Schema walks the Config struct via reflection and returns every settable leaf
// as a FieldInfo keyed by its koanf dot-path. It recurses into nested structs,
// composing parent keys with ".", and treats bool / *bool / string / int /
// []string as leaves. Fields without a koanf tag (or tagged "-") are skipped.
// This single source of truth powers `config set` coercion, unknown-key
// validation, and `config show --all`.
func Schema() []FieldInfo {
	var out []FieldInfo
	walkSchema(reflect.TypeOf(Config{}), "", &out)
	return out
}

// walkSchema appends the leaves of t (prefixed by prefix) to out. Leaf kinds are
// recognised exactly; any other kind (including unhandled nested non-struct
// types) is skipped rather than guessed at, so adding an exotic field can never
// silently produce a wrong coercion.
func walkSchema(t reflect.Type, prefix string, out *[]FieldInfo) {
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag := f.Tag.Get("koanf")
		if tag == "" || tag == "-" {
			continue
		}
		key := tag
		if prefix != "" {
			key = prefix + "." + tag
		}

		ft := f.Type
		switch ft.Kind() {
		case reflect.Bool:
			*out = append(*out, FieldInfo{Key: key, Kind: KindBool})
		case reflect.String:
			*out = append(*out, FieldInfo{Key: key, Kind: KindString})
		case reflect.Int:
			*out = append(*out, FieldInfo{Key: key, Kind: KindInt})
		case reflect.Ptr:
			if ft.Elem().Kind() == reflect.Bool {
				*out = append(*out, FieldInfo{Key: key, Kind: KindPtrBool})
			}
		case reflect.Slice:
			if ft.Elem().Kind() == reflect.String {
				*out = append(*out, FieldInfo{Key: key, Kind: KindStringSlice})
			}
		case reflect.Struct:
			walkSchema(ft, key, out)
		default:
			// Other kinds are not settable config leaves; skip them.
		}
	}
}

// Lookup returns the FieldInfo for a dotted key, or false when the key is not a
// known settable leaf. Callers use this to validate user-supplied keys and pick
// the right coercion.
func Lookup(key string) (FieldInfo, bool) {
	for _, fi := range Schema() {
		if fi.Key == key {
			return fi, true
		}
	}
	return FieldInfo{}, false
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
