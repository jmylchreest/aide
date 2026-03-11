package store

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/mapping"
	"github.com/blevesearch/bleve/v2/search/query"
	"github.com/oklog/ulid/v2"
	bolt "go.etcd.io/bbolt"
)

// searchableStoreConfig configures a new searchableStore.
type searchableStoreConfig[T any] struct {
	// Dir is the directory for the store's database and search index files.
	Dir string
	// DBFilename is the name of the BoltDB file (e.g. "findings.db", "survey.db").
	DBFilename string
	// BucketName is the BoltDB bucket for storing entities.
	BucketName []byte
	// MetaBucketName is the BoltDB bucket for storing metadata (e.g. mapping hashes).
	MetaBucketName []byte
	// StoreName is used in log messages (e.g. "findings", "survey").
	StoreName string

	// BuildIndexMapping returns the Bleve index mapping for this store.
	BuildIndexMapping func() (mapping.IndexMapping, error)

	// ToSearchDoc converts a domain entity to a Bleve search document.
	ToSearchDoc func(*T) map[string]interface{}

	// GetID returns the entity's ID.
	GetID func(*T) string
	// SetID sets the entity's ID.
	SetID func(*T, string)
	// GetCreatedAt returns the entity's created timestamp.
	GetCreatedAt func(*T) time.Time
	// SetCreatedAt sets the entity's created timestamp.
	SetCreatedAt func(*T, time.Time)
	// GetAnalyzer returns the entity's analyzer field (for ClearAnalyzer).
	GetAnalyzer func(*T) string

	// RunMigrations runs domain-specific migrations on the BoltDB.
	RunMigrations func(*bolt.DB) error

	// LegacySearchPaths are alternative search index paths to check before the default.
	// The first path that exists on disk will be used as the search index location.
	LegacySearchPaths []string
}

// searchableStore is a generic BoltDB+Bleve store for domain entities.
// It encapsulates the common CRUD, search, list, replace, clear, and close patterns
// shared by findings and survey stores.
type searchableStore[T any] struct {
	db         *bolt.DB
	idx        bleve.Index
	dbPath     string
	searchPath string
	cfg        searchableStoreConfig[T]
}

