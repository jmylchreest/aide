package survey

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/jmylchreest/aide/aide/pkg/grammar"
)

// TopologyResult holds the output of the topology analyzer.
type TopologyResult struct {
	Entries []*Entry
}

// RunTopology detects repo structure by filesystem inspection.
// It iterates the project markers from the default PackRegistry, walks/stats
// the filesystem according to each marker's check type, and emits survey
// entries for each discovery. Fully data-driven from packs/index.json.
func RunTopology(rootDir string) (*TopologyResult, error) {
	return RunTopologyWithRegistry(rootDir, grammar.DefaultPackRegistry())
}

// RunTopologyWithRegistry is like RunTopology but accepts a specific PackRegistry.
// Useful for testing with custom marker sets.
func RunTopologyWithRegistry(rootDir string, reg *grammar.PackRegistry) (*TopologyResult, error) {
	result := &TopologyResult{}
	markers := reg.ProjectMarkers()

	// Phase 1: Process file and directory markers.
	// Track where file markers are found so sibling markers can reference them.
	// Key: marker filename, Value: list of directories where it was found.
	foundLocations := make(map[string][]string)

	for i := range markers {
		m := &markers[i]
		switch m.Check {
		case grammar.MarkerCheckFile:
			result.processFileMarker(rootDir, m, foundLocations)
		case grammar.MarkerCheckDirectory:
			result.processDirectoryMarker(rootDir, m)
		}
	}

	// Phase 2: Process sibling markers using found locations from Phase 1.
	for i := range markers {
		m := &markers[i]
		if m.Check == grammar.MarkerCheckSibling {
			result.processSiblingMarker(rootDir, m, foundLocations)
		}
	}

	AnnotateEstTokens(rootDir, result.Entries)
	return result, nil
}

// processFileMarker walks the filesystem looking for a file with the marker's name,
// up to the specified MaxDepth. For each found instance, it emits a survey entry
// and records the location for sibling marker resolution.
func (r *TopologyResult) processFileMarker(rootDir string, m *grammar.ProjectMarker, foundLocs map[string][]string) {
	// Build skip set for this marker.
	skipSet := make(map[string]bool, len(m.SkipPaths))
	for _, sp := range m.SkipPaths {
		skipSet[sp] = true
	}

	walkForFile(rootDir, m.File, m.MaxDepth, skipSet, func(path string) {
		dir := filepath.Dir(path)
		relPath, _ := filepath.Rel(rootDir, dir)
		if relPath == "." {
			relPath = ""
		}

		// Record location for sibling resolution.
		foundLocs[m.File] = append(foundLocs[m.File], dir)

		// Determine entry name.
		name := m.ResolvedName()

		// Apply parse directive if present.
		if m.Parse != nil {
			if parsed := parseFileContent(path, m.Parse); parsed != "" {
				name = parsed
			}
		}

		// For module entries without a parsed name, use the directory basename.
		if m.Kind == KindModule && m.Parse == nil {
			dirName := filepath.Base(dir)
			if relPath == "" {
				dirName = filepath.Base(rootDir)
			}
			name = dirName
		}

		// Build metadata — start with marker's declared metadata, then enrich.
		metadata := make(map[string]string)
		for k, v := range m.Metadata {
			metadata[k] = v
		}
		if _, ok := metadata["marker"]; !ok {
			metadata["marker"] = m.File
		}
		if m.Pack != "" {
			if _, ok := metadata["language"]; !ok {
				metadata["language"] = m.Pack
			}
		}

		title := buildTitle(m, name, relPath)

		r.Entries = append(r.Entries, &Entry{
			Analyzer: AnalyzerTopology,
			Kind:     m.Kind,
			Name:     name,
			FilePath: relPath,
			Title:    title,
			Metadata: metadata,
		})
	})
}

// processDirectoryMarker checks for a directory at root and emits a survey entry
// if the directory exists and is non-empty.
func (r *TopologyResult) processDirectoryMarker(rootDir string, m *grammar.ProjectMarker) {
	fullPath := filepath.Join(rootDir, m.File)
	fi, err := os.Stat(fullPath)
	if err != nil || !fi.IsDir() {
		return
	}

	// Check directory is non-empty.
	entries, err := os.ReadDir(fullPath)
	if err != nil || len(entries) == 0 {
		return
	}

	name := m.ResolvedName()
	metadata := make(map[string]string)
	for k, v := range m.Metadata {
		metadata[k] = v
	}

	r.Entries = append(r.Entries, &Entry{
		Analyzer: AnalyzerTopology,
		Kind:     m.Kind,
		Name:     name,
		FilePath: m.File,
		Title:    buildTitle(m, name, ""),
		Metadata: metadata,
	})
}

