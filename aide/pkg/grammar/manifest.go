package grammar

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Manifest tracks installed dynamic grammars in the grammar cache directory.
type Manifest struct {
	AideVersion string                    `json:"aide_version"`
	AbiVersion  uint32                    `json:"abi_version"`
	Grammars    map[string]*ManifestEntry `json:"grammars"`
}

// ManifestEntry describes a single installed dynamic grammar.
type ManifestEntry struct {
	Version     string    `json:"version"`
	File        string    `json:"file"`
	SHA256      string    `json:"sha256"`
	CSymbol     string    `json:"c_symbol"`
	InstalledAt time.Time `json:"installed_at"`
}

// manifestStore handles reading/writing the manifest.json file.
type manifestStore struct {
	mu   sync.RWMutex
	dir  string
	data *Manifest
}

func newManifestStore(dir string) *manifestStore {
	return &manifestStore{
		dir: dir,
		data: &Manifest{
			Grammars: make(map[string]*ManifestEntry),
		},
	}
}

// manifestPath returns the full path to manifest.json.
func (ms *manifestStore) manifestPath() string {
	return filepath.Join(ms.dir, "manifest.json")
}

// load reads the manifest from disk. If it doesn't exist, returns an empty manifest.
func (ms *manifestStore) load() error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	data, err := os.ReadFile(ms.manifestPath())
	if err != nil {
		if os.IsNotExist(err) {
			ms.data = &Manifest{
				Grammars: make(map[string]*ManifestEntry),
			}
			return nil
		}
		return err
	}

	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	if m.Grammars == nil {
		m.Grammars = make(map[string]*ManifestEntry)
	}
	ms.data = &m
	return nil
}

// save writes the manifest to disk.
func (ms *manifestStore) save() error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	if err := os.MkdirAll(ms.dir, 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(ms.data, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(ms.manifestPath(), data, 0o644)
}

// get returns the manifest entry for a grammar, or nil if not installed.
func (ms *manifestStore) get(name string) *ManifestEntry {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	return ms.data.Grammars[name]
}

// set adds or updates a grammar entry in the manifest.
func (ms *manifestStore) set(name string, entry *ManifestEntry) {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	ms.data.Grammars[name] = entry
}

// setAideVersion records the aide version that last modified the manifest.
func (ms *manifestStore) setAideVersion(v string) {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	if v != "" {
		ms.data.AideVersion = v
	}
}

// remove deletes a grammar entry from the manifest.
func (ms *manifestStore) remove(name string) {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	delete(ms.data.Grammars, name)
}

// entries returns all manifest entries.
func (ms *manifestStore) entries() map[string]*ManifestEntry {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	// Return a copy
	result := make(map[string]*ManifestEntry, len(ms.data.Grammars))
	for k, v := range ms.data.Grammars {
		result[k] = v
	}
	return result
}
