// Package store provides storage backends for aide.
// This file implements a combined store that writes to both bbolt and bleve search.
package store

import (
	"log"

	"github.com/jmylchreest/aide/aide/pkg/memory"
)

// CombinedStore wraps both BoltStore and SearchStore for consistent memory operations.
type CombinedStore struct {
	bolt   *BoltStore
	search *SearchStore
}

// NewCombinedStore creates a store that writes memories to both bbolt and search index.
func NewCombinedStore(dbPath string) (*CombinedStore, error) {
	bolt, err := NewBoltStore(dbPath)
	if err != nil {
		return nil, err
	}

	// Search index path is alongside the bolt db
	searchPath := GetSearchPath(dbPath)
	search, err := NewSearchStore(SearchConfig{Path: searchPath})
	if err != nil {
		bolt.Close()
		return nil, err
	}

	cs := &CombinedStore{
		bolt:   bolt,
		search: search,
	}

	if err := cs.ensureSearchMapping(); err != nil {
		search.Close()
		bolt.Close()
		return nil, err
	}

	return cs, nil
}

// ensureSearchMapping checks if the search index mapping has changed and rebuilds if needed.
func (c *CombinedStore) ensureSearchMapping() error {
	m, err := buildIndexMapping()
	if err != nil {
		return err
	}
	hash := MappingHash(m)
	stored, err := c.bolt.GetMeta("search_mapping_hash")
	if err == nil && hash == stored {
		return nil
	}

	// First time or mapping changed — rebuild.
	if err == nil {
		log.Printf("store: search mapping changed, rebuilding index")
	}
	if err := c.search.Clear(); err != nil {
		return err
	}
	if err := c.SyncSearchIndex(); err != nil {
		return err
	}
	return c.bolt.SetMeta("search_mapping_hash", hash)
}

// Close closes both stores.
func (c *CombinedStore) Close() error {
	c.search.Close()
	return c.bolt.Close()
}

// Bolt returns the underlying BoltStore for non-memory operations.
func (c *CombinedStore) Bolt() *BoltStore {
	return c.bolt
}

// --- Memory Operations (dual-write) ---

// AddMemory stores a memory in both bbolt and search index.
func (c *CombinedStore) AddMemory(m *memory.Memory) error {
	if err := c.bolt.AddMemory(m); err != nil {
		return err
	}
	// Search index failure is non-fatal - log but continue
	_ = c.search.IndexMemory(m)
	return nil
}

// DeleteMemory removes a memory from both stores.
func (c *CombinedStore) DeleteMemory(id string) error {
	if err := c.bolt.DeleteMemory(id); err != nil {
		return err
	}
	_ = c.search.DeleteMemory(id)
	return nil
}

// ListMemories returns memories from bbolt (source of truth).
func (c *CombinedStore) ListMemories(opts memory.SearchOptions) ([]*memory.Memory, error) {
	return c.bolt.ListMemories(opts)
}

// GetMemory retrieves a memory by ID from bbolt.
func (c *CombinedStore) GetMemory(id string) (*memory.Memory, error) {
	return c.bolt.GetMemory(id)
}

// SelectMemories performs substring search on bbolt (exact matching).
func (c *CombinedStore) SelectMemories(query string, limit int) ([]*memory.Memory, error) {
	return c.bolt.SearchMemories(query, limit)
}

// SearchMemories performs full-text search using bleve (fuzzy, ngram, edge-ngram).
func (c *CombinedStore) SearchMemories(query string, limit int) ([]SearchResult, error) {
	results, err := c.search.Search(query, limit)
	if err != nil {
		// Fall back to substring search
		memories, subErr := c.bolt.SearchMemories(query, limit)
		if subErr != nil {
			return nil, err // Return original search error
		}
		// Convert to SearchResult format
		fallback := make([]SearchResult, len(memories))
		for i, m := range memories {
			fallback[i] = SearchResult{
				ID:       m.ID,
				Content:  m.Content,
				Category: string(m.Category),
				Score:    0.0, // No score for substring match
				Memory:   m,
			}
		}
		return fallback, nil
	}

	// Enrich results with full memory data
	for i := range results {
		if m, err := c.bolt.GetMemory(results[i].ID); err == nil {
			results[i].Memory = m
		}
	}

	return results, nil
}

// SyncSearchIndex rebuilds the search index from bbolt (for recovery/migration).
func (c *CombinedStore) SyncSearchIndex() error {
	memories, err := c.bolt.ListMemories(memory.SearchOptions{})
	if err != nil {
		return err
	}
	return c.search.Reindex(memories)
}

// SearchCount returns the number of documents in the search index.
func (c *CombinedStore) SearchCount() (uint64, error) {
	return c.search.Count()
}

// ClearMemories removes all memories from both stores.
func (c *CombinedStore) ClearMemories() (int, error) {
	count, err := c.bolt.ClearMemories()
	if err != nil {
		return count, err
	}
	_ = c.search.Clear()
	return count, nil
}
