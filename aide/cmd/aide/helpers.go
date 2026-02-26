package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// fatal prints an error message and exits with code 1.
func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

// truncate shortens a string to n characters with ellipsis.
func truncate(s string, n int) string {
	if n < 4 {
		return s
	}
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
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
		WindowSize int `json:"windowSize"`
		MinLines   int `json:"minLines"`
	} `json:"clones"`
}

// aideJSON is the top-level structure of .aide/config/aide.json.
type aideJSON struct {
	Findings findingsConfig `json:"findings"`
}

// loadFindingsConfig reads analyser thresholds from .aide/config/aide.json.
// Returns zero-valued config if the file does not exist or cannot be parsed.
func loadFindingsConfig(projectRoot string) findingsConfig {
	configPath := filepath.Join(projectRoot, ".aide", "config", "aide.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return findingsConfig{}
	}
	var cfg aideJSON
	if err := json.Unmarshal(data, &cfg); err != nil {
		return findingsConfig{}
	}
	return cfg.Findings
}
