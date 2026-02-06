package store

import (
	"encoding/json"
	"log"
	"strings"

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

// DeleteMemory removes a memory by ID.
func (s *BoltStore) DeleteMemory(id string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketMemories)
		return b.Delete([]byte(id))
	})
}

// ListMemories returns memories matching the given options.
func (s *BoltStore) ListMemories(opts memory.SearchOptions) ([]*memory.Memory, error) {
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
func (s *BoltStore) SearchMemories(query string, limit int) ([]*memory.Memory, error) {
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
