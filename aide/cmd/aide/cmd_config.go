// Package main provides the config command for inspecting and editing the
// resolved aide configuration.
//
// Configuration flows from (in increasing precedence) hard-coded defaults →
// ~/.aide/config/aide.json global file → project .aide/config/aide.json →
// AIDE_* environment variables (see pkg/config). The read-only subcommands
// (show, get) resolve values through the exact same layering as the daemon via
// config.Resolve, so what you see is what the rest of aide sees. The write
// subcommands (set, unset) edit only the project .aide/config/aide.json by
// default, or the global ~/.aide/config/aide.json with --global — they never
// touch env or any other file — and store values with their real JSON type
// (bool / number / array, not a string) so the koanf loader round-trips them.
// path prints the project file, or the global file with --global. The set of
// settable keys, and the Go type each coerces to, comes entirely from
// config.Schema() so the CLI and the loader can never disagree about what exists.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/jmylchreest/aide/aide/pkg/config"
	"github.com/knadh/koanf/v2"
)

// cmdConfig dispatches config subcommands.
func cmdConfig(dbPath string, args []string) error {
	return dispatchSubcmd("config", args, printConfigUsage, []subcmd{
		{name: "show", handler: func(a []string) error { return cmdConfigShow(dbPath, a) }},
		{name: "get", handler: func(a []string) error { return cmdConfigGet(dbPath, a) }},
		{name: "set", handler: func(a []string) error { return cmdConfigSet(dbPath, a) }},
		{name: "unset", handler: func(a []string) error { return cmdConfigUnset(dbPath, a) }},
		{name: "path", handler: func(a []string) error { return cmdConfigPath(dbPath, a) }},
	})
}

func printConfigUsage() {
	fmt.Println(`aide config - Inspect and edit aide configuration

Configuration resolves from defaults → ~/.aide/config/aide.json (global) →
project .aide/config/aide.json → AIDE_* env vars (later sources win). Keys are
koanf dot-paths, e.g. share.decisions.export or cleanup.observe_max_age. set,
unset and path act on the project file by default, or the global file with
--global; env-var overrides are reported but never written.

Usage:
  aide config <subcommand> [arguments]

Subcommands:
  show [--all] [--json]   Show the resolved config (grouped). --all lists every
                          key with its value, default and source (env/file/
                          global/default); --json emits machine-readable output.
  get <key>               Print one resolved value.
  set [--global] <key> <value...>
                          Set a value in aide.json (--global writes the global
                          file). List keys take multiple args or a single
                          comma-joined arg; set with no values clears a list to [].
  unset [--global] <key>  Remove a key from aide.json (reverts to default/env);
                          --global edits the global file.
  path [--global]         Print the path to aide.json; --global prints the
                          global file path.

Examples:
  aide config show
  aide config show --all
  aide config show --json
  aide config get share.decisions.export
  aide config set share.decisions.export false
  aide config set --global cleanup.observe_max_age 720h
  aide config set share.memories.export_filter.exclude scope:global session:*
  aide config set share.memories.export_filter.exclude scope:global,session:*
  aide config unset share.decisions.export
  aide config unset --global cleanup.observe_max_age
  aide config path
  aide config path --global`)
}

// --- path ---

func cmdConfigPath(dbPath string, args []string) error {
	if hasFlag(args, "--global") {
		path := config.GlobalFilePath()
		if path == "" {
			return errNoGlobalHome
		}
		fmt.Println(path)
		return nil
	}
	fmt.Println(config.FilePath(projectRoot(dbPath)))
	return nil
}

// errNoGlobalHome is returned by the --global write/path paths when the user's
// home directory cannot be resolved, so the global config file location is
// unknown. Without a home there is nowhere to read or write the global file.
var errNoGlobalHome = fmt.Errorf("cannot resolve --global config path: home directory is unknown")

// targetConfigPath returns the file path the write subcommands (set, unset)
// should edit: the global ~/.aide/config/aide.json when --global is present,
// else the project file. It errors when --global is asked for but the home
// directory cannot be resolved.
func targetConfigPath(dbPath string, args []string) (string, error) {
	if hasFlag(args, "--global") {
		path := config.GlobalFilePath()
		if path == "" {
			return "", errNoGlobalHome
		}
		return path, nil
	}
	return config.FilePath(projectRoot(dbPath)), nil
}

// --- get ---

