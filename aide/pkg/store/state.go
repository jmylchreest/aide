package store

import (
	"encoding/json"
	"log"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/memory"
	bolt "go.etcd.io/bbolt"
)

// SetState stores a state key-value pair.
func (s *BoltStore) SetState(st *memory.State) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketState)
		st.UpdatedAt = time.Now()
		data, err := json.Marshal(st)
		if err != nil {
			return err
		}
		return b.Put([]byte(st.Key), data)
	})
}

// GetState retrieves a state value by key.
func (s *BoltStore) GetState(key string) (*memory.State, error) {
	var st memory.State
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketState)
		data := b.Get([]byte(key))
		if data == nil {
			return ErrNotFound
		}
		return json.Unmarshal(data, &st)
	})
	if err != nil {
		return nil, err
	}
	return &st, nil
}

// DeleteState removes a state key.
func (s *BoltStore) DeleteState(key string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketState)
		return b.Delete([]byte(key))
	})
}

// ListState returns all state entries, optionally filtered by agent.
func (s *BoltStore) ListState(agentFilter string) ([]*memory.State, error) {
	var states []*memory.State

	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketState)
		return b.ForEach(func(k, v []byte) error {
			var st memory.State
			if err := json.Unmarshal(v, &st); err != nil {
				log.Printf("store: skipping malformed state entry: %v", err)
				return nil
			}
			// Filter by agent if specified.
			if agentFilter == "" || st.Agent == agentFilter {
				states = append(states, &st)
			}
			return nil
		})
	})

	return states, err
}

// ClearState removes all state entries for an agent (or all if agentID is empty).
func (s *BoltStore) ClearState(agentID string) (int, error) {
	var count int
	var keysToDelete [][]byte

	// First, collect keys to delete.
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketState)
		return b.ForEach(func(k, v []byte) error {
			if agentID == "" {
				// Delete all.
				keysToDelete = append(keysToDelete, k)
			} else {
				// Delete only agent-specific keys.
				var st memory.State
				if err := json.Unmarshal(v, &st); err == nil && st.Agent == agentID {
					keysToDelete = append(keysToDelete, k)
				}
			}
			return nil
		})
	})
	if err != nil {
		return 0, err
	}

	// Then delete them.
	err = s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketState)
		for _, k := range keysToDelete {
			if err := b.Delete(k); err != nil {
				return err
			}
			count++
		}
		return nil
	})

	return count, err
}

// CleanupStaleState removes agent-specific state entries older than maxAge.
// Global state (no agent) is preserved.
func (s *BoltStore) CleanupStaleState(maxAge time.Duration) (int, error) {
	var count int
	var keysToDelete [][]byte
	cutoff := time.Now().Add(-maxAge)

	// First, collect stale keys.
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketState)
		return b.ForEach(func(k, v []byte) error {
			var st memory.State
			if err := json.Unmarshal(v, &st); err != nil {
				log.Printf("store: skipping malformed state entry: %v", err)
				return nil
			}
			// Only clean up agent-specific state that's stale.
			if st.Agent != "" && st.UpdatedAt.Before(cutoff) {
				keysToDelete = append(keysToDelete, k)
			}
			return nil
		})
	})
	if err != nil {
		return 0, err
	}

	// Then delete them.
	err = s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketState)
		for _, k := range keysToDelete {
			if err := b.Delete(k); err != nil {
				return err
			}
			count++
		}
		return nil
	})

	return count, err
}