// newSearchableStore opens or creates a searchable store at the given directory.
func newSearchableStore[T any](cfg searchableStoreConfig[T]) (*searchableStore[T], error) {
	if err := os.MkdirAll(cfg.Dir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create %s directory: %w", cfg.StoreName, err)
	}

	dbPath := filepath.Join(cfg.Dir, cfg.DBFilename)

	// Determine search path: check legacy paths first, fall back to search.bleve.
	searchPath := filepath.Join(cfg.Dir, "search.bleve")
	for _, legacy := range cfg.LegacySearchPaths {
		p := filepath.Join(cfg.Dir, legacy)
		if _, statErr := os.Stat(p); statErr == nil {
			searchPath = p
			break
		}
	}

	// Open BBolt database.
	db, err := bolt.Open(dbPath, 0o600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("failed to open %s db: %w", cfg.StoreName, err)
	}

	// Initialize buckets.
	err = db.Update(func(tx *bolt.Tx) error {
		buckets := [][]byte{cfg.BucketName, cfg.MetaBucketName}
		for _, bucket := range buckets {
			if _, err := tx.CreateBucketIfNotExists(bucket); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize %s buckets: %w", cfg.StoreName, err)
	}

	if cfg.RunMigrations != nil {
		if err := cfg.RunMigrations(db); err != nil {
			db.Close()
			return nil, fmt.Errorf("%s schema migration failed: %w", cfg.StoreName, err)
		}
	}

	// Open or create Bleve search index.
	index, err := openOrCreateBleveIndex(searchPath, cfg.BuildIndexMapping, cfg.StoreName)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create/open %s search index: %w", cfg.StoreName, err)
	}

	ss := &searchableStore[T]{
		db:         db,
		idx:        index,
		dbPath:     dbPath,
		searchPath: searchPath,
		cfg:        cfg,
	}

	if err := ss.ensureSearchMapping(searchPath); err != nil {
		index.Close()
		db.Close()
		return nil, fmt.Errorf("%s search mapping check failed: %w", cfg.StoreName, err)
	}

	return ss, nil
}

// openOrCreateBleveIndex opens an existing Bleve index or creates a new one.
func openOrCreateBleveIndex(path string, buildMapping func() (mapping.IndexMapping, error), storeName string) (bleve.Index, error) {
	if _, statErr := os.Stat(path); os.IsNotExist(statErr) {
		return createBleveIndex(path, buildMapping)
	}

	index, err := bleve.Open(path)
	if err == nil {
		return index, nil
	}

	log.Printf("%s search index corrupted at %s (%v), rebuilding", storeName, path, err)
	if removeErr := os.RemoveAll(path); removeErr != nil {
		return nil, fmt.Errorf("failed to remove corrupted %s search index: %w (original error: %v)", storeName, removeErr, err)
	}
	return createBleveIndex(path, buildMapping)
}

// createBleveIndex creates a new Bleve search index.
func createBleveIndex(path string, buildMapping func() (mapping.IndexMapping, error)) (bleve.Index, error) {
	indexMapping, err := buildMapping()
	if err != nil {
		return nil, err
	}
	return bleve.New(path, indexMapping)
}

// ensureSearchMapping checks whether the search index mapping has changed and rebuilds if needed.
func (s *searchableStore[T]) ensureSearchMapping(searchPath string) error {
	m, err := s.cfg.BuildIndexMapping()
	if err != nil {
		return err
	}
	hash := MappingHash(m)

	var stored string
	if err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(s.cfg.MetaBucketName)
		if b == nil {
			return nil
		}
		data := b.Get([]byte("search_mapping_hash"))
		if data != nil {
			stored = string(data)
		}
		return nil
	}); err != nil {
		return err
	}

	if hash == stored {
		return nil
	}

	if stored != "" {
		log.Printf("store: %s search mapping changed, rebuilding index", s.cfg.StoreName)
	}

	// Close current index, remove, recreate.
	if err := s.idx.Close(); err != nil {
		return fmt.Errorf("failed to close %s search for rebuild: %w", s.cfg.StoreName, err)
	}
	if err := os.RemoveAll(searchPath); err != nil {
		return fmt.Errorf("failed to remove %s search for rebuild: %w", s.cfg.StoreName, err)
	}

	indexMapping, err := s.cfg.BuildIndexMapping()
	if err != nil {
		return err
	}
	index, err := bleve.New(searchPath, indexMapping)
	if err != nil {
		return err
	}
	s.idx = index

	// Re-index all items from BBolt.
	err = s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(s.cfg.BucketName)
		c := b.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var item T
			if err := json.Unmarshal(v, &item); err != nil {
				continue
			}
			doc := s.cfg.ToSearchDoc(&item)
			id := s.cfg.GetID(&item)
			if err := s.idx.Index(id, doc); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(s.cfg.MetaBucketName)
		if b == nil {
			return fmt.Errorf("%s meta bucket not found", s.cfg.StoreName)
		}
		return b.Put([]byte("search_mapping_hash"), []byte(hash))
	})
}

// errSearchIndexClosed returns a formatted error for when the search index is closed.
func (s *searchableStore[T]) errSearchIndexClosed() error {
	return fmt.Errorf("%s search index is closed", s.cfg.StoreName)
}

// Close closes both the Bleve search index and the BoltDB database.
func (s *searchableStore[T]) Close() error {
	var errs []error
	if s.idx != nil {
		if err := s.idx.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close %s search: %w", s.cfg.StoreName, err))
		}
	}
	if s.db != nil {
		if err := s.db.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close %s db: %w", s.cfg.StoreName, err))
		}
	}
	if len(errs) == 1 {
		return errs[0]
	}
	if len(errs) > 1 {
		return fmt.Errorf("%v; %v", errs[0], errs[1])
	}
	return nil
}

// Add stores an entity and indexes it for search.
func (s *searchableStore[T]) Add(item *T) error {
	if s.idx == nil {
		return s.errSearchIndexClosed()
	}
	if s.cfg.GetID(item) == "" {
		s.cfg.SetID(item, ulid.Make().String())
	}
	if s.cfg.GetCreatedAt(item).IsZero() {
		s.cfg.SetCreatedAt(item, time.Now())
	}

	data, err := json.Marshal(item)
	if err != nil {
		return fmt.Errorf("failed to marshal %s: %w", s.cfg.StoreName, err)
	}

	id := s.cfg.GetID(item)
	err = s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(s.cfg.BucketName).Put([]byte(id), data)
	})
	if err != nil {
		return err
	}

	return s.idx.Index(id, s.cfg.ToSearchDoc(item))
}

