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

// =============================================================================
// Code Store Migrations
// =============================================================================

// setupCodeMigrateTestDB creates a fresh BBolt database with the code meta bucket.
func setupCodeMigrateTestDB(t *testing.T) (*bolt.DB, func()) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "aide-code-migrate-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	dbPath := filepath.Join(tmpDir, "code.db")
	db, err := bolt.Open(dbPath, 0o600, nil)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to open db: %v", err)
	}

	err = db.Update(func(tx *bolt.Tx) error {
		for _, b := range [][]byte{BucketSymbols, BucketReferences, BucketFileIndex, BucketRefIndex, BucketCodeMeta} {
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

func TestRunCodeMigrations_FreshDB(t *testing.T) {
	db, cleanup := setupCodeMigrateTestDB(t)
	defer cleanup()

	v, err := GetCodeSchemaVersion(db)
	if err != nil {
		t.Fatalf("GetCodeSchemaVersion: %v", err)
	}
	if v != 0 {
		t.Fatalf("expected version 0 on fresh db, got %d", v)
	}

	if err := RunCodeMigrations(db); err != nil {
		t.Fatalf("RunCodeMigrations: %v", err)
	}

	v, err = GetCodeSchemaVersion(db)
	if err != nil {
		t.Fatalf("GetCodeSchemaVersion: %v", err)
	}
	if v != CodeSchemaVersion {
		t.Errorf("expected version %d, got %d", CodeSchemaVersion, v)
	}
}

func TestRunCodeMigrations_AlreadyCurrent(t *testing.T) {
	db, cleanup := setupCodeMigrateTestDB(t)
	defer cleanup()

	// Stamp at current version.
	writeCodeSchemaVersion(t, db, CodeSchemaVersion)

	if err := RunCodeMigrations(db); err != nil {
		t.Fatalf("RunCodeMigrations: %v", err)
	}

	v, err := GetCodeSchemaVersion(db)
	if err != nil {
		t.Fatalf("GetCodeSchemaVersion: %v", err)
	}
	if v != CodeSchemaVersion {
		t.Errorf("expected version %d, got %d", CodeSchemaVersion, v)
	}
}

func TestRunCodeMigrations_DowngradeError(t *testing.T) {
	db, cleanup := setupCodeMigrateTestDB(t)
	defer cleanup()

	writeCodeSchemaVersion(t, db, CodeSchemaVersion+10)

	err := RunCodeMigrations(db)
	if err == nil {
		t.Fatal("expected error for downgrade, got nil")
	}
}

func TestRunCodeMigrations_AppliesPending(t *testing.T) {
	db, cleanup := setupCodeMigrateTestDB(t)
	defer cleanup()

	writeCodeSchemaVersion(t, db, 1)

	// Seed a symbol.
	err := db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketSymbols)
		data, _ := json.Marshal(map[string]interface{}{
			"id":   "sym-1",
			"name": "testFunc",
		})
		return b.Put([]byte("sym-1"), data)
	})
	if err != nil {
		t.Fatalf("failed to seed symbol: %v", err)
	}

	origMigrations := codeMigrations
	origVersion := CodeSchemaVersion
	defer func() {
		codeMigrations = origMigrations
		CodeSchemaVersion = origVersion
	}()

	CodeSchemaVersion = 2
	codeMigrations = append(codeMigrations, migration{
		version:     2,
		description: "add language field to symbols",
		migrate: func(tx *bolt.Tx) error {
			b := tx.Bucket(BucketSymbols)
			c := b.Cursor()
			for k, v := c.First(); k != nil; k, v = c.Next() {
				var m map[string]interface{}
				if err := json.Unmarshal(v, &m); err != nil {
					return err
				}
				if _, ok := m["language"]; !ok {
					m["language"] = "go"
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

	if err := RunCodeMigrations(db); err != nil {
		t.Fatalf("RunCodeMigrations: %v", err)
	}

	v, err := GetCodeSchemaVersion(db)
	if err != nil {
		t.Fatalf("GetCodeSchemaVersion: %v", err)
	}
	if v != 2 {
		t.Errorf("expected version 2, got %d", v)
	}

	// Verify data was transformed.
	err = db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketSymbols)
		data := b.Get([]byte("sym-1"))
		if data == nil {
			return fmt.Errorf("sym-1 not found")
		}
		var m map[string]interface{}
		if err := json.Unmarshal(data, &m); err != nil {
			return err
		}
		lang, ok := m["language"]
		if !ok {
			return fmt.Errorf("language field not added")
		}
		if lang != "go" {
			return fmt.Errorf("expected language 'go', got %v", lang)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("data verification failed: %v", err)
	}
}

func TestCodeStoreRunsMigrationsOnOpen(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "aide-code-migrate-open-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "index.db")
	searchPath := filepath.Join(tmpDir, "search.bleve")
	cs, err := NewCodeStore(dbPath, searchPath)
	if err != nil {
		t.Fatalf("NewCodeStore: %v", err)
	}
	defer cs.Close()

	v, err := GetCodeSchemaVersion(cs.db)
	if err != nil {
		t.Fatalf("GetCodeSchemaVersion: %v", err)
	}
	if v != CodeSchemaVersion {
		t.Errorf("expected code schema version %d after NewCodeStore, got %d", CodeSchemaVersion, v)
	}
}

// writeCodeSchemaVersion sets the schema_version in the code meta bucket.
func writeCodeSchemaVersion(t *testing.T, db *bolt.DB, version uint64) {
	t.Helper()
	err := db.Update(func(tx *bolt.Tx) error {
		meta := tx.Bucket(BucketCodeMeta)
		buf := make([]byte, 8)
		binary.BigEndian.PutUint64(buf, version)
		return meta.Put([]byte("schema_version"), buf)
	})
	if err != nil {
		t.Fatalf("failed to write code schema version: %v", err)
	}
}

// =============================================================================
// Independent Version Tracking
// =============================================================================

func TestMainAndCodeVersionsAreIndependent(t *testing.T) {
	// Both stores use different meta buckets, so versions don't interfere.
	tmpDir, err := os.MkdirTemp("", "aide-independent-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := bolt.Open(dbPath, 0o600, nil)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	// Create both meta buckets in the same DB (hypothetical scenario).
	err = db.Update(func(tx *bolt.Tx) error {
		for _, b := range [][]byte{BucketMeta, BucketCodeMeta} {
			if _, err := tx.CreateBucketIfNotExists(b); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("bucket init: %v", err)
	}

	// Run main migrations.
	if err := RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	// Code version should still be 0.
	cv, err := GetCodeSchemaVersion(db)
	if err != nil {
		t.Fatalf("GetCodeSchemaVersion: %v", err)
	}
	if cv != 0 {
		t.Errorf("expected code version 0 (untouched), got %d", cv)
	}

	// Main version should be stamped.
	mv, err := GetSchemaVersion(db)
	if err != nil {
		t.Fatalf("GetSchemaVersion: %v", err)
	}
	if mv != SchemaVersion {
		t.Errorf("expected main version %d, got %d", SchemaVersion, mv)
	}

	// Now run code migrations.
	if err := RunCodeMigrations(db); err != nil {
		t.Fatalf("RunCodeMigrations: %v", err)
	}

	// Both should be at their respective versions.
	mv, _ = GetSchemaVersion(db)
	cv, _ = GetCodeSchemaVersion(db)
	if mv != SchemaVersion {
		t.Errorf("main version changed: got %d", mv)
	}
	if cv != CodeSchemaVersion {
		t.Errorf("code version: got %d, want %d", cv, CodeSchemaVersion)
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
