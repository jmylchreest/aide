package store

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/memory"
	bolt "go.etcd.io/bbolt"
)

// SetDecision stores a decision (append-only, allows multiple per topic).
// Decisions are stored with composite key: topic:timestamp.
func (s *BoltStore) SetDecision(d *memory.Decision) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketDecisions)

		// Ensure CreatedAt is set.
		if d.CreatedAt.IsZero() {
			d.CreatedAt = time.Now()
		}

		// Composite key: topic:timestamp (allows multiple per topic).
		key := fmt.Sprintf("%s:%d", d.Topic, d.CreatedAt.UnixNano())

		data, err := json.Marshal(d)
		if err != nil {
			return err
		}
		return b.Put([]byte(key), data)
	})
}

// GetDecision retrieves the latest decision by topic.
func (s *BoltStore) GetDecision(topic string) (*memory.Decision, error) {
	var latest *memory.Decision
	prefix := topic + ":"

	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketDecisions)
		c := b.Cursor()

		// Seek to the topic prefix and find the latest (highest timestamp).
		for k, v := c.Seek([]byte(prefix)); k != nil && strings.HasPrefix(string(k), prefix); k, v = c.Next() {
			var d memory.Decision
			if err := json.Unmarshal(v, &d); err != nil {
				log.Printf("store: skipping malformed decision entry: %v", err)
				continue
			}
			if latest == nil || d.CreatedAt.After(latest.CreatedAt) {
				latest = &d
			}
		}
		return nil
	})

	if err != nil {
		return nil, err
	}
	if latest == nil {
		return nil, ErrNotFound
	}
	return latest, nil
}

// GetDecisionHistory retrieves all decisions for a topic, ordered by time.
func (s *BoltStore) GetDecisionHistory(topic string) ([]*memory.Decision, error) {
	var decisions []*memory.Decision
	prefix := topic + ":"

	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketDecisions)
		c := b.Cursor()

		for k, v := c.Seek([]byte(prefix)); k != nil && strings.HasPrefix(string(k), prefix); k, v = c.Next() {
			var d memory.Decision
			if err := json.Unmarshal(v, &d); err != nil {
				log.Printf("store: skipping malformed decision entry: %v", err)
				continue
			}
			decisions = append(decisions, &d)
		}
		return nil
	})

	return decisions, err
}

// ListDecisions returns all decisions.
func (s *BoltStore) ListDecisions() ([]*memory.Decision, error) {
	var decisions []*memory.Decision

	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketDecisions)
		if b == nil {
			return nil // read-only open of a store without the bucket
		}
		return b.ForEach(func(_, v []byte) error {
			var d memory.Decision
			if err := json.Unmarshal(v, &d); err != nil {
				log.Printf("store: skipping malformed decision entry: %v", err)
				return nil
			}
			decisions = append(decisions, &d)
			return nil
		})
	})

	return decisions, err
}

// DeleteDecision removes all decisions for a topic. When at least one
// decision existed, a decision-topic tombstone is recorded in the same
// transaction so the deletion propagates through share export/import.
// Collecting, deleting and tombstoning all happen in one write transaction
// so a concurrent write can never slip between the scan and the delete.
func (s *BoltStore) DeleteDecision(topic string) (int, error) {
	var count int
	prefix := []byte(topic + ":")

	err := s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketDecisions)
		c := b.Cursor()

		// Collect first (copying keys: cursor memory is invalidated by
		// mutation), then delete, all inside the same write transaction.
		var keysToDelete [][]byte
		for k, _ := c.Seek(prefix); k != nil && strings.HasPrefix(string(k), string(prefix)); k, _ = c.Next() {
			keysToDelete = append(keysToDelete, append([]byte(nil), k...))
		}
		for _, k := range keysToDelete {
			if err := b.Delete(k); err != nil {
				return err
			}
			count++
		}
		if count == 0 {
			return nil
		}
		return putTombstone(tx, &memory.Tombstone{
			ID:        topic,
			Kind:      memory.TombstoneKindDecisionTopic,
			DeletedAt: time.Now(),
		})
	})
	if err != nil {
		return 0, err
	}
	return count, nil
}

// ClearDecisions removes all decisions.
func (s *BoltStore) ClearDecisions() (int, error) {
	var count int
	var keysToDelete [][]byte

	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketDecisions)
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
			b := tx.Bucket(BucketDecisions)
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