// Get retrieves an entity by ID.
func (s *searchableStore[T]) Get(id string) (*T, error) {
	var item T
	err := s.db.View(func(tx *bolt.Tx) error {
		data := tx.Bucket(s.cfg.BucketName).Get([]byte(id))
		if data == nil {
			return ErrNotFound
		}
		return json.Unmarshal(data, &item)
	})
	if err != nil {
		return nil, err
	}
	return &item, nil
}

// updateItem re-stores an entity and re-indexes it. Used for in-place mutations
// (e.g. marking a finding as accepted) where the caller already has the modified entity.
func (s *searchableStore[T]) updateItem(item *T) error {
	if s.idx == nil {
		return s.errSearchIndexClosed()
	}
	id := s.cfg.GetID(item)
	data, err := json.Marshal(item)
	if err != nil {
		return fmt.Errorf("marshal %s: %w", s.cfg.StoreName, err)
	}
	if err := s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(s.cfg.BucketName).Put([]byte(id), data)
	}); err != nil {
		return err
	}
	return s.idx.Index(id, s.cfg.ToSearchDoc(item))
}

// Delete removes an entity by ID from both BoltDB and the search index.
func (s *searchableStore[T]) Delete(id string) error {
	if s.idx == nil {
		return s.errSearchIndexClosed()
	}
	err := s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(s.cfg.BucketName).Delete([]byte(id))
	})
	if err != nil {
		return err
	}
	return s.idx.Delete(id)
}

// ClearAnalyzer removes all entities for a specific analyzer.
func (s *searchableStore[T]) ClearAnalyzer(analyzer string) (int, error) {
	var toDelete []string
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(s.cfg.BucketName)
		c := b.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var item T
			if err := json.Unmarshal(v, &item); err != nil {
				continue
			}
			if s.cfg.GetAnalyzer(&item) == analyzer {
				toDelete = append(toDelete, s.cfg.GetID(&item))
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}

	for _, id := range toDelete {
		if err := s.Delete(id); err != nil {
			return 0, err
		}
	}
	return len(toDelete), nil
}

// bleveSearchHit holds the ID and score from a Bleve search hit.
type bleveSearchHit struct {
	ID    string
	Score float64
}

// search executes a Bleve search query and returns raw hits.
// The caller builds domain-specific filter queries; this method handles
// the common query construction and execution.
func (s *searchableStore[T]) searchBleve(queryStr string, filters []query.Query, limit, defaultLimit int) ([]bleveSearchHit, error) {
	if s.idx == nil {
		return nil, s.errSearchIndexClosed()
	}
	if limit == 0 {
		limit = defaultLimit
	} else if limit < 0 {
		limit = 100_000 // Effectively unlimited for bleve.
	}

	// Build compound query.
	var queries []query.Query
	if queryStr != "" {
		queries = append(queries, bleve.NewQueryStringQuery(queryStr))
	}
	queries = append(queries, filters...)

	var searchQuery query.Query
	switch len(queries) {
	case 0:
		searchQuery = bleve.NewMatchAllQuery()
	case 1:
		searchQuery = queries[0]
	default:
		searchQuery = bleve.NewConjunctionQuery(queries...)
	}

	req := bleve.NewSearchRequestOptions(searchQuery, limit, 0, false)
	result, err := s.idx.Search(req)
	if err != nil {
		return nil, fmt.Errorf("%s search failed: %w", s.cfg.StoreName, err)
	}

	hits := make([]bleveSearchHit, 0, len(result.Hits))
	for _, hit := range result.Hits {
		hits = append(hits, bleveSearchHit{ID: hit.ID, Score: hit.Score})
	}
	return hits, nil
}

// list iterates over all entities in BoltDB and returns those matching the predicate.
func (s *searchableStore[T]) list(matchFn func(*T) bool, limit, defaultLimit int) ([]*T, error) {
	if limit == 0 {
		limit = defaultLimit
	} else if limit < 0 {
		limit = 0 // Negative means no limit; the >0 check below will never break.
	}

	var result []*T
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(s.cfg.BucketName)
		c := b.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var item T
			if err := json.Unmarshal(v, &item); err != nil {
				continue
			}
			if matchFn != nil && !matchFn(&item) {
				continue
			}
			result = append(result, &item)
			if limit > 0 && len(result) >= limit {
				break
			}
		}
		return nil
	})
	return result, err
}

