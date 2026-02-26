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

// SchemaVersion is the current schema version for the main store.
// Increment this when adding new migrations to the migrations slice.
var SchemaVersion uint64 = 1

// CodeSchemaVersion is the current schema version for the code store.
// Increment this when adding new migrations to the codeMigrations slice.
var CodeSchemaVersion uint64 = 1

// FindingsSchemaVersion is the current schema version for the findings store.
// Increment this when adding new migrations to the findingsMigrations slice.
var FindingsSchemaVersion uint64 = 1

// migration represents a single schema migration step.
type migration struct {
	version     uint64
	description string
	migrate     func(tx *bolt.Tx) error
}

// migrations is the ordered list of all main store schema migrations.
var migrations = []migration{
	{version: 1, description: "baseline schema stamp", migrate: func(tx *bolt.Tx) error { return nil }},
}

// codeMigrations is the ordered list of all code store schema migrations.
var codeMigrations = []migration{
	{version: 1, description: "baseline code schema stamp", migrate: func(tx *bolt.Tx) error { return nil }},
}

// findingsMigrations is the ordered list of all findings store schema migrations.
var findingsMigrations = []migration{
	{version: 1, description: "baseline findings schema stamp", migrate: func(tx *bolt.Tx) error { return nil }},
}

// migrationConfig holds the parameters for a migration run.
type migrationConfig struct {
	metaBucket []byte
	versionKey string
	target     uint64
	migrations []migration
}

// mainMigrationConfig returns the config for the main store.
func mainMigrationConfig() migrationConfig {
	return migrationConfig{
		metaBucket: BucketMeta,
		versionKey: "schema_version",
		target:     SchemaVersion,
		migrations: migrations,
	}
}

// codeMigrationConfig returns the config for the code store.
func codeMigrationConfig() migrationConfig {
	return migrationConfig{
		metaBucket: BucketCodeMeta,
		versionKey: "schema_version",
		target:     CodeSchemaVersion,
		migrations: codeMigrations,
	}
}

// findingsMigrationConfig returns the config for the findings store.
func findingsMigrationConfig() migrationConfig {
	return migrationConfig{
		metaBucket: BucketFindingsMeta,
		versionKey: "schema_version",
		target:     FindingsSchemaVersion,
		migrations: findingsMigrations,
	}
}

// RunMigrations applies pending schema migrations to the main store database.
func RunMigrations(db *bolt.DB) error {
	return runMigrations(db, mainMigrationConfig())
}

// RunCodeMigrations applies pending schema migrations to the code store database.
func RunCodeMigrations(db *bolt.DB) error {
	return runMigrations(db, codeMigrationConfig())
}

// RunFindingsMigrations applies pending schema migrations to the findings store database.
func RunFindingsMigrations(db *bolt.DB) error {
	return runMigrations(db, findingsMigrationConfig())
}

// GetSchemaVersion reads the current schema version from the main store meta bucket.
func GetSchemaVersion(db *bolt.DB) (uint64, error) {
	return getSchemaVersion(db, BucketMeta, "schema_version")
}

// GetCodeSchemaVersion reads the current schema version from the code store meta bucket.
func GetCodeSchemaVersion(db *bolt.DB) (uint64, error) {
	return getSchemaVersion(db, BucketCodeMeta, "schema_version")
}

// GetFindingsSchemaVersion reads the current schema version from the findings store meta bucket.
func GetFindingsSchemaVersion(db *bolt.DB) (uint64, error) {
	return getSchemaVersion(db, BucketFindingsMeta, "schema_version")
}

// runMigrations is the parameterized migration engine.
func runMigrations(db *bolt.DB, cfg migrationConfig) error {
	current, err := getSchemaVersion(db, cfg.metaBucket, cfg.versionKey)
	if err != nil {
		return fmt.Errorf("reading schema version: %w", err)
	}

	if current > cfg.target {
		return fmt.Errorf("database schema version %d is ahead of binary version %d (downgrade not supported)", current, cfg.target)
	}

	if current == cfg.target {
		return nil
	}

	// Collect pending migrations.
	var pending []migration
	for _, m := range cfg.migrations {
		if m.version > current {
			pending = append(pending, m)
		}
	}

	if len(pending) == 0 {
		// No migrations to run but version differs — just stamp.
		return setSchemaVersion(db, cfg.metaBucket, cfg.versionKey, cfg.target)
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
		meta := tx.Bucket(cfg.metaBucket)
		if meta == nil {
			return fmt.Errorf("meta bucket %q not found", string(cfg.metaBucket))
		}
		buf := make([]byte, 8)
		binary.BigEndian.PutUint64(buf, cfg.target)
		return meta.Put([]byte(cfg.versionKey), buf)
	})
	if err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}

	return nil
}

// getSchemaVersion reads a schema version from the specified bucket and key.
func getSchemaVersion(db *bolt.DB, metaBucket []byte, versionKey string) (uint64, error) {
	var version uint64
	err := db.View(func(tx *bolt.Tx) error {
		meta := tx.Bucket(metaBucket)
		if meta == nil {
			return nil // Fresh DB, no meta bucket yet
		}
		data := meta.Get([]byte(versionKey))
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

// setSchemaVersion writes a schema version to the specified bucket and key.
func setSchemaVersion(db *bolt.DB, metaBucket []byte, versionKey string, version uint64) error {
	return db.Update(func(tx *bolt.Tx) error {
		meta := tx.Bucket(metaBucket)
		if meta == nil {
			return fmt.Errorf("meta bucket %q not found", string(metaBucket))
		}
		buf := make([]byte, 8)
		binary.BigEndian.PutUint64(buf, version)
		return meta.Put([]byte(versionKey), buf)
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
