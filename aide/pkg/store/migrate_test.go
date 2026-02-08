package store

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/analysis/analyzer/keyword"
	"github.com/blevesearch/bleve/v2/mapping"
	bolt "go.etcd.io/bbolt"
)

// setupMigrateTestDB creates a fresh BBolt database with the meta bucket initialized.
// Returns the DB, its path, and a cleanup function.
func setupMigrateTestDB(t *testing.T) (*bolt.DB, func()) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "aide-migrate-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := bolt.Open(dbPath, 0o600, nil)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to open db: %v", err)
	}

	// Create required buckets (mirrors NewBoltStore init).
	err = db.Update(func(tx *bolt.Tx) error {
		for _, b := range [][]byte{BucketMemories, BucketTasks, BucketMessages, BucketDecisions, BucketState, BucketMeta} {
			if _, err := tx.CreateBucketIfNotExists(b); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		db.Close()
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to create buckets: %v", err)
	}

	cleanup := func() {
		db.Close()
		os.RemoveAll(tmpDir)
	}
	return db, cleanup
}

// writeSchemaVersion sets the schema_version directly in the meta bucket.
func writeSchemaVersion(t *testing.T, db *bolt.DB, version uint64) {
	t.Helper()
	err := db.Update(func(tx *bolt.Tx) error {
		meta := tx.Bucket(BucketMeta)
		buf := make([]byte, 8)
		binary.BigEndian.PutUint64(buf, version)
		return meta.Put([]byte("schema_version"), buf)
	})
	if err != nil {
		t.Fatalf("failed to write schema version: %v", err)
	}
}

func TestRunMigrations_FreshDB(t *testing.T) {
	db, cleanup := setupMigrateTestDB(t)
	defer cleanup()

	// Fresh DB has version 0.
	v, err := GetSchemaVersion(db)
	if err != nil {
		t.Fatalf("GetSchemaVersion: %v", err)
	}
	if v != 0 {
		t.Fatalf("expected version 0 on fresh db, got %d", v)
	}

	// Run migrations should bring it to SchemaVersion.
	if err := RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	v, err = GetSchemaVersion(db)
	if err != nil {
		t.Fatalf("GetSchemaVersion: %v", err)
	}
	if v != SchemaVersion {
		t.Errorf("expected version %d after migration, got %d", SchemaVersion, v)
	}
}

func TestRunMigrations_AlreadyCurrent(t *testing.T) {
	db, cleanup := setupMigrateTestDB(t)
	defer cleanup()

	writeSchemaVersion(t, db, SchemaVersion)

	// Should be a no-op.
	if err := RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	v, err := GetSchemaVersion(db)
	if err != nil {
		t.Fatalf("GetSchemaVersion: %v", err)
	}
	if v != SchemaVersion {
		t.Errorf("expected version %d, got %d", SchemaVersion, v)
	}
}

func TestRunMigrations_AppliesPending(t *testing.T) {
	db, cleanup := setupMigrateTestDB(t)
	defer cleanup()

	// Seed a task in BBolt.
	err := db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketTasks)
		data, _ := json.Marshal(map[string]interface{}{
			"id":    "task-1",
			"title": "Original title",
		})
		return b.Put([]byte("task-1"), data)
	})
	if err != nil {
		t.Fatalf("failed to seed task: %v", err)
	}

	// Set version to 1 (current baseline).
	writeSchemaVersion(t, db, 1)

	// Save and restore globals to inject a test migration.
	origMigrations := migrations
	origVersion := SchemaVersion
	defer func() {
		migrations = origMigrations
		SchemaVersion = origVersion
	}()

	SchemaVersion = 2
	migrations = append(migrations, migration{
		version:     2,
		description: "add status field to tasks",
		migrate: func(tx *bolt.Tx) error {
			b := tx.Bucket(BucketTasks)
			c := b.Cursor()
			for k, v := c.First(); k != nil; k, v = c.Next() {
				var m map[string]interface{}
				if err := json.Unmarshal(v, &m); err != nil {
					return err
				}
				if _, ok := m["status"]; !ok {
					m["status"] = "pending"
				}
				data, err := json.Marshal(m)
				if err != nil {
					return err
				}
				if err := b.Put(k, data); err != nil {
					return err
				}
			}
			return nil
		},
	})

	if err := RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	// Verify version bumped.
	v, err := GetSchemaVersion(db)
	if err != nil {
		t.Fatalf("GetSchemaVersion: %v", err)
	}
	if v != 2 {
		t.Errorf("expected version 2, got %d", v)
	}

	// Verify data was transformed.
	err = db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketTasks)
		data := b.Get([]byte("task-1"))
		if data == nil {
			return fmt.Errorf("task-1 not found")
		}
		var m map[string]interface{}
		if err := json.Unmarshal(data, &m); err != nil {
			return err
		}
		status, ok := m["status"]
		if !ok {
			return fmt.Errorf("status field not added")
		}
		if status != "pending" {
			return fmt.Errorf("expected status 'pending', got %v", status)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("data verification failed: %v", err)
	}
}