func cmdConfigGet(dbPath string, args []string) error {
	key := firstPositional(args)
	if key == "" {
		return fmt.Errorf("usage: aide config get <key>")
	}
	fi, ok := config.Lookup(key)
	if !ok {
		return unknownKeyError(key)
	}
	root := projectRoot(dbPath)
	k, err := config.Resolve(root)
	if err != nil {
		return err
	}
	cfg, err := config.Load(root)
	if err != nil {
		return err
	}
	fmt.Println(formatValue(resolveValue(k, cfg, fi)))
	return nil
}

// --- show ---

// configKeyView is one line of `config show --all`, in both human and --json
// forms: the resolved value plus the bare default and the source it came from.
type configKeyView struct {
	Key     string `json:"key"`
	Value   any    `json:"value"`
	Default any    `json:"default"`
	Source  string `json:"source"` // env | file | global | default
}

func cmdConfigShow(dbPath string, args []string) error {
	all := hasFlag(args, "--all")
	root := projectRoot(dbPath)

	cfg, err := config.Load(root)
	if err != nil {
		return err
	}

	if all {
		views, err := buildConfigKeyViews(root, cfg)
		if err != nil {
			return err
		}
		if wantJSON(args) {
			return printJSON(views)
		}
		printConfigKeyViews(views)
		return nil
	}

	// Default show: --json marshals the resolved typed Config; the human form
	// prints it grouped by section.
	if wantJSON(args) {
		return printJSON(cfg)
	}
	printResolvedConfig(cfg)
	return nil
}

// buildConfigKeyViews enumerates every schema key and records its resolved
// value, its bare default, and where the resolved value came from.
func buildConfigKeyViews(root string, cfg *config.Config) ([]configKeyView, error) {
	k, err := config.Resolve(root)
	if err != nil {
		return nil, err
	}
	fileKeys := fileKeySet(config.FilePath(root))
	globalKeys := fileKeySet(config.GlobalFilePath())
	defCfg := defaultConfig()
	defK := defaultKoanf()

	schema := config.Schema()
	views := make([]configKeyView, 0, len(schema))
	for _, fi := range schema {
		views = append(views, configKeyView{
			Key:     fi.Key,
			Value:   resolveValue(k, cfg, fi),
			Default: resolveValue(defK, defCfg, fi),
			Source:  keySource(fi.Key, fileKeys, globalKeys),
		})
	}
	return views, nil
}

// printConfigKeyViews prints the --all table: one line per key with the
// resolved value, the default, and the source. env-overridden keys are flagged.
func printConfigKeyViews(views []configKeyView) {
	w := newTabWriter()
	fmt.Fprintln(w, "KEY\tVALUE\tDEFAULT\tSOURCE")
	for _, v := range views {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			v.Key, formatValue(v.Value), formatValue(v.Default), v.Source)
	}
	w.Flush()
}

