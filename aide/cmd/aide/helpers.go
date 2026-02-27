package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/jmylchreest/aide/aide/internal/version"
	"github.com/jmylchreest/aide/aide/pkg/grammar"
)

// fatal prints an error message and exits with code 1.
func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

// truncate shortens a string to n characters (runes) with ellipsis.
func truncate(s string, n int) string {
	if n < 4 {
		return s
	}
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n-3]) + "..."
}

// flagAliases maps British spelling flag prefixes to their canonical (American) forms.
// The CLI uses American spelling internally but accepts both.
var flagAliases = map[string]string{
	"--analyser=": "--analyzer=",
}

// parseFlag extracts a flag value from args (e.g., "--key=value").
// Also checks British spelling aliases defined in flagAliases.
func parseFlag(args []string, prefix string) string {
	for _, arg := range args {
		if strings.HasPrefix(arg, prefix) {
			return strings.TrimPrefix(arg, prefix)
		}
	}
	// Check British aliases: if an alias maps to our prefix, try the alias too.
	for alias, canonical := range flagAliases {
		if canonical == prefix {
			for _, arg := range args {
				if strings.HasPrefix(arg, alias) {
					return strings.TrimPrefix(arg, alias)
				}
			}
		}
	}
	return ""
}

// hasFlag checks if a flag is present in args.
func hasFlag(args []string, flag string) bool {
	for _, arg := range args {
		if arg == flag {
			return true
		}
	}
	return false
}

// titleCase capitalizes the first letter of each word.
func titleCase(s string) string {
	if s == "" {
		return s
	}
	words := strings.Fields(s)
	for i, word := range words {
		if len(word) > 0 {
			words[i] = strings.ToUpper(string(word[0])) + word[1:]
		}
	}
	return strings.Join(words, " ")
}

// findingsConfig holds analyser thresholds read from .aide/config/aide.json.
// Zero values mean "use default".
type findingsConfig struct {
	Complexity struct {
		Threshold int `json:"threshold"`
	} `json:"complexity"`
	Coupling struct {
		FanOut int `json:"fanOut"`
		FanIn  int `json:"fanIn"`
	} `json:"coupling"`
	Clones struct {
		WindowSize    int     `json:"windowSize"`
		MinLines      int     `json:"minLines"`
		MinMatchCount int     `json:"minMatchCount"`
		MaxBucketSize int     `json:"maxBucketSize"`
		MinSimilarity float64 `json:"minSimilarity"`
		MinSeverity   string  `json:"minSeverity"`
	} `json:"clones"`
}

// aideJSON is the top-level structure of .aide/config/aide.json.
type aideJSON struct {
	Findings findingsConfig `json:"findings"`
	Grammars grammarsConfig `json:"grammars"`
}

// grammarsConfig holds grammar-related settings from .aide/config/aide.json.
type grammarsConfig struct {
	// URL is the URL template for downloading grammar shared libraries.
	// Supported placeholders: {version}, {asset}, {name}, {os}, {arch}.
	// Defaults to DefaultGrammarURL if empty.
	URL string `json:"url"`
	// AutoDownload controls whether grammars are fetched automatically when
	// a language is encountered that has no local grammar. Default is true.
	// Set to false to require explicit `aide grammar install`.
	AutoDownload *bool `json:"autoDownload,omitempty"`
}

// loadFindingsConfig reads analyser thresholds from .aide/config/aide.json.
// Returns zero-valued config if the file does not exist or cannot be parsed.
func loadFindingsConfig(projectRoot string) findingsConfig {
	cfg := loadAideConfig(projectRoot)
	return cfg.Findings
}