func TestRunMigrations_DowngradeError(t *testing.T) {
	db, cleanup := setupMigrateTestDB(t)
	defer cleanup()

	// Set version above SchemaVersion.
	writeSchemaVersion(t, db, SchemaVersion+10)

	err := RunMigrations(db)
	if err == nil {
		t.Fatal("expected error for downgrade, got nil")
	}
}

func TestRunMigrations_PartialFailure(t *testing.T) {
	db, cleanup := setupMigrateTestDB(t)
	defer cleanup()

	writeSchemaVersion(t, db, 1)

	origMigrations := migrations
	origVersion := SchemaVersion
	defer func() {
		migrations = origMigrations
		SchemaVersion = origVersion
	}()

	SchemaVersion = 2
	migrations = append(migrations, migration{
		version:     2,
		description: "intentionally failing migration",
		migrate: func(tx *bolt.Tx) error {
			return fmt.Errorf("simulated failure")
		},
	})

	err := RunMigrations(db)
	if err == nil {
		t.Fatal("expected error from failing migration, got nil")
	}

	// Version should remain at 1 since the transaction rolled back.
	v, err := GetSchemaVersion(db)
	if err != nil {
		t.Fatalf("GetSchemaVersion: %v", err)
	}
	if v != 1 {
		t.Errorf("expected version to stay at 1 after failure, got %d", v)
	}
}

func TestGetSchemaVersion_EmptyDB(t *testing.T) {
	db, cleanup := setupMigrateTestDB(t)
	defer cleanup()

	v, err := GetSchemaVersion(db)
	if err != nil {
		t.Fatalf("GetSchemaVersion: %v", err)
	}
	if v != 0 {
		t.Errorf("expected 0 for empty db, got %d", v)
	}
}

func TestMappingHash_Deterministic(t *testing.T) {
	m1, err := buildIndexMapping()
	if err != nil {
		t.Fatalf("buildIndexMapping: %v", err)
	}
	m2, err := buildIndexMapping()
	if err != nil {
		t.Fatalf("buildIndexMapping: %v", err)
	}

	h1 := MappingHash(m1)
	h2 := MappingHash(m2)

	if h1 == "" {
		t.Fatal("hash should not be empty")
	}
	if h1 != h2 {
		t.Errorf("same mapping produced different hashes: %s vs %s", h1, h2)
	}
}

func TestMappingHash_DifferentMappings(t *testing.T) {
	m1, err := buildIndexMapping()
	if err != nil {
		t.Fatalf("buildIndexMapping: %v", err)
	}

	// Build a different mapping.
	m2 := bleve.NewIndexMapping()
	doc := bleve.NewDocumentMapping()
	f := mapping.NewTextFieldMapping()
	f.Analyzer = keyword.Name
	doc.AddFieldMappingsAt("different_field", f)
	m2.AddDocumentMapping("different", doc)
	m2.DefaultMapping = doc

	h1 := MappingHash(m1)
	h2 := MappingHash(m2)

	if h1 == h2 {
		t.Error("different mappings should produce different hashes")
	}
}
