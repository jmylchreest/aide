// Package store provides storage backends for aide.
// This file implements a combined store that writes to both bbolt and bleve search.
// CombinedStore implements the Store interface, delegating non-memory operations
// to BoltStore and adding bleve full-text search for memory operations.
package store

import (
	"log"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/memory"
)

// Verify CombinedStore implements Store at compile time.
var _ Store = (*CombinedStore)(nil)

// CombinedStore wraps both BoltStore and SearchStore for consistent memory operations.
// It implements the full Store interface so it can be used as a drop-in replacement
// for BoltStore in any context that expects store.Store.
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

// --- Memory Operations (dual-write to bolt + bleve) ---

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

// GetMemory retrieves a memory by ID from bbolt.
func (c *CombinedStore) GetMemory(id string) (*memory.Memory, error) {
	return c.bolt.GetMemory(id)
}

// ListMemories returns memories from bbolt (source of truth).
func (c *CombinedStore) ListMemories(opts memory.SearchOptions) ([]*memory.Memory, error) {
	return c.bolt.ListMemories(opts)
}

// SearchMemories performs full-text search using bleve, returning []*memory.Memory
// to satisfy the Store interface. Falls back to bolt substring search on error.
func (c *CombinedStore) SearchMemories(query string, limit int) ([]*memory.Memory, error) {
	results, err := c.search.Search(query, limit)
	if err != nil {
		// Fall back to substring search
		return c.bolt.SearchMemories(query, limit)
	}

	// Enrich results with full memory data from bolt (source of truth)
	memories := make([]*memory.Memory, 0, len(results))
	for _, r := range results {
		if m, err := c.bolt.GetMemory(r.ID); err == nil {
			memories = append(memories, m)
		}
	}
	return memories, nil
}

// SearchMemoriesWithScore performs full-text search returning results with relevance scores.
// This is the scored variant for callers that need score-based filtering (e.g. CLI --min-score).
func (c *CombinedStore) SearchMemoriesWithScore(query string, limit int) ([]SearchResult, error) {
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

// ClearMemories removes all memories from both stores.
func (c *CombinedStore) ClearMemories() (int, error) {
	count, err := c.bolt.ClearMemories()
	if err != nil {
		return count, err
	}
	_ = c.search.Clear()
	return count, nil
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

// --- State Operations (delegated to BoltStore) ---

func (c *CombinedStore) SetState(st *memory.State) error            { return c.bolt.SetState(st) }
func (c *CombinedStore) GetState(key string) (*memory.State, error) { return c.bolt.GetState(key) }
func (c *CombinedStore) DeleteState(key string) error               { return c.bolt.DeleteState(key) }
func (c *CombinedStore) ListState(agentFilter string) ([]*memory.State, error) {
	return c.bolt.ListState(agentFilter)
}
func (c *CombinedStore) ClearState(agentID string) (int, error) { return c.bolt.ClearState(agentID) }
func (c *CombinedStore) CleanupStaleState(maxAge time.Duration) (int, error) {
	return c.bolt.CleanupStaleState(maxAge)
}

// --- Decision Operations (delegated to BoltStore) ---

func (c *CombinedStore) SetDecision(d *memory.Decision) error { return c.bolt.SetDecision(d) }
func (c *CombinedStore) GetDecision(topic string) (*memory.Decision, error) {
	return c.bolt.GetDecision(topic)
}
func (c *CombinedStore) ListDecisions() ([]*memory.Decision, error) { return c.bolt.ListDecisions() }
func (c *CombinedStore) GetDecisionHistory(topic string) ([]*memory.Decision, error) {
	return c.bolt.GetDecisionHistory(topic)
}
func (c *CombinedStore) DeleteDecision(topic string) (int, error) {
	return c.bolt.DeleteDecision(topic)
}
func (c *CombinedStore) ClearDecisions() (int, error) { return c.bolt.ClearDecisions() }

// --- Message Operations (delegated to BoltStore) ---

func (c *CombinedStore) AddMessage(m *memory.Message) error { return c.bolt.AddMessage(m) }
func (c *CombinedStore) GetMessages(agentID string) ([]*memory.Message, error) {
	return c.bolt.GetMessages(agentID)
}
func (c *CombinedStore) AckMessage(messageID uint64, agentID string) error {
	return c.bolt.AckMessage(messageID, agentID)
}
func (c *CombinedStore) PruneMessages() (int, error) { return c.bolt.PruneMessages() }

// --- Task Operations (delegated to BoltStore) ---

func (c *CombinedStore) CreateTask(t *memory.Task) error { return c.bolt.CreateTask(t) }
func (c *CombinedStore) GetTask(id string) (*memory.Task, error) {
	return c.bolt.GetTask(id)
}
func (c *CombinedStore) ListTasks(status memory.TaskStatus) ([]*memory.Task, error) {
	return c.bolt.ListTasks(status)
}
func (c *CombinedStore) ClaimTask(taskID, agentID string) (*memory.Task, error) {
	return c.bolt.ClaimTask(taskID, agentID)
}
func (c *CombinedStore) CompleteTask(taskID, result string) error {
	return c.bolt.CompleteTask(taskID, result)
}
func (c *CombinedStore) UpdateTask(t *memory.Task) error { return c.bolt.UpdateTask(t) }
func (c *CombinedStore) DeleteTask(id string) error      { return c.bolt.DeleteTask(id) }
func (c *CombinedStore) ClearTasks(status memory.TaskStatus) (int, error) {
	return c.bolt.ClearTasks(status)
}