// printResolvedConfig renders the resolved config grouped by section. It reads
// the resolved Config for accessor-defaulted values (the share *bool fields) and
// the koanf tree for the plain scalars.
func printResolvedConfig(cfg *config.Config) {
	fmt.Println("general")
	fmt.Printf("  mode            %s\n", emptyDash(cfg.Mode))
	fmt.Printf("  project_root    %s\n", emptyDash(cfg.ProjectRoot))
	fmt.Printf("  force_init      %v\n", cfg.ForceInit)
	fmt.Printf("  index_non_vcs   %v\n", cfg.IndexNonVCS)
	fmt.Printf("  index_workers   %d\n", cfg.IndexWorkers)

	fmt.Println("\ncode")
	fmt.Printf("  watch           %v\n", cfg.Code.Watch)
	fmt.Printf("  watch_paths     %s\n", emptyDash(cfg.Code.WatchPaths))
	fmt.Printf("  watch_delay     %s\n", emptyDash(cfg.Code.WatchDelay))
	fmt.Printf("  store_enabled   %v\n", cfg.Code.StoreEnabled)
	fmt.Printf("  store_sync      %v\n", cfg.Code.StoreSync)

	fmt.Println("\npprof")
	fmt.Printf("  enable          %v\n", cfg.Pprof.Enable)
	fmt.Printf("  addr            %s\n", emptyDash(cfg.Pprof.Addr))

	fmt.Println("\ngrammar")
	fmt.Printf("  url             %s\n", emptyDash(cfg.Grammar.URL))
	fmt.Printf("  auto_download   %s\n", emptyDash(cfg.Grammar.AutoDownload))

	fmt.Println("\nmemory")
	fmt.Printf("  scoring_enabled %v\n", cfg.Memory.ScoringEnabled)
	fmt.Printf("  decay_enabled   %v\n", cfg.Memory.DecayEnabled)

	fmt.Println("\nreflect")
	fmt.Printf("  enabled         %v\n", cfg.Reflect.Enabled)
	fmt.Printf("  repetition.min_count       %d\n", cfg.Reflect.Repetition.MinCount)
	fmt.Printf("  repetition.window_minutes  %d\n", cfg.Reflect.Repetition.WindowMinutes)
	fmt.Printf("  repetition.ignore_commands %s\n", fmtList(cfg.Reflect.Repetition.IgnoreCommands))

	fmt.Println("\ncleanup")
	fmt.Printf("  enabled         %v\n", cfg.Cleanup.Enabled)
	fmt.Printf("  interval        %s\n", emptyDash(cfg.Cleanup.Interval))
	fmt.Printf("  state_max_age   %s\n", emptyDash(cfg.Cleanup.StateMaxAge))
	fmt.Printf("  observe_max_age %s\n", emptyDash(cfg.Cleanup.ObserveMaxAge))
	fmt.Printf("  task_max_age    %s\n", emptyDash(cfg.Cleanup.TaskMaxAge))
	fmt.Printf("  token_max_age   %s\n", emptyDash(cfg.Cleanup.TokenMaxAge))

	fmt.Println("\nmaintenance")
	fmt.Printf("  compact_on_exit %v\n", cfg.Maintenance.CompactOnExit)

	fmt.Println("\nshare")
	fmt.Printf("  auto_import     %v\n", cfg.Share.AutoImport)
	printShareTypePolicy("decisions",
		cfg.Share.DecisionExportEnabled(), cfg.Share.DecisionImportEnabled(),
		cfg.Share.DecisionExportFilter(), cfg.Share.DecisionImportFilter())
	printShareTypePolicy("memories",
		cfg.Share.MemoryExportEnabled(), cfg.Share.MemoryImportEnabled(),
		cfg.Share.MemoryExportFilter(), cfg.Share.MemoryImportFilter())
}

func printShareTypePolicy(label string, exp, imp bool, expF, impF config.ShareFilter) {
	fmt.Printf("  %s\n", label)
	fmt.Printf("    export %-3s  import %s\n", onOff(exp), onOff(imp))
	fmt.Printf("    export_filter include %s exclude %s\n", fmtList(expF.Include), fmtList(expF.Exclude))
	fmt.Printf("    import_filter include %s exclude %s\n", fmtList(impF.Include), fmtList(impF.Exclude))
}

// --- set ---

func cmdConfigSet(dbPath string, args []string) error {
	positionals := positionals(args)
	if len(positionals) == 0 {
		return fmt.Errorf("usage: aide config set <key> <value...>")
	}
	key := positionals[0]
	values := positionals[1:]

	fi, ok := config.Lookup(key)
	if !ok {
		return unknownKeyError(key)
	}

	typed, err := coerce(fi, values)
	if err != nil {
		return err
	}

	path, err := targetConfigPath(dbPath, args)
	if err != nil {
		return err
	}
	m, err := readConfigMap(path)
	if err != nil {
		return err
	}
	setNested(m, strings.Split(key, "."), typed)
	if err := writeConfigMap(path, m); err != nil {
		return err
	}

	fmt.Printf("set %s = %s\n", key, formatValue(typed))
	if name, overrides := envOverride(key); overrides {
		fmt.Printf("  (note: overridden by env %s=%q)\n", name, os.Getenv(name))
	}
	return nil
}

// coerce converts the raw string arg(s) for a key into the typed Go value its
// schema kind requires, ready to be stored verbatim in the JSON map. List keys
// flatten every arg on commas; a list key with no args yields an explicit empty
// slice (intentionally clearing a defaulted list). Scalar keys take exactly one
// value.
func coerce(fi config.FieldInfo, values []string) (any, error) {
	switch fi.Kind {
	case config.KindStringSlice:
		out := []string{}
		for _, v := range values {
			for _, part := range strings.Split(v, ",") {
				if t := strings.TrimSpace(part); t != "" {
					out = append(out, t)
				}
			}
		}
		return out, nil
	case config.KindBool, config.KindPtrBool:
		v, err := singleValue(fi.Key, values)
		if err != nil {
			return nil, err
		}
		b, err := strconv.ParseBool(v)
		if err != nil {
			return nil, fmt.Errorf("invalid bool value %q for %s: want true or false", v, fi.Key)
		}
		return b, nil
	case config.KindInt:
		v, err := singleValue(fi.Key, values)
		if err != nil {
			return nil, err
		}
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("invalid int value %q for %s", v, fi.Key)
		}
		return n, nil
	case config.KindString:
		v, err := singleValue(fi.Key, values)
		if err != nil {
			return nil, err
		}
		return v, nil
	default:
		return nil, fmt.Errorf("unsupported kind for key %s", fi.Key)
	}
}

