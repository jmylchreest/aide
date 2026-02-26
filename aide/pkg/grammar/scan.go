package grammar

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jmylchreest/aide/aide/pkg/aideignore"
)

// LanguageDetector is a function that detects the language of a file
// given its path and optional content. Returns empty string if unknown.
// This avoids an import cycle with the code package.
type LanguageDetector func(filePath string, content []byte) string

// ScanResult describes languages found in a project directory.
type ScanResult struct {
	// Languages maps language name to the number of files found.
	Languages map[string]int
	// TotalFiles is the total number of recognised source files scanned.
	TotalFiles int
	// Needed lists languages that have a grammar available but not installed locally.
	Needed []string
	// Unavailable lists languages detected in the project that have no grammar at all.
	Unavailable []string
}

// ScanProject walks a directory tree, detects languages using the provided
// detector, and reports which grammars are needed but not yet installed.
func ScanProject(root string, loader *CompositeLoader, detect LanguageDetector, ignore *aideignore.Matcher) (*ScanResult, error) {
	if ignore == nil {
		ignore = aideignore.NewFromDefaults()
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	shouldSkip := ignore.WalkFunc(absRoot)

	result := &ScanResult{
		Languages: make(map[string]int),
	}

	// Collect all available grammar names for lookup.
	availableSet := make(map[string]bool)
	for _, name := range loader.Available() {
		availableSet[name] = true
	}

	// Collect installed grammar names.
	installedSet := make(map[string]bool)
	for _, info := range loader.Installed() {
		installedSet[info.Name] = true
	}

	err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip errors
		}

		// Skip directories per aideignore.
		if info.IsDir() {
			skip, skipDir := shouldSkip(path, info)
			if skip || skipDir {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip non-regular files and ignored files.
		if !info.Mode().IsRegular() {
			return nil
		}
		if skip, _ := shouldSkip(path, info); skip {
			return nil
		}

		// Detect language by extension/filename.
		lang := detect(path, nil)
		if lang == "" {
			return nil
		}

		result.Languages[lang]++
		result.TotalFiles++
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Classify languages into needed vs unavailable.
	neededSet := make(map[string]bool)
	unavailableSet := make(map[string]bool)

	for lang := range result.Languages {
		if installedSet[lang] {
			continue // already installed (builtin or dynamic)
		}
		if availableSet[lang] {
			neededSet[lang] = true
		} else {
			unavailableSet[lang] = true
		}
	}

	result.Needed = sortedKeys(neededSet)
	result.Unavailable = sortedKeys(unavailableSet)
	return result, nil
}

// sortedKeys returns sorted keys from a bool map.
func sortedKeys(m map[string]bool) []string {
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// LanguageStatus describes the installation state of a language grammar.
type LanguageStatus struct {
	Name       string
	Files      int    // Number of files found in the project
	Status     string // "builtin", "installed", "available", "unavailable"
	CanInstall bool
}

// ScanDetail returns per-language status for a project scan.
// This is a higher-level view combining scan results with loader state.
func ScanDetail(root string, loader *CompositeLoader, detect LanguageDetector, ignore *aideignore.Matcher) ([]LanguageStatus, error) {
	scan, err := ScanProject(root, loader, detect, ignore)
	if err != nil {
		return nil, err
	}

	// Build status for each detected language.
	builtinSet := make(map[string]bool)
	for _, name := range loader.builtin.Names() {
		builtinSet[name] = true
	}

	installedDynamic := make(map[string]bool)
	for _, info := range loader.dynamic.Installed() {
		installedDynamic[info.Name] = true
	}

	_, availableDynamic := dynamicAvailable()

	var statuses []LanguageStatus
	for lang, count := range scan.Languages {
		status := LanguageStatus{
			Name:  lang,
			Files: count,
		}
		switch {
		case builtinSet[lang]:
			status.Status = "builtin"
		case installedDynamic[lang]:
			status.Status = "installed"
		case availableDynamic[lang]:
			status.Status = "available"
			status.CanInstall = true
		default:
			status.Status = "unavailable"
		}
		statuses = append(statuses, status)
	}

	// Sort: builtin first, then installed, then available, then unavailable.
	// Within each group, sort by file count descending.
	statusOrder := map[string]int{"builtin": 0, "installed": 1, "available": 2, "unavailable": 3}
	sort.Slice(statuses, func(i, j int) bool {
		oi, oj := statusOrder[statuses[i].Status], statusOrder[statuses[j].Status]
		if oi != oj {
			return oi < oj
		}
		return statuses[i].Files > statuses[j].Files
	})

	return statuses, nil
}

// dynamicAvailable returns sorted names and a set of all downloadable grammars.
func dynamicAvailable() ([]string, map[string]bool) {
	set := make(map[string]bool, len(DynamicGrammars))
	names := make([]string, 0, len(DynamicGrammars))
	for name := range DynamicGrammars {
		set[name] = true
		names = append(names, name)
	}
	sort.Strings(names)
	return names, set
}

// NormaliseLang converts common aliases to canonical language names
// (e.g., "ts" -> "typescript", "py" -> "python", "c++" -> "cpp").
func NormaliseLang(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	aliases := map[string]string{
		"ts":        "typescript",
		"tsx":       "typescript",
		"js":        "javascript",
		"jsx":       "javascript",
		"py":        "python",
		"rs":        "rust",
		"c++":       "cpp",
		"c#":        "csharp",
		"cs":        "csharp",
		"rb":        "ruby",
		"sh":        "bash",
		"shell":     "bash",
		"kt":        "kotlin",
		"ex":        "elixir",
		"exs":       "elixir",
		"ml":        "ocaml",
		"tf":        "hcl",
		"terraform": "hcl",
		"proto":     "protobuf",
		"yml":       "yaml",
	}
	if canonical, ok := aliases[s]; ok {
		return canonical
	}
	return s
}
