package store

import (
	"encoding/json"
	"log"
	"strings"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/memory"
	bolt "go.etcd.io/bbolt"
)

// AddMemory stores a new memory entry.
func (s *BoltStore) AddMemory(m *memory.Memory) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketMemories)
		data, err := json.Marshal(m)
		if err != nil {
			return err
		}
		return b.Put([]byte(m.ID), data)
	})
}

// GetMemory retrieves a memory by ID.
func (s *BoltStore) GetMemory(id string) (*memory.Memory, error) {
	var m memory.Memory
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketMemories)
		data := b.Get([]byte(id))
		if data == nil {
			return ErrNotFound
		}
		return json.Unmarshal(data, &m)
	})
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// UpdateMemory updates an existing memory entry (replaces content in place).
func (s *BoltStore) UpdateMemory(m *memory.Memory) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketMemories)
		// Verify it exists first.
		if b.Get([]byte(m.ID)) == nil {
			return ErrNotFound
		}
		data, err := json.Marshal(m)
		if err != nil {
			return err
		}
		return b.Put([]byte(m.ID), data)
	})
}

// DeleteMemory removes a memory by ID.
func (s *BoltStore) DeleteMemory(id string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketMemories)
		return b.Delete([]byte(id))
	})
}

// ListMemories returns memories matching the given options.
func (s *BoltStore) ListMemories(opts memory.SearchOptions) ([]*memory.Memory, error) {
	opts.ApplyDefaults()

	// Build exclude set for O(1) lookup.
	excludeSet := make(map[string]bool, len(opts.ExcludeTags))
	for _, t := range opts.ExcludeTags {
		excludeSet[t] = true
	}

	var memories []*memory.Memory

	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketMemories)
		return b.ForEach(func(k, v []byte) error {
			var m memory.Memory
			if err := json.Unmarshal(v, &m); err != nil {
				log.Printf("store: skipping malformed memory entry: %v", err)
				return nil
			}

			// Apply filters.
			if opts.Category != "" && m.Category != opts.Category {
				return nil
			}
			if opts.Plan != "" && m.Plan != opts.Plan {
				return nil
			}
			if len(opts.Tags) > 0 && !hasAnyTag(m.Tags, opts.Tags) {
				return nil
			}
			// Exclude memories with any excluded tag.
			if len(excludeSet) > 0 {
				for _, tag := range m.Tags {
					if excludeSet[tag] {
						return nil
					}
				}
			}

			memories = append(memories, &m)
			return nil
		})
	})

	// Apply limit.
	if opts.Limit > 0 && len(memories) > opts.Limit {
		memories = memories[:opts.Limit]
	}

	return memories, err
}

// SearchMemories performs a simple text search across memories.
// Results are post-filtered by DefaultExcludeTags. Use SearchMemoriesAll to bypass.
func (s *BoltStore) SearchMemories(query string, limit int) ([]*memory.Memory, error) {
	memories, err := s.SearchMemoriesAll(query, limit)
	if err != nil {
		return nil, err
	}
	return memory.FilterMemories(memories, memory.DefaultExcludeTags), nil
}

// SearchMemoriesAll performs a simple text search without exclude-tag filtering.
func (s *BoltStore) SearchMemoriesAll(query string, limit int) ([]*memory.Memory, error) {
	var memories []*memory.Memory
	queryLower := strings.ToLower(query)

	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketMemories)
		return b.ForEach(func(_, v []byte) error {
			if limit > 0 && len(memories) >= limit {
				return nil
			}

			var m memory.Memory
			if err := json.Unmarshal(v, &m); err != nil {
				log.Printf("store: skipping malformed memory entry: %v", err)
				return nil
			}

			// Simple substring search in content.
			if strings.Contains(strings.ToLower(m.Content), queryLower) {
				memories = append(memories, &m)
			}
			return nil
		})
	})

	return memories, err
}

// ClearMemories removes all memories.
func (s *BoltStore) ClearMemories() (int, error) {
	var count int
	var keysToDelete [][]byte

	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketMemories)
		return b.ForEach(func(k, _ []byte) error {
			keysToDelete = append(keysToDelete, k)
			return nil
		})
	})
	if err != nil {
		return 0, err
	}

	if len(keysToDelete) > 0 {
		err = s.db.Update(func(tx *bolt.Tx) error {
			b := tx.Bucket(BucketMemories)
			for _, k := range keysToDelete {
				if err := b.Delete(k); err != nil {
					return err
				}
				count++
			}
			return nil
		})
	}

	return count, err
}

// TouchMemory increments AccessCount and updates LastAccessed for the given memory IDs.
// This is a lightweight operation for tracking access patterns (foundation for memory decay).
func (s *BoltStore) TouchMemory(ids []string) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	var touched int
	now := time.Now()
	err := s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketMemories)
		for _, id := range ids {
			data := b.Get([]byte(id))
			if data == nil {
				continue
			}
			var m memory.Memory
			if err := json.Unmarshal(data, &m); err != nil {
				log.Printf("store: touchMemory: skipping malformed entry %s: %v", id, err)
				continue
			}
			m.AccessCount++
			m.LastAccessed = now
			updated, err := json.Marshal(&m)
			if err != nil {
				log.Printf("store: touchMemory: marshal error for %s: %v", id, err)
				continue
			}
			if err := b.Put([]byte(id), updated); err != nil {
				return err
			}
			touched++
		}
		return nil
	})
	return touched, err
}