// loadGrammarsConfig reads grammar settings from .aide/config/aide.json.
// Environment variables take precedence over the config file:
//   - AIDE_GRAMMAR_URL overrides the URL template.
//   - AIDE_GRAMMAR_AUTO_DOWNLOAD overrides auto-download ("0"/"false" to disable).
func loadGrammarsConfig(projectRoot string) grammarsConfig {
	cfg := loadAideConfig(projectRoot)
	// Environment variable overrides config file.
	if envURL := os.Getenv("AIDE_GRAMMAR_URL"); envURL != "" {
		cfg.Grammars.URL = envURL
	}
	if envAuto := os.Getenv("AIDE_GRAMMAR_AUTO_DOWNLOAD"); envAuto != "" {
		disabled := envAuto == "0" || strings.EqualFold(envAuto, "false")
		val := !disabled
		cfg.Grammars.AutoDownload = &val
	}
	return cfg.Grammars
}

// loadAideConfig reads .aide/config/aide.json.
// Returns zero-valued config if the file does not exist or cannot be parsed.
func loadAideConfig(projectRoot string) aideJSON {
	configPath := filepath.Join(projectRoot, ".aide", "config", "aide.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return aideJSON{}
	}
	var cfg aideJSON
	if err := json.Unmarshal(data, &cfg); err != nil {
		return aideJSON{}
	}
	return cfg
}

// projectRoot derives the project root from the database path.
// dbPath is <root>/.aide/memory/memory.db — three Dir() calls to reach <root>.
func projectRoot(dbPath string) string {
	return filepath.Dir(filepath.Dir(filepath.Dir(dbPath)))
}

// grammarDir returns the grammar storage directory for the project.
func grammarDir(dbPath string) string {
	return filepath.Join(projectRoot(dbPath), ".aide", "grammars")
}

// grammarVersion returns the version tag to use when downloading grammar assets.
// For release builds (e.g. Version="0.0.39") it returns the release tag "v0.0.39".
// For snapshot/dev builds it returns "snapshot".
func grammarVersion() string {
	if version.IsRelease() {
		return "v" + version.Version
	}
	return "snapshot"
}

// newGrammarLoader creates a CompositeLoader configured from aide.json and env
// vars. Auto-download is enabled by default and can be disabled via config
// ("grammars.autoDownload": false) or environment variable
// (AIDE_GRAMMAR_AUTO_DOWNLOAD=0).
//
// If logger is non-nil it is wired into the loader for grammar download/staleness
// logging. Pass nil to suppress all grammar log output.
//
// This is the standard factory for all parsing / indexing call sites. The
// grammar CLI subcommands override auto-download explicitly since they manage
// grammars interactively.
func newGrammarLoader(dbPath string, logger *log.Logger) *grammar.CompositeLoader {
	root := projectRoot(dbPath)
	cfg := loadGrammarsConfig(root)
	opts := []grammar.CompositeLoaderOption{
		grammar.WithGrammarDir(grammarDir(dbPath)),
		grammar.WithVersion(grammarVersion()),
	}
	if logger != nil {
		opts = append(opts, grammar.WithLogger(logger))
	}
	if cfg.URL != "" {
		opts = append(opts, grammar.WithBaseURL(cfg.URL))
	}
	// Respect config / env for auto-download. The NewCompositeLoader default
	// is true, so we only need to override when the config explicitly disables.
	if cfg.AutoDownload != nil {
		opts = append(opts, grammar.WithAutoDownload(*cfg.AutoDownload))
	}
	return grammar.NewCompositeLoader(opts...)
}

// newGrammarLoaderNoAuto creates a CompositeLoader with auto-download disabled.
// Used by grammar CLI subcommands that manage grammars explicitly.
// If logger is non-nil it is wired into the loader for grammar event logging.
func newGrammarLoaderNoAuto(dbPath string, logger *log.Logger) *grammar.CompositeLoader {
	root := projectRoot(dbPath)
	cfg := loadGrammarsConfig(root)
	opts := []grammar.CompositeLoaderOption{
		grammar.WithGrammarDir(grammarDir(dbPath)),
		grammar.WithAutoDownload(false),
		grammar.WithVersion(grammarVersion()),
	}
	if logger != nil {
		opts = append(opts, grammar.WithLogger(logger))
	}
	if cfg.URL != "" {
		opts = append(opts, grammar.WithBaseURL(cfg.URL))
	}
	return grammar.NewCompositeLoader(opts...)
}