// singleValue enforces that a scalar key got exactly one value. Quoting and
// globbing are the shell's job; aide stores whatever literal it receives.
func singleValue(key string, values []string) (string, error) {
	if len(values) != 1 {
		return "", fmt.Errorf("%s takes exactly one value, got %d", key, len(values))
	}
	return values[0], nil
}

// --- unset ---

func cmdConfigUnset(dbPath string, args []string) error {
	key := firstPositional(args)
	if key == "" {
		return fmt.Errorf("usage: aide config unset <key>")
	}
	if _, ok := config.Lookup(key); !ok {
		return unknownKeyError(key)
	}

	path, err := targetConfigPath(dbPath, args)
	if err != nil {
		return err
	}
	m, err := readConfigMap(path)
	if err != nil {
		return err
	}
	deleteNested(m, strings.Split(key, "."))
	if err := writeConfigMap(path, m); err != nil {
		return err
	}

	fmt.Printf("unset %s\n", key)
	if name, overrides := envOverride(key); overrides {
		fmt.Printf("  (note: still overridden by env %s=%q)\n", name, os.Getenv(name))
	}
	return nil
}

// --- JSON file manipulation ---

// readConfigMap reads aide.json into a generic map. A missing file yields an
// empty map so the first `set` can create it; a present-but-unparseable file is
// a hard error rather than a silent overwrite.
func readConfigMap(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, nil
		}
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return map[string]any{}, nil
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	if m == nil {
		m = map[string]any{}
	}
	return m, nil
}

