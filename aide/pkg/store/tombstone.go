package store

import (
	"encoding/json"
	"log"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/memory"
	bolt "go.etcd.io/bbolt"
)

// tombstoneKey builds the bucket key for a tombstone. Kind comes first so a
// memory ULID and a decision topic with the same text can never collide.
func tombstoneKey(kind, id string) []byte {
	return []byte(kind + ":" + id)
}

// AddTombstone stores a tombstone, overwriting any existing one for the same
// kind/id pair. Callers that need newest-wins semantics must compare DeletedAt
// before calling.
func (s *BoltStore) AddTombstone(t *memory.Tombstone) error {
	if t.DeletedAt.IsZero() {
		t.DeletedAt = time.Now()
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		return putTombstone(tx, t)
	})
}

// putTombstone writes a tombstone within an existing transaction so delete
// operations can record one atomically with the record removal.
func putTombstone(tx *bolt.Tx, t *memory.Tombstone) error {
	b := tx.Bucket(BucketTombstones)
	data, err := json.Marshal(t)
	if err != nil {
		return err
	}
	return b.Put(tombstoneKey(t.Kind, t.ID), data)
}

// GetTombstone retrieves a tombstone by kind and id.
func (s *BoltStore) GetTombstone(kind, id string) (*memory.Tombstone, error) {
	var t memory.Tombstone
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketTombstones)
		data := b.Get(tombstoneKey(kind, id))
		if data == nil {
			return ErrNotFound
		}
		return json.Unmarshal(data, &t)
	})
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// ListTombstones returns all tombstones.
func (s *BoltStore) ListTombstones() ([]*memory.Tombstone, error) {
	var tombstones []*memory.Tombstone
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketTombstones)
		return b.ForEach(func(_, v []byte) error {
			var t memory.Tombstone
			if err := json.Unmarshal(v, &t); err != nil {
				log.Printf("store: skipping malformed tombstone entry: %v", err)
				return nil
			}
			tombstones = append(tombstones, &t)
			return nil
		})
	})
	return tombstones, err
}

// DeleteTombstone removes a tombstone by kind and id.
func (s *BoltStore) DeleteTombstone(kind, id string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(BucketTombstones).Delete(tombstoneKey(kind, id))
	})
}
