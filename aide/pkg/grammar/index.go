package grammar

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

// PackIndex holds the declarative project marker index.
// It maps filesystem markers (files/directories) to grammar packs or standalone labels,
// enabling data-driven project detection for both topology analysis and grammar scanning.
type PackIndex struct {
	SchemaVersion   int              `json:"schema_version"`
	ProjectMarkers  []ProjectMarker  `json:"project_markers,omitempty"`
	ConsumerFormats []ConsumerFormat `json:"consumer_formats,omitempty"`
}

// ConsumerFormat describes a file format that is not parsed by tree-sitter
// (no grammar pack, no symbol extraction) but DOES consume code from other
// files — typically templating or composition formats. Reference verifiers
// scan these as plain text to catch use sites the index cannot capture.
//
// Examples: .astro/.svelte/.vue templates that reference React/Vue components
// by name, .mdx files embedding JSX, .stories.* files importing components.
type ConsumerFormat struct {
	// Extensions list the file extensions this format applies to (".astro").
	Extensions []string `json:"extensions"`
	// Label is a short tag identifying the format (e.g. "astro", "svelte").
	// Used as the merge/override key.
	Label string `json:"label"`
	// Description is human-readable context about what this format does.
	Description string `json:"description,omitempty"`
}

// ProjectMarker maps a filesystem marker to a pack or standalone label.
// Each marker describes a file or directory whose presence indicates a particular
// technology, language, build system, CI/CD pipeline, or other project characteristic.
type ProjectMarker struct {
	// Detection — what to look for on the filesystem.

	// File is the marker filename or relative path (e.g., "go.mod", ".github/workflows").
	File string `json:"file"`
	// Check is the detection mode: "file", "directory", or "sibling".
	//   - "file": walk filesystem up to MaxDepth looking for this filename.
	//   - "directory": stat for a directory (with non-empty check).
	//   - "sibling": only checked relative to a found instance of SiblingOf.
	Check string `json:"check"`
	// MaxDepth controls walk depth: 0 = root only (default), positive = limit, -1 = unlimited.
	MaxDepth int `json:"max_depth,omitempty"`

	// SiblingOf is the parent marker file this is relative to (only when Check == "sibling").
	// For example, "go.work" is a sibling of "go.mod".
	SiblingOf string `json:"sibling_of,omitempty"`

	// Resolution — exactly one of Pack or Label must be set.

	// Pack links to an existing grammar pack by name (e.g., "go", "rust", "php").
	// When set, finding this marker implies the grammar for this pack may be needed.
	Pack string `json:"pack,omitempty"`
	// Label is a standalone name for non-language tools (e.g., "make", "task", "github-actions").
	// Used when no grammar pack exists for this technology.
	Label string `json:"label,omitempty"`

	// Kind classifies the survey entry: "module", "workspace", "tech_stack".
	Kind string `json:"kind"`

	// Optional enrichment.

	// SkipPaths lists directory names to skip during filesystem walk (e.g., ["node_modules"]).
	SkipPaths []string `json:"skip_paths,omitempty"`
	// Metadata holds passthrough key-value pairs for survey.Entry.Metadata
	// (e.g., {"build_system": "make"}, {"ci_cd": "github-actions"}).
	Metadata map[string]string `json:"metadata,omitempty"`
	// Parse defines how to extract a value (e.g., module name) from the marker file's content.
	Parse *MarkerParse `json:"parse,omitempty"`
}

// MarkerParse defines how to extract a value from a marker file's content.
type MarkerParse struct {
	// Regex is a regular expression with at least one capture group.
	Regex string `json:"regex"`
	// Group is the 1-based capture group index to extract.
	Group int `json:"group"`
	// Field is the target field on the survey entry: "name".
	Field string `json:"field"`
}

// MarkerCheckFile is the check type for regular file markers.
const MarkerCheckFile = "file"

// MarkerCheckDirectory is the check type for directory markers.
const MarkerCheckDirectory = "directory"

// MarkerCheckSibling is the check type for markers checked relative to another marker.
const MarkerCheckSibling = "sibling"

// markerKey returns a composite key used for merge/override deduplication.
func (m *ProjectMarker) markerKey() string {
	return m.File + "\x00" + m.Kind
}

