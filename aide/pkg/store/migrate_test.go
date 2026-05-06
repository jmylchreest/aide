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

// =============================================================================
// Survey Store Migrations
// =============================================================================

// setupSurveyMigrateTestDB creates a fresh BBolt database with the survey meta bucket.
func setupSurveyMigrateTestDB(t *testing.T) (*bolt.DB, func()) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "aide-survey-migrate-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	dbPath := filepath.Join(tmpDir, "survey.db")
	db, err := bolt.Open(dbPath, 0o600, nil)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to open db: %v", err)
	}

	err = db.Update(func(tx *bolt.Tx) error {
		for _, b := range [][]byte{BucketSurvey, BucketSurveyMeta} {
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

// writeSurveySchemaVersion sets the schema_version in the survey meta bucket.
func writeSurveySchemaVersion(t *testing.T, db *bolt.DB, version uint64) {
	t.Helper()
	err := db.Update(func(tx *bolt.Tx) error {
		meta := tx.Bucket(BucketSurveyMeta)
		buf := make([]byte, 8)
		binary.BigEndian.PutUint64(buf, version)
		return meta.Put([]byte("schema_version"), buf)
	})
	if err != nil {
		t.Fatalf("failed to write survey schema version: %v", err)
	}
}

func TestRunSurveyMigrations_FreshDB(t *testing.T) {
	db, cleanup := setupSurveyMigrateTestDB(t)
	defer cleanup()

	v, err := GetSurveySchemaVersion(db)
	if err != nil {
		t.Fatalf("GetSurveySchemaVersion: %v", err)
	}
	if v != 0 {
		t.Fatalf("expected version 0 on fresh db, got %d", v)
	}

	if err := RunSurveyMigrations(db); err != nil {
		t.Fatalf("RunSurveyMigrations: %v", err)
	}

	v, err = GetSurveySchemaVersion(db)
	if err != nil {
		t.Fatalf("GetSurveySchemaVersion: %v", err)
	}
	if v != SurveySchemaVersion {
		t.Errorf("expected version %d, got %d", SurveySchemaVersion, v)
	}
}

func TestRunSurveyMigrations_AlreadyCurrent(t *testing.T) {
	db, cleanup := setupSurveyMigrateTestDB(t)
	defer cleanup()

	writeSurveySchemaVersion(t, db, SurveySchemaVersion)

	if err := RunSurveyMigrations(db); err != nil {
		t.Fatalf("RunSurveyMigrations: %v", err)
	}

	v, err := GetSurveySchemaVersion(db)
	if err != nil {
		t.Fatalf("GetSurveySchemaVersion: %v", err)
	}
	if v != SurveySchemaVersion {
		t.Errorf("expected version %d, got %d", SurveySchemaVersion, v)
	}
}

func TestRunSurveyMigrations_DowngradeError(t *testing.T) {
	db, cleanup := setupSurveyMigrateTestDB(t)
	defer cleanup()

	writeSurveySchemaVersion(t, db, SurveySchemaVersion+10)

	err := RunSurveyMigrations(db)
	if err == nil {
		t.Fatal("expected error for downgrade, got nil")
	}
}

func TestRunSurveyMigrations_AppliesPending(t *testing.T) {
	db, cleanup := setupSurveyMigrateTestDB(t)
	defer cleanup()

	writeSurveySchemaVersion(t, db, 1)

	// Seed a survey entry.
	err := db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketSurvey)
		data, _ := json.Marshal(map[string]interface{}{
			"id":       "entry-1",
			"category": "topology",
		})
		return b.Put([]byte("entry-1"), data)
	})
	if err != nil {
		t.Fatalf("failed to seed survey entry: %v", err)
	}

	origMigrations := surveyMigrations
	origVersion := SurveySchemaVersion
	defer func() {
		surveyMigrations = origMigrations
		SurveySchemaVersion = origVersion
	}()

	SurveySchemaVersion = 2
	surveyMigrations = append(surveyMigrations, migration{
		version:     2,
		description: "add scope field to survey entries",
		migrate: func(tx *bolt.Tx) error {
			b := tx.Bucket(BucketSurvey)
			c := b.Cursor()
			for k, v := c.First(); k != nil; k, v = c.Next() {
				var m map[string]interface{}
				if err := json.Unmarshal(v, &m); err != nil {
					return err
				}
				if _, ok := m["scope"]; !ok {
					m["scope"] = "repo"
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

	if err := RunSurveyMigrations(db); err != nil {
		t.Fatalf("RunSurveyMigrations: %v", err)
	}

	v, err := GetSurveySchemaVersion(db)
	if err != nil {
		t.Fatalf("GetSurveySchemaVersion: %v", err)
	}
	if v != 2 {
		t.Errorf("expected version 2, got %d", v)
	}

	// Verify data was transformed.
	err = db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketSurvey)
		data := b.Get([]byte("entry-1"))
		if data == nil {
			return fmt.Errorf("entry-1 not found")
		}
		var m map[string]interface{}
		if err := json.Unmarshal(data, &m); err != nil {
			return err
		}
		scope, ok := m["scope"]
		if !ok {
			return fmt.Errorf("scope field not added")
		}
		if scope != "repo" {
			return fmt.Errorf("expected scope 'repo', got %v", scope)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("data verification failed: %v", err)
	}
}

// TestRunCodeMigrations_V2BackfillsByFileAndCompositeRefIndex verifies that
// upgrading from v1 to v2 correctly populates BucketSymbolsByFile,
// BucketReferencesByFile, and the new composite-key BucketRefIndex from
// existing symbol/reference rows.
func TestRunCodeMigrations_V2BackfillsByFileAndCompositeRefIndex(t *testing.T) {
	db, cleanup := setupCodeMigrateTestDB(t)
	defer cleanup()

	writeCodeSchemaVersion(t, db, 1)

	// Seed v1 data: two symbols in two different files, two references with a
	// shared SymbolName, and a v1-format BucketRefIndex JSON-slice entry.
	err := db.Update(func(tx *bolt.Tx) error {
		symPayload := func(filePath string) []byte {
			b, _ := json.Marshal(map[string]string{"FilePath": filePath, "Name": "x", "Kind": "function"})
			return b
		}
		if err := tx.Bucket(BucketSymbols).Put([]byte("sym-A"), symPayload("a.go")); err != nil {
			return err
		}
		if err := tx.Bucket(BucketSymbols).Put([]byte("sym-B"), symPayload("b.go")); err != nil {
			return err
		}

		refPayload := func(filePath, name string) []byte {
			b, _ := json.Marshal(map[string]string{"FilePath": filePath, "SymbolName": name})
			return b
		}
		if err := tx.Bucket(BucketReferences).Put([]byte("ref-1"), refPayload("a.go", "doThing")); err != nil {
			return err
		}
		if err := tx.Bucket(BucketReferences).Put([]byte("ref-2"), refPayload("b.go", "doThing")); err != nil {
			return err
		}

		// v1 reverse index: name -> JSON slice of refIDs.
		v1Index, _ := json.Marshal([]string{"ref-1", "ref-2"})
		return tx.Bucket(BucketRefIndex).Put([]byte("doThing"), v1Index)
	})
	if err != nil {
		t.Fatalf("seed v1 data: %v", err)
	}

	if err := RunCodeMigrations(db); err != nil {
		t.Fatalf("RunCodeMigrations: %v", err)
	}

	// Schema stamp at v2.
	if v, err := GetCodeSchemaVersion(db); err != nil || v != CodeSchemaVersion {
		t.Fatalf("expected code schema version %d, got %d (err=%v)", CodeSchemaVersion, v, err)
	}

	err = db.View(func(tx *bolt.Tx) error {
		// Symbols-by-file: one entry per (filePath, symID).
		gotByFile := map[string]bool{}
		_ = tx.Bucket(BucketSymbolsByFile).ForEach(func(k, _ []byte) error {
			gotByFile[string(k)] = true
			return nil
		})
		for _, want := range []string{"a.go\x00sym-A", "b.go\x00sym-B"} {
			if !gotByFile[want] {
				return fmt.Errorf("symbols_by_file missing key %q", want)
			}
		}

		// References-by-file: value carries SymbolName for ClearFileReferences.
		if got := tx.Bucket(BucketReferencesByFile).Get([]byte("a.go\x00ref-1")); string(got) != "doThing" {
			return fmt.Errorf("references_by_file[a.go\\0ref-1] = %q, want doThing", got)
		}
		if got := tx.Bucket(BucketReferencesByFile).Get([]byte("b.go\x00ref-2")); string(got) != "doThing" {
			return fmt.Errorf("references_by_file[b.go\\0ref-2] = %q, want doThing", got)
		}

		// Reverse index: composite-key, one row per ref.
		refIdx := tx.Bucket(BucketRefIndex)
		for _, key := range []string{"doThing\x00ref-1", "doThing\x00ref-2"} {
			if refIdx.Get([]byte(key)) == nil {
				return fmt.Errorf("refindex missing composite key %q", key)
			}
		}
		// Old JSON-slice key must be gone.
		if v := refIdx.Get([]byte("doThing")); v != nil {
			return fmt.Errorf("v1 refindex slice key still present: %q", v)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("verify v2 layout: %v", err)
	}
}

func TestSurveyStoreRunsMigrationsOnOpen(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "aide-survey-migrate-open-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	ss, err := NewSurveyStore(tmpDir)
	if err != nil {
		t.Fatalf("NewSurveyStore: %v", err)
	}
	defer ss.Close()

	v, err := GetSurveySchemaVersion(ss.db)
	if err != nil {
		t.Fatalf("GetSurveySchemaVersion: %v", err)
	}
	if v != SurveySchemaVersion {
		t.Errorf("expected survey schema version %d after NewSurveyStore, got %d", SurveySchemaVersion, v)
	}
}
