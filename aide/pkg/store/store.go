// Package store provides storage backends for aide.
package store

import (
	"errors"
	"fmt"
	"time"

	bolt "go.etcd.io/bbolt"
)

// Common errors.
var (
	ErrNotFound       = errors.New("not found")
	ErrAlreadyClaimed = errors.New("task already claimed")
	ErrConflict       = errors.New("decision conflict")
)

// Bucket names.
var (
	BucketMemories  = []byte("memories")
	BucketTasks     = []byte("tasks")
	BucketMessages  = []byte("messages")
	BucketDecisions = []byte("decisions")
	BucketState     = []byte("state")
	BucketMeta      = []byte("meta")
)

// BoltStore implements storage using bbolt.
type BoltStore struct {
	db *bolt.DB
}

// NewBoltStore creates a new bbolt-backed store.
func NewBoltStore(path string) (*BoltStore, error) {
	db, err := bolt.Open(path, 0o600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		return nil, err
	}

	// Initialize buckets.
	err = db.Update(func(tx *bolt.Tx) error {
		buckets := [][]byte{
			BucketMemories,
			BucketTasks,
			BucketMessages,
			BucketDecisions,
			BucketState,
			BucketMeta,
		}
		for _, bucket := range buckets {
			if _, err := tx.CreateBucketIfNotExists(bucket); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		db.Close()
		return nil, err
	}

	if err := RunMigrations(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("schema migration failed: %w", err)
	}

	return &BoltStore{db: db}, nil
}

// Close closes the database.
func (s *BoltStore) Close() error {
	return s.db.Close()
}

// itob converts a uint64 to a byte slice.
func itob(v uint64) []byte {
	b := make([]byte, 8)
	for i := uint(0); i < 8; i++ {
		b[7-i] = byte(v >> (i * 8))
	}
	return b
}

// GetMeta reads a string value from the meta bucket.
func (s *BoltStore) GetMeta(key string) (string, error) {
	var val string
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketMeta)
		if b == nil {
			return ErrNotFound
		}
		data := b.Get([]byte(key))
		if data == nil {
			return ErrNotFound
		}
		val = string(data)
		return nil
	})
	return val, err
}

// SetMeta writes a string value to the meta bucket.
func (s *BoltStore) SetMeta(key, value string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketMeta)
		if b == nil {
			return fmt.Errorf("meta bucket not found")
		}
		return b.Put([]byte(key), []byte(value))
	})
}

// hasAnyTag checks if any of the filter tags exist in the memory tags.
func hasAnyTag(memoryTags, filterTags []string) bool {
	tagSet := make(map[string]bool)
	for _, t := range memoryTags {
		tagSet[t] = true
	}
	for _, t := range filterTags {
		if tagSet[t] {
			return true
		}
	}
	return false
}