// ResolvedName returns the display name for a marker — the Pack name if linked
// to a grammar pack, otherwise the Label.
func (m *ProjectMarker) ResolvedName() string {
	if m.Pack != "" {
		return m.Pack
	}
	return m.Label
}

// indexState holds the loaded project markers and consumer formats,
// protected by its own mutex to avoid holding the main PackRegistry lock
// during index operations.
type indexState struct {
	mu        sync.RWMutex
	markers   []ProjectMarker
	consumers []ConsumerFormat
}

// LoadIndex parses a PackIndex from raw JSON and merges with existing markers.
// Markers from the new index override existing ones with the same (File, Kind) key.
func (r *PackRegistry) LoadIndex(data []byte) error {
	var idx PackIndex
	if err := json.Unmarshal(data, &idx); err != nil {
		return fmt.Errorf("parsing pack index: %w", err)
	}
	r.idx.mu.Lock()
	defer r.idx.mu.Unlock()

	// Build lookup of existing markers by key for merge.
	existing := make(map[string]int, len(r.idx.markers))
	for i, m := range r.idx.markers {
		existing[m.markerKey()] = i
	}

	for _, newMarker := range idx.ProjectMarkers {
		key := newMarker.markerKey()
		if i, ok := existing[key]; ok {
			// Override existing entry.
			r.idx.markers[i] = newMarker
		} else {
			// Append new entry.
			r.idx.markers = append(r.idx.markers, newMarker)
			existing[key] = len(r.idx.markers) - 1
		}
	}

	// Consumer formats merge by Label (the unique tag).
	existingConsumers := make(map[string]int, len(r.idx.consumers))
	for i, c := range r.idx.consumers {
		existingConsumers[c.Label] = i
	}
	for _, newConsumer := range idx.ConsumerFormats {
		if newConsumer.Label == "" {
			continue
		}
		if i, ok := existingConsumers[newConsumer.Label]; ok {
			r.idx.consumers[i] = newConsumer
		} else {
			r.idx.consumers = append(r.idx.consumers, newConsumer)
			existingConsumers[newConsumer.Label] = len(r.idx.consumers) - 1
		}
	}

	return nil
}

// LoadIndexFromFile loads a PackIndex from a file path, merging with existing markers.
func (r *PackRegistry) LoadIndexFromFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return r.LoadIndex(data)
}

// ProjectMarkers returns a copy of all registered project markers.
func (r *PackRegistry) ProjectMarkers() []ProjectMarker {
	r.idx.mu.RLock()
	defer r.idx.mu.RUnlock()
	result := make([]ProjectMarker, len(r.idx.markers))
	copy(result, r.idx.markers)
	return result
}

// MarkersForPack returns project markers that link to the named grammar pack.
func (r *PackRegistry) MarkersForPack(packName string) []ProjectMarker {
	r.idx.mu.RLock()
	defer r.idx.mu.RUnlock()
	var result []ProjectMarker
	for _, m := range r.idx.markers {
		if m.Pack == packName {
			result = append(result, m)
		}
	}
	return result
}

// ConsumerFormats returns a copy of all registered consumer formats.
func (r *PackRegistry) ConsumerFormats() []ConsumerFormat {
	r.idx.mu.RLock()
	defer r.idx.mu.RUnlock()
	result := make([]ConsumerFormat, len(r.idx.consumers))
	copy(result, r.idx.consumers)
	return result
}

// ConsumerExtensions returns a flat list of every extension declared by any
// registered consumer format. Used by reference verifiers to walk additional
// files beyond the parsed code-index set.
func (r *PackRegistry) ConsumerExtensions() []string {
	r.idx.mu.RLock()
	defer r.idx.mu.RUnlock()
	var out []string
	for _, c := range r.idx.consumers {
		out = append(out, c.Extensions...)
	}
	return out
}

// PackLinkedMarkers returns all project markers that link to any grammar pack
// (i.e., markers with a non-empty Pack field). These are the markers that can
// trigger grammar download detection.
func (r *PackRegistry) PackLinkedMarkers() []ProjectMarker {
	r.idx.mu.RLock()
	defer r.idx.mu.RUnlock()
	var result []ProjectMarker
	for _, m := range r.idx.markers {
		if m.Pack != "" {
			result = append(result, m)
		}
	}
	return result
}
