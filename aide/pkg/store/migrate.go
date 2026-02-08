// Package store provides storage backends for aide.
// This file implements schema versioning and migration for BBolt and Bleve search indexes.
package store

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"

	"github.com/blevesearch/bleve/v2/mapping"
	bolt "go.etcd.io/bbolt"
)

// SchemaVersion is the current schema version. Increment this when adding new migrations.
var SchemaVersion uint64 = 1

// migration represents a single schema migration step.
type migration struct {
	version     uint64
	description string
	migrate     func(tx *bolt.Tx) error
}

// migrations is the ordered list of all schema migrations.
// Each migration is applied exactly once, in order, when the DB version is below SchemaVersion.
var migrations = []migration{
	{version: 1, description: "baseline schema stamp", migrate: func(tx *bolt.Tx) error { return nil }},
}

// RunMigrations applies any pending schema migrations to the database.
// It reads the current version from the meta bucket, applies migrations with version > current,
// and writes the new version. Returns an error if the DB version is ahead of SchemaVersion (downgrade).
func RunMigrations(db *bolt.DB) error {
	current, err := GetSchemaVersion(db)
	if err != nil {
		return fmt.Errorf("reading schema version: %w", err)
	}

	if current > SchemaVersion {
		return fmt.Errorf("database schema version %d is ahead of binary version %d (downgrade not supported)", current, SchemaVersion)
	}

	if current == SchemaVersion {
		return nil
	}

	// Collect pending migrations.
	var pending []migration
	for _, m := range migrations {
		if m.version > current {
			pending = append(pending, m)
		}
	}

	if len(pending) == 0 {
		// No migrations to run but version differs — just stamp.
		return setSchemaVersion(db, SchemaVersion)
	}

	// Apply all pending migrations in a single transaction.
	err = db.Update(func(tx *bolt.Tx) error {
		for _, m := range pending {
			log.Printf("store: applying migration v%d: %s", m.version, m.description)
			if err := m.migrate(tx); err != nil {
				return fmt.Errorf("migration v%d (%s) failed: %w", m.version, m.description, err)
			}
		}

		// Write the new version inside the same transaction.
		meta := tx.Bucket(BucketMeta)
		if meta == nil {
			return fmt.Errorf("meta bucket not found")
		}
		buf := make([]byte, 8)
		binary.BigEndian.PutUint64(buf, SchemaVersion)
		return meta.Put([]byte("schema_version"), buf)
	})
	if err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}

	return nil
}

// GetSchemaVersion reads the current schema version from the meta bucket.
// Returns 0 if no version has been set (fresh database).
func GetSchemaVersion(db *bolt.DB) (uint64, error) {
	var version uint64
	err := db.View(func(tx *bolt.Tx) error {
		meta := tx.Bucket(BucketMeta)
		if meta == nil {
			return nil // Fresh DB, no meta bucket yet
		}
		data := meta.Get([]byte("schema_version"))
		if data == nil {
			return nil // No version set
		}
		if len(data) != 8 {
			return fmt.Errorf("corrupt schema_version: expected 8 bytes, got %d", len(data))
		}
		version = binary.BigEndian.Uint64(data)
		return nil
	})
	return version, err
}

// setSchemaVersion writes the schema version to the meta bucket.
func setSchemaVersion(db *bolt.DB, version uint64) error {
	return db.Update(func(tx *bolt.Tx) error {
		meta := tx.Bucket(BucketMeta)
		if meta == nil {
			return fmt.Errorf("meta bucket not found")
		}
		buf := make([]byte, 8)
		binary.BigEndian.PutUint64(buf, version)
		return meta.Put([]byte("schema_version"), buf)
	})
}

// MappingHash computes a deterministic SHA-256 hex digest of a Bleve index mapping.
// Used to detect when a mapping has changed and the search index needs rebuilding.
func MappingHash(m mapping.IndexMapping) string {
	data, err := json.Marshal(m)
	if err != nil {
		// This should never happen with a valid mapping; return empty to force rebuild.
		return ""
	}
	h := sha256.Sum256(data)
	return fmt.Sprintf("%x", h)
}

// -------------------------------------------------------------------
// Example migrations (commented out) showing common patterns.
// Copy and adapt these when adding real migrations.
// -------------------------------------------------------------------
//
// // Example 1: Add a field with a default value.
// // Iterates a bucket, unmarshals each value, sets the new field, re-marshals.
// {version: 2, description: "add priority field to memories", migrate: func(tx *bolt.Tx) error {
// 	b := tx.Bucket(BucketMemories)
// 	c := b.Cursor()
// 	for k, v := c.First(); k != nil; k, v = c.Next() {
// 		var m map[string]interface{}
// 		if err := json.Unmarshal(v, &m); err != nil {
// 			return err
// 		}
// 		if _, ok := m["priority"]; !ok {
// 			m["priority"] = 0.5
// 		}
// 		data, err := json.Marshal(m)
// 		if err != nil {
// 			return err
// 		}
// 		if err := b.Put(k, data); err != nil {
// 			return err
// 		}
// 	}
// 	return nil
// }},
//
// // Example 2: Add a new bucket.
// {version: 3, description: "add sessions bucket", migrate: func(tx *bolt.Tx) error {
// 	_, err := tx.CreateBucketIfNotExists([]byte("sessions"))
// 	return err
// }},
//
// // Example 3: Rename a field.
// {version: 4, description: "rename 'desc' to 'description' in tasks", migrate: func(tx *bolt.Tx) error {
// 	b := tx.Bucket(BucketTasks)
// 	c := b.Cursor()
// 	for k, v := c.First(); k != nil; k, v = c.Next() {
// 		var m map[string]interface{}
// 		if err := json.Unmarshal(v, &m); err != nil {
// 			return err
// 		}
// 		if val, ok := m["desc"]; ok {
// 			m["description"] = val
// 			delete(m, "desc")
// 			data, err := json.Marshal(m)
// 			if err != nil {
// 				return err
// 			}
// 			if err := b.Put(k, data); err != nil {
// 				return err
// 			}
// 		}
// 	}
// 	return nil
// }},
