// Package blueprint provides loading, validation, and resolution of best-practice
// decision bundles ("blueprints") that seed a project's decision store.
package blueprint

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

//go:embed blueprints/*.json
var embeddedBlueprints embed.FS

// Blueprint is a portable bundle of best-practice decisions for a language or stack.
type Blueprint struct {
	SchemaVersion int                  `json:"schema_version"`
	Name          string               `json:"name"`
	DisplayName   string               `json:"display_name"`
	Description   string               `json:"description"`
	Version       string               `json:"version"`
	Tags          []string             `json:"tags,omitempty"`
	Includes      []string             `json:"includes,omitempty"`
	Decisions     []BlueprintDecision  `json:"decisions"`
}

// BlueprintDecision is a single decision within a blueprint.
type BlueprintDecision struct {
	Topic      string   `json:"topic"`
	Decision   string   `json:"decision"`
	Rationale  string   `json:"rationale"`
	Details    string   `json:"details,omitempty"`
	References []string `json:"references,omitempty"`
}

// ImportResult summarises the outcome of importing a blueprint.
type ImportResult struct {
	BlueprintName string
	Imported      int
	Skipped       int
	SkippedTopics []string
}

// Validate checks that a blueprint has all required fields and no duplicate topics.
func (bp *Blueprint) Validate() error {
	if bp.SchemaVersion != 1 {
		return fmt.Errorf("unsupported schema_version: %d (expected 1)", bp.SchemaVersion)
	}
	if bp.Name == "" {
		return fmt.Errorf("missing required field: name")
	}
	if bp.DisplayName == "" {
		return fmt.Errorf("missing required field: display_name")
	}
	if bp.Description == "" {
		return fmt.Errorf("missing required field: description")
	}
	if bp.Version == "" {
		return fmt.Errorf("missing required field: version")
	}

	seen := make(map[string]bool, len(bp.Decisions))
	for i, d := range bp.Decisions {
		if d.Topic == "" {
			return fmt.Errorf("decision %d: missing required field: topic", i)
		}
		if d.Decision == "" {
			return fmt.Errorf("decision %d (%s): missing required field: decision", i, d.Topic)
		}
		if d.Rationale == "" {
			return fmt.Errorf("decision %d (%s): missing required field: rationale", i, d.Topic)
		}
		if seen[d.Topic] {
			return fmt.Errorf("duplicate topic: %s", d.Topic)
		}
		seen[d.Topic] = true
	}
	return nil
}

// LoadEmbedded loads a blueprint by name from the embedded filesystem.
func LoadEmbedded(name string) (*Blueprint, error) {
	return loadFromFS(embeddedBlueprints, name)
}

// LoadFromDir loads a blueprint by name from a directory on disk.
// Returns os.ErrNotExist if the file does not exist.
func LoadFromDir(dir, name string) (*Blueprint, error) {
	path := filepath.Join(dir, name+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parseAndValidate(data, path)
}

// LoadFromFile loads a blueprint from an explicit file path.
func LoadFromFile(path string) (*Blueprint, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parseAndValidate(data, path)
}

// LoadFromURL loads a blueprint from a remote URL.
func LoadFromURL(url string) (*Blueprint, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch %s: HTTP %d", url, resp.StatusCode)
	}

	var bp Blueprint
	if err := json.NewDecoder(resp.Body).Decode(&bp); err != nil {
		return nil, fmt.Errorf("parse %s: %w", url, err)
	}
	if err := bp.Validate(); err != nil {
		return nil, fmt.Errorf("validate %s: %w", url, err)
	}
	return &bp, nil
}

// LoadFromRegistry tries to load a blueprint from a registry base URL.
// A registry is just a URL prefix; it fetches {baseURL}/{name}.json.
func LoadFromRegistry(baseURL, name string) (*Blueprint, error) {
	url := strings.TrimRight(baseURL, "/") + "/" + name + ".json"
	return LoadFromURL(url)
}

// ListEmbedded returns metadata for all embedded blueprints.
func ListEmbedded() ([]*Blueprint, error) {
	entries, err := fs.ReadDir(embeddedBlueprints, "blueprints")
	if err != nil {
		return nil, err
	}

	var blueprints []*Blueprint
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".json")
		bp, err := LoadEmbedded(name)
		if err != nil {
			return nil, fmt.Errorf("load embedded %s: %w", name, err)
		}
		blueprints = append(blueprints, bp)
	}
	return blueprints, nil
}

// Resolve loads a blueprint by name using the resolution chain:
// 1. Local override directory (.aide/blueprints/)
// 2. Embedded in the binary
// 3. Remote registries (in order)
//
// Returns the blueprint and its source description.
func Resolve(name, localDir string, registries []string) (*Blueprint, string, error) {
	// 1. Local override
	if localDir != "" {
		bp, err := LoadFromDir(localDir, name)
		if err == nil {
			return bp, "local: " + localDir, nil
		}
		if !os.IsNotExist(err) {
			return nil, "", fmt.Errorf("local override %s: %w", name, err)
		}
	}

	// 2. Embedded
	bp, err := LoadEmbedded(name)
	if err == nil {
		return bp, "embedded", nil
	}

	// 3. Registries
	for _, reg := range registries {
		bp, err := LoadFromRegistry(reg, name)
		if err == nil {
			return bp, "registry: " + reg, nil
		}
	}

	return nil, "", fmt.Errorf("blueprint not found: %s", name)
}

// ResolveWithIncludes loads a blueprint and all its transitive includes,
// returning them in topological order (includes first, root last).
// Detects and rejects cycles.
func ResolveWithIncludes(name, localDir string, registries []string) ([]*Blueprint, error) {
	var result []*Blueprint
	visited := make(map[string]bool)
	inStack := make(map[string]bool)

	var visit func(string) error
	visit = func(n string) error {
		if visited[n] {
			return nil
		}
		if inStack[n] {
			return fmt.Errorf("cycle detected: %s includes itself (directly or transitively)", n)
		}
		inStack[n] = true

		bp, _, err := Resolve(n, localDir, registries)
		if err != nil {
			return err
		}

		for _, inc := range bp.Includes {
			if err := visit(inc); err != nil {
				return err
			}
		}

		delete(inStack, n)
		visited[n] = true
		result = append(result, bp)
		return nil
	}

	if err := visit(name); err != nil {
		return nil, err
	}
	return result, nil
}

// loadFromFS loads a blueprint from an fs.FS.
func loadFromFS(fsys fs.FS, name string) (*Blueprint, error) {
	path := "blueprints/" + name + ".json"
	data, err := fs.ReadFile(fsys, path)
	if err != nil {
		return nil, err
	}
	return parseAndValidate(data, path)
}

// parseAndValidate unmarshals and validates a blueprint.
func parseAndValidate(data []byte, source string) (*Blueprint, error) {
	var bp Blueprint
	if err := json.Unmarshal(data, &bp); err != nil {
		return nil, fmt.Errorf("parse %s: %w", source, err)
	}
	if err := bp.Validate(); err != nil {
		return nil, fmt.Errorf("validate %s: %w", source, err)
	}
	return &bp, nil
}