// processSiblingMarker checks for a sibling file next to each location where
// the parent marker (SiblingOf) was found.
func (r *TopologyResult) processSiblingMarker(rootDir string, m *grammar.ProjectMarker, foundLocs map[string][]string) {
	parentDirs := foundLocs[m.SiblingOf]
	for _, dir := range parentDirs {
		siblingPath := filepath.Join(dir, m.File)
		if _, err := os.Stat(siblingPath); err != nil {
			continue
		}

		relPath, _ := filepath.Rel(rootDir, dir)
		if relPath == "." {
			relPath = ""
		}

		name := m.ResolvedName()

		// Apply parse if present.
		if m.Parse != nil {
			if parsed := parseFileContent(siblingPath, m.Parse); parsed != "" {
				name = parsed
			}
		}

		metadata := make(map[string]string)
		for k, v := range m.Metadata {
			metadata[k] = v
		}
		if m.Pack != "" {
			if _, ok := metadata["language"]; !ok {
				metadata["language"] = m.Pack
			}
		}
		if _, ok := metadata["build_system"]; !ok && m.Kind == KindWorkspace {
			metadata["build_system"] = m.File
		}

		r.Entries = append(r.Entries, &Entry{
			Analyzer: AnalyzerTopology,
			Kind:     m.Kind,
			Name:     name,
			FilePath: relPath,
			Title:    buildTitle(m, name, relPath),
			Metadata: metadata,
		})
	}
}

// buildTitle generates a human-readable title for a survey entry based on
// the marker definition.
func buildTitle(m *grammar.ProjectMarker, name, relPath string) string {
	switch m.Kind {
	case KindModule:
		lang := m.Pack
		if lang == "" {
			lang = m.Label
		}
		return fmt.Sprintf("%s module: %s", capitalise(lang), name)
	case KindWorkspace:
		tool := m.ResolvedName()
		if relPath != "" {
			return fmt.Sprintf("%s workspace at %s", capitalise(tool), relPath)
		}
		return fmt.Sprintf("Monorepo tool: %s", tool)
	case KindTechStack:
		displayName := m.ResolvedName()
		if bs, ok := m.Metadata["build_system"]; ok {
			return fmt.Sprintf("Build system: %s", bs)
		}
		if ci, ok := m.Metadata["ci_cd"]; ok {
			return fmt.Sprintf("CI/CD: %s", ci)
		}
		return fmt.Sprintf("%s detected", capitalise(displayName))
	default:
		return fmt.Sprintf("%s: %s", m.Kind, name)
	}
}

// capitalise returns the string with its first letter upper-cased.
func capitalise(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// walkForFile walks the directory tree up to maxDepth levels looking for files
// with the given name. Skips hidden directories, common vendor dirs, and any
// directories in the skipSet.
// maxDepth: 0 = root only, positive = limit, -1 = unlimited.
func walkForFile(rootDir, fileName string, maxDepth int, skipSet map[string]bool, fn func(path string)) {
	_ = filepath.WalkDir(rootDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		rel, _ := filepath.Rel(rootDir, path)
		depth := strings.Count(rel, string(filepath.Separator))

		// Apply maxDepth guard: -1 means unlimited.
		if maxDepth >= 0 && depth > maxDepth {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if d.IsDir() {
			base := d.Name()
			// Skip hidden directories (but not the root ".").
			if base != "." && strings.HasPrefix(base, ".") {
				return filepath.SkipDir
			}
			if base == "node_modules" || base == "vendor" || base == "__pycache__" || base == "target" {
				return filepath.SkipDir
			}
			if skipSet[base] {
				return filepath.SkipDir
			}
		}

		if !d.IsDir() && d.Name() == fileName {
			fn(path)
		}

		return nil
	})
}

// parseFileContent applies a MarkerParse regex to extract a value from file content.
func parseFileContent(path string, p *grammar.MarkerParse) string {
	re, err := regexp.Compile(p.Regex)
	if err != nil {
		return ""
	}

	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		matches := re.FindStringSubmatch(scanner.Text())
		if len(matches) > p.Group {
			return matches[p.Group]
		}
	}
	return ""
}

// parseGoModuleName reads the module directive from a go.mod file.
// Kept for backward compatibility with tests.
func parseGoModuleName(goModPath string) string {
	f, err := os.Open(goModPath)
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module"))
		}
	}
	return ""
}

// parseJSONField does a quick scan for a "name": "value" pattern in a JSON file.
// Kept for backward compatibility with tests.
func parseJSONField(path, field string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	target := fmt.Sprintf(`"%s"`, field)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.Contains(line, target) {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) < 2 {
				continue
			}
			val := strings.TrimSpace(parts[1])
			val = strings.Trim(val, `",`)
			if val != "" {
				return val
			}
		}
	}
	return ""
}