// writeConfigMap writes the map back as 2-space-indented JSON, creating the
// .aide/config/ directory on first write. An empty map is written as `{}` so the
// file always remains valid JSON the loader can read.
func writeConfigMap(path string, m map[string]any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding config: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

// setNested walks (creating as needed) the parent maps for a dotted key and
// stores value at the leaf. A non-map sitting where a parent map should be is
// replaced so a re-typed key can't wedge the file.
func setNested(m map[string]any, parts []string, value any) {
	for i := 0; i < len(parts)-1; i++ {
		child, ok := m[parts[i]].(map[string]any)
		if !ok {
			child = map[string]any{}
			m[parts[i]] = child
		}
		m = child
	}
	m[parts[len(parts)-1]] = value
}

// deleteNested removes the leaf for a dotted key and prunes any parent maps it
// leaves empty, so unsetting the last key under a section doesn't strand an
// empty `{}` object in the file.
func deleteNested(m map[string]any, parts []string) {
	if len(parts) == 1 {
		delete(m, parts[0])
		return
	}
	child, ok := m[parts[0]].(map[string]any)
	if !ok {
		return
	}
	deleteNested(child, parts[1:])
	if len(child) == 0 {
		delete(m, parts[0])
	}
}

// --- resolution / formatting helpers ---

// resolveValue returns the resolved value of a schema leaf, formatted to its Go
// kind. For *bool keys the koanf tree carries the value only when it was set in
// the file or env; an unset *bool falls back to the resolved Config accessor so
// the type default (e.g. decisions.export = true) is reflected, not Go's false.
func resolveValue(k *koanf.Koanf, cfg *config.Config, fi config.FieldInfo) any {
	switch fi.Kind {
	case config.KindBool:
		return k.Bool(fi.Key)
	case config.KindPtrBool:
		if k.Exists(fi.Key) {
			return k.Bool(fi.Key)
		}
		return ptrBoolDefault(cfg, fi.Key)
	case config.KindInt:
		return k.Int(fi.Key)
	case config.KindString:
		return k.String(fi.Key)
	case config.KindStringSlice:
		return k.Strings(fi.Key)
	default:
		return k.Get(fi.Key)
	}
}

// ptrBoolDefault resolves the four share *bool keys through their accessor
// defaults when neither file nor env set them. Any other *bool key (none today)
// defaults to false.
func ptrBoolDefault(cfg *config.Config, key string) bool {
	switch key {
	case "share.decisions.export":
		return cfg.Share.DecisionExportEnabled()
	case "share.decisions.import":
		return cfg.Share.DecisionImportEnabled()
	case "share.memories.export":
		return cfg.Share.MemoryExportEnabled()
	case "share.memories.import":
		return cfg.Share.MemoryImportEnabled()
	default:
		return false
	}
}

// formatValue renders a resolved value for human output: bools and ints
// verbatim, strings as-is (empty as "(none)"), and slices as [a, b].
func formatValue(v any) string {
	switch t := v.(type) {
	case nil:
		return "(none)"
	case bool:
		return strconv.FormatBool(t)
	case int:
		return strconv.Itoa(t)
	case string:
		if t == "" {
			return "(none)"
		}
		return t
	case []string:
		if len(t) == 0 {
			return "[]"
		}
		return fmtList(t)
	default:
		return fmt.Sprintf("%v", v)
	}
}

func emptyDash(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}

// --- source detection ---

// fileKeySet parses the aide.json at path (if present) and returns the set of
// dotted leaf paths it explicitly contains. Used to report a key's source as
// `file` (project path) or `global` (~/.aide path). An empty path — e.g. when
// the global path can't be resolved — yields the empty set.
func fileKeySet(path string) map[string]bool {
	out := map[string]bool{}
	if path == "" {
		return out
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	var m map[string]any
	if json.Unmarshal(data, &m) != nil {
		return out
	}
	collectLeafKeys(m, "", out)
	return out
}

// collectLeafKeys records the dotted path of every non-map leaf in m.
func collectLeafKeys(m map[string]any, prefix string, out map[string]bool) {
	for k, v := range m {
		key := k
		if prefix != "" {
			key = prefix + "." + k
		}
		if child, ok := v.(map[string]any); ok {
			collectLeafKeys(child, key, out)
			continue
		}
		out[key] = true
	}
}

// keySource reports where a resolved key's value came from, in resolution-
// precedence order: `env` when a mapped AIDE_* var is set, then `file` when the
// dotted path is present in the project aide.json, then `global` when it is
// present in the ~/.aide global aide.json, else `default`. An env override is
// reported first because it sits at the top of precedence and is the value the
// user actually sees; below that, the project file shadows the global file just
// as buildKoanf layers them.
func keySource(key string, fileKeys, globalKeys map[string]bool) string {
	if name, ok := envOverride(key); ok {
		return "env (" + name + ")"
	}
	if fileKeys[key] {
		return "file"
	}
	if globalKeys[key] {
		return "global"
	}
	return "default"
}

// envOverride reverse-maps a dotted key to its AIDE_* env-var name and reports
// whether that variable is currently set. The forward mapping is the same one
// the loader uses (envToKey), so the reverse is the dotted key upper-cased with
// dots turned into underscores and the AIDE_ prefix prepended.
func envOverride(key string) (string, bool) {
	name := config.EnvPrefix + strings.ToUpper(strings.ReplaceAll(key, ".", "_"))
	if _, ok := os.LookupEnv(name); ok {
		return name, true
	}
	return "", false
}

// --- default snapshots ---

// defaultConfig builds a Config from defaults only (no file, no env) so the
// --all view can show each key's bare default alongside its resolved value. The
// env is not cleared here — callers that need a pristine default call this with
// no AIDE_* set; in practice the only defaulted *bool values come from the
// accessor methods, which read the zero-value struct, so env can't perturb them.
func defaultConfig() *config.Config {
	k := defaultKoanf()
	cfg := &config.Config{}
	_ = k.Unmarshal("", cfg)
	return cfg
}

// defaultKoanf resolves the defaults-only tree (empty project root means no
// file is read; the env layer still applies but defaults are what we read for
// the `default` column via keys the env doesn't set).
func defaultKoanf() *koanf.Koanf {
	k, err := config.Resolve("")
	if err != nil {
		return koanf.New(".")
	}
	return k
}

// --- arg helpers ---

// positionals returns the non-flag args in order (anything not starting with
// "--"). Config keys and values are positional; the only flags are --all/--json.
func positionals(args []string) []string {
	out := make([]string, 0, len(args))
	for _, a := range args {
		if strings.HasPrefix(a, "--") {
			continue
		}
		out = append(out, a)
	}
	return out
}

// unknownKeyError builds the standard error for a key not in the schema,
// pointing the user at the canonical key listing.
func unknownKeyError(key string) error {
	return fmt.Errorf("unknown config key %q (run 'aide config show --all' to list valid keys)", key)
}