// allMatching iterates over all entities and returns those matching the predicate.
// Unlike list, it has no limit — used for stats aggregation.
func (s *searchableStore[T]) allMatching(matchFn func(*T) bool) ([]*T, error) {
	var result []*T
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(s.cfg.BucketName)
		c := b.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var item T
			if err := json.Unmarshal(v, &item); err != nil {
				continue
			}
			if matchFn != nil && !matchFn(&item) {
				continue
			}
			result = append(result, &item)
		}
		return nil
	})
	return result, err
}

// replace atomically replaces entities matching shouldDelete with newItems.
// It collects keys to delete and new data inside the BBolt tx, then applies
// Bleve mutations outside so a tx rollback doesn't leave Bleve inconsistent.
func (s *searchableStore[T]) replace(shouldDelete func(*T) bool, newItems []*T) error {
	if s.idx == nil {
		return s.errSearchIndexClosed()
	}
	type pendingPut struct {
		id  string
		doc map[string]interface{}
	}
	var deleteIDs []string
	var puts []pendingPut

	err := s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(s.cfg.BucketName)
		c := b.Cursor()

		for k, v := c.First(); k != nil; k, v = c.Next() {
			var item T
			if err := json.Unmarshal(v, &item); err != nil {
				continue
			}
			if shouldDelete(&item) {
				// Copy key — cursor keys are only valid for the current position.
				deleteIDs = append(deleteIDs, string(append([]byte(nil), k...)))
			}
		}

		for _, id := range deleteIDs {
			if err := b.Delete([]byte(id)); err != nil {
				return err
			}
		}

		for _, item := range newItems {
			if s.cfg.GetID(item) == "" {
				s.cfg.SetID(item, ulid.Make().String())
			}
			if s.cfg.GetCreatedAt(item).IsZero() {
				s.cfg.SetCreatedAt(item, time.Now())
			}

			data, err := json.Marshal(item)
			if err != nil {
				return fmt.Errorf("marshal %s: %w", s.cfg.StoreName, err)
			}
			id := s.cfg.GetID(item)
			if err := b.Put([]byte(id), data); err != nil {
				return err
			}
			puts = append(puts, pendingPut{id: id, doc: s.cfg.ToSearchDoc(item)})
		}

		return nil
	})
	if err != nil {
		return err
	}

	// Apply Bleve mutations outside the BBolt tx for atomicity.
	for _, id := range deleteIDs {
		if err := s.idx.Delete(id); err != nil {
			log.Printf("store: warning: failed to delete %s %s from search index: %v", s.cfg.StoreName, id, err)
		}
	}
	for _, p := range puts {
		if err := s.idx.Index(p.id, p.doc); err != nil {
			return err
		}
	}
	return nil
}

// Clear removes all entities by deleting and recreating the BoltDB bucket
// and the Bleve search index.
func (s *searchableStore[T]) Clear() error {
	err := s.db.Update(func(tx *bolt.Tx) error {
		if err := tx.DeleteBucket(s.cfg.BucketName); err != nil {
			return err
		}
		_, err := tx.CreateBucket(s.cfg.BucketName)
		return err
	})
	if err != nil {
		return err
	}

	searchPath := s.searchPath
	if err := s.idx.Close(); err != nil {
		return fmt.Errorf("failed to close %s search index: %w", s.cfg.StoreName, err)
	}
	if err := os.RemoveAll(searchPath); err != nil {
		// Reopen the old index so the store remains usable.
		if idx, reopenErr := bleve.Open(searchPath); reopenErr == nil {
			s.idx = idx
		} else {
			s.idx = nil
		}
		return fmt.Errorf("failed to remove %s search index: %w", s.cfg.StoreName, err)
	}

	indexMapping, err := s.cfg.BuildIndexMapping()
	if err != nil {
		s.idx = nil
		return fmt.Errorf("failed to build %s index mapping after clear: %w", s.cfg.StoreName, err)
	}
	index, err := bleve.New(searchPath, indexMapping)
	if err != nil {
		s.idx = nil
		return fmt.Errorf("failed to recreate %s search index after clear: %w", s.cfg.StoreName, err)
	}
	s.idx = index

	return nil
}
