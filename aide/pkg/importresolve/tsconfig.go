package importresolve

import (
	"encoding/json"
	"os"
	"path"
	"strings"
)

// resolveAlias applies compilerOptions.paths from the nearest config whose
// directory contains fromFile. Per tsc semantics only one config governs a
// file, so matching stops at the first applicable config even if its rules
// miss.
func (t *tsResolver) resolveAlias(fromFile, imp string) string {
	for _, cfg := range t.configs {
		if cfg.dir != "" && !strings.HasPrefix(fromFile, cfg.dir+"/") {
			continue
		}
		for _, rule := range cfg.rules {
			var middle string
			if rule.exact {
				if imp != rule.prefix {
					continue
				}
			} else {
				if !strings.HasPrefix(imp, rule.prefix) || !strings.HasSuffix(imp, rule.suffix) ||
					len(imp) < len(rule.prefix)+len(rule.suffix) {
					continue
				}
				middle = imp[len(rule.prefix) : len(imp)-len(rule.suffix)]
			}
			for _, target := range rule.targets {
				cand := path.Join(cfg.baseURL, strings.Replace(target, "*", middle, 1))
				if u := t.probeModule(cand); u != "" {
					return u
				}
			}
		}
		return ""
	}
	return ""
}

// parseTSConfig extracts baseUrl + paths from a tsconfig/jsconfig file.
// Returns nil when the file has no paths mapping (nothing to resolve with).
func parseTSConfig(relDir, absPath string) *tsConfig {
	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil
	}
	var parsed struct {
		CompilerOptions struct {
			BaseURL string              `json:"baseUrl"`
			Paths   map[string][]string `json:"paths"`
		} `json:"compilerOptions"`
	}
	if json.Unmarshal(stripJSONC(data), &parsed) != nil {
		return nil
	}
	if len(parsed.CompilerOptions.Paths) == 0 {
		return nil
	}

	cfg := &tsConfig{
		dir:     relDir,
		baseURL: path.Join(relDir, parsed.CompilerOptions.BaseURL),
	}
	// Sort patterns longest-prefix-first: tsc picks the most specific match.
	patterns := make([]string, 0, len(parsed.CompilerOptions.Paths))
	for p := range parsed.CompilerOptions.Paths {
		patterns = append(patterns, p)
	}
	sortByPrefixSpecificity(patterns)
	for _, p := range patterns {
		rule := tsPathRule{targets: parsed.CompilerOptions.Paths[p]}
		if i := strings.Index(p, "*"); i >= 0 {
			rule.prefix, rule.suffix = p[:i], p[i+1:]
		} else {
			rule.prefix, rule.exact = p, true
		}
		cfg.rules = append(cfg.rules, rule)
	}
	return cfg
}

func sortByPrefixSpecificity(patterns []string) {
	prefixLen := func(p string) int {
		if i := strings.Index(p, "*"); i >= 0 {
			return i
		}
		return len(p) + 1 // exact patterns outrank any wildcard of equal length
	}
	for i := 1; i < len(patterns); i++ { // insertion sort: stable, deterministic, tiny inputs
		for j := i; j > 0; j-- {
			if prefixLen(patterns[j-1]) > prefixLen(patterns[j]) ||
				(prefixLen(patterns[j-1]) == prefixLen(patterns[j]) && patterns[j-1] <= patterns[j]) {
				break
			}
			patterns[j-1], patterns[j] = patterns[j], patterns[j-1]
		}
	}
}

// stripJSONC removes // and /* */ comments plus trailing commas — tsconfig
// files are JSONC, which encoding/json rejects. String contents (including
// escaped quotes) are preserved untouched.
func stripJSONC(data []byte) []byte {
	out := make([]byte, 0, len(data))
	inStr, esc := false, false
	for i := 0; i < len(data); i++ {
		c := data[i]
		if inStr {
			out = append(out, c)
			switch {
			case esc:
				esc = false
			case c == '\\':
				esc = true
			case c == '"':
				inStr = false
			}
			continue
		}
		switch {
		case c == '"':
			inStr = true
			out = append(out, c)
		case c == '/' && i+1 < len(data) && data[i+1] == '/':
			for i < len(data) && data[i] != '\n' {
				i++
			}
			if i < len(data) {
				out = append(out, '\n')
			}
		case c == '/' && i+1 < len(data) && data[i+1] == '*':
			i += 2
			for i+1 < len(data) && (data[i] != '*' || data[i+1] != '/') {
				i++
			}
			i++
		case c == ',':
			// Trailing comma: drop if the next non-whitespace is } or ].
			j := i + 1
			for j < len(data) && (data[j] == ' ' || data[j] == '\t' || data[j] == '\n' || data[j] == '\r') {
				j++
			}
			if j < len(data) && (data[j] == '}' || data[j] == ']') {
				continue
			}
			out = append(out, c)
		default:
			out = append(out, c)
		}
	}
	return out
}
