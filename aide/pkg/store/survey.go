package store

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/analysis/analyzer/custom"
	"github.com/blevesearch/bleve/v2/analysis/analyzer/keyword"
	"github.com/blevesearch/bleve/v2/analysis/token/lowercase"
	"github.com/blevesearch/bleve/v2/analysis/tokenizer/unicode"
	"github.com/blevesearch/bleve/v2/mapping"
	"github.com/blevesearch/bleve/v2/search/query"
	"github.com/jmylchreest/aide/aide/pkg/survey"
	"github.com/oklog/ulid/v2"
	bolt "go.etcd.io/bbolt"
)

var (
	BucketSurvey     = []byte("survey_entries")
	BucketSurveyMeta = []byte("survey_meta")

	errSurveySearchClosed = fmt.Errorf("survey search index is closed")
)

// SurveyStoreImpl implements SurveyStore using BoltDB + Bleve.
type SurveyStoreImpl struct {
	db         *bolt.DB
	search     bleve.Index
	dbPath     string
	searchPath string
}

// NewSurveyStore opens or creates a survey store at the given directory.
func NewSurveyStore(dir string) (*SurveyStoreImpl, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create survey directory: %w", err)
	}

	dbPath := filepath.Join(dir, "survey.db")
	searchPath := filepath.Join(dir, "search.bleve")

	// Open BBolt database
	db, err := bolt.Open(dbPath, 0o600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("failed to open survey db: %w", err)
	}

	// Initialize buckets
	err = db.Update(func(tx *bolt.Tx) error {
		buckets := [][]byte{BucketSurvey, BucketSurveyMeta}
		for _, bucket := range buckets {
			if _, err := tx.CreateBucketIfNotExists(bucket); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize survey buckets: %w", err)
	}

	if err := RunSurveyMigrations(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("survey schema migration failed: %w", err)
	}

	// Open or create Bleve search index
	index, err := openOrCreateSurveySearchIndex(searchPath)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create/open survey search index: %w", err)
	}

	ss := &SurveyStoreImpl{
		db:         db,
		search:     index,
		dbPath:     dbPath,
		searchPath: searchPath,
	}

	if err := ss.ensureSurveySearchMapping(searchPath); err != nil {
		index.Close()
		db.Close()
		return nil, fmt.Errorf("survey search mapping check failed: %w", err)
	}

	return ss, nil
}

func openOrCreateSurveySearchIndex(path string) (bleve.Index, error) {
	if _, statErr := os.Stat(path); os.IsNotExist(statErr) {
		return createSurveySearchIndex(path)
	}

	index, err := bleve.Open(path)
	if err == nil {
		return index, nil
	}

	log.Printf("survey search index corrupted at %s (%v), rebuilding", path, err)
	if removeErr := os.RemoveAll(path); removeErr != nil {
		return nil, fmt.Errorf("failed to remove corrupted survey search index: %w (original error: %v)", removeErr, err)
	}
	return createSurveySearchIndex(path)
}

func createSurveySearchIndex(path string) (bleve.Index, error) {
	indexMapping, err := buildSurveyIndexMapping()
	if err != nil {
		return nil, err
	}
	return bleve.New(path, indexMapping)
}

func buildSurveyIndexMapping() (mapping.IndexMapping, error) {
	indexMapping := bleve.NewIndexMapping()

	// Standard analyzer with lowercase
	err := indexMapping.AddCustomAnalyzer("standard_lower", map[string]interface{}{
		"type":      custom.Name,
		"tokenizer": unicode.Name,
		"token_filters": []string{
			lowercase.Name,
		},
	})
	if err != nil {
		return nil, err
	}

	// Document mapping for survey entries
	entryMapping := bleve.NewDocumentMapping()

	// Name field (most important for search)
	nameField := bleve.NewTextFieldMapping()
	nameField.Analyzer = "standard_lower"
	nameField.Store = true
	entryMapping.AddFieldMappingsAt("name", nameField)

	// Title field
	titleField := bleve.NewTextFieldMapping()
	titleField.Analyzer = "standard_lower"
	titleField.Store = true
	entryMapping.AddFieldMappingsAt("title", titleField)

	// Detail field
	detailField := bleve.NewTextFieldMapping()
	detailField.Analyzer = "standard_lower"
	detailField.Store = false
	entryMapping.AddFieldMappingsAt("detail", detailField)

	// Keyword fields (exact match filtering)
	analyzerField := bleve.NewTextFieldMapping()
	analyzerField.Analyzer = keyword.Name
	entryMapping.AddFieldMappingsAt("analyzer", analyzerField)

	kindField := bleve.NewTextFieldMapping()
	kindField.Analyzer = keyword.Name
	entryMapping.AddFieldMappingsAt("kind", kindField)

	fileField := bleve.NewTextFieldMapping()
	fileField.Analyzer = keyword.Name
	entryMapping.AddFieldMappingsAt("file", fileField)

	indexMapping.AddDocumentMapping("survey_entry", entryMapping)
	indexMapping.DefaultMapping = entryMapping

	return indexMapping, nil
}

func (s *SurveyStoreImpl) ensureSurveySearchMapping(searchPath string) error {
	m, err := buildSurveyIndexMapping()
	if err != nil {
		return err
	}
	hash := MappingHash(m)

	var stored string
	if err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketSurveyMeta)
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
		log.Printf("store: survey search mapping changed, rebuilding index")
	}

	// Close current index, remove, recreate.
	if err := s.search.Close(); err != nil {
		return fmt.Errorf("failed to close survey search for rebuild: %w", err)
	}
	if err := os.RemoveAll(searchPath); err != nil {
		return fmt.Errorf("failed to remove survey search for rebuild: %w", err)
	}

	indexMapping, err := buildSurveyIndexMapping()
	if err != nil {
		return err
	}
	index, err := bleve.New(searchPath, indexMapping)
	if err != nil {
		return err
	}
	s.search = index

	// Re-index all entries from BBolt.
	err = s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketSurvey)
		c := b.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var e survey.Entry
			if err := json.Unmarshal(v, &e); err != nil {
				continue
			}
			doc := surveyEntryToSearchDoc(&e)
			if err := s.search.Index(e.ID, doc); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketSurveyMeta)
		if b == nil {
			return fmt.Errorf("survey meta bucket not found")
		}
		return b.Put([]byte("search_mapping_hash"), []byte(hash))
	})
}

func surveyEntryToSearchDoc(e *survey.Entry) map[string]interface{} {
	return map[string]interface{}{
		"name":     e.Name,
		"title":    e.Title,
		"detail":   e.Detail,
		"analyzer": e.Analyzer,
		"kind":     e.Kind,
		"file":     e.FilePath,
	}
}

// Close closes the survey store.
func (s *SurveyStoreImpl) Close() error {
	var errs []error
	if s.search != nil {
		if err := s.search.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close survey search: %w", err))
		}
	}
	if s.db != nil {
		if err := s.db.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close survey db: %w", err))
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

// AddEntry stores a survey entry and indexes it for search.
func (s *SurveyStoreImpl) AddEntry(e *survey.Entry) error {
	if s.search == nil {
		return errSurveySearchClosed
	}
	if e.ID == "" {
		e.ID = ulid.Make().String()
	}
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now()
	}

	data, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("failed to marshal survey entry: %w", err)
	}

	err = s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(BucketSurvey).Put([]byte(e.ID), data)
	})
	if err != nil {
		return err
	}

	return s.search.Index(e.ID, surveyEntryToSearchDoc(e))
}

// GetEntry retrieves a survey entry by ID.
func (s *SurveyStoreImpl) GetEntry(id string) (*survey.Entry, error) {
	var e survey.Entry
	err := s.db.View(func(tx *bolt.Tx) error {
		data := tx.Bucket(BucketSurvey).Get([]byte(id))
		if data == nil {
			return ErrNotFound
		}
		return json.Unmarshal(data, &e)
	})
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// DeleteEntry removes a survey entry by ID.
func (s *SurveyStoreImpl) DeleteEntry(id string) error {
	if s.search == nil {
		return errSurveySearchClosed
	}
	err := s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(BucketSurvey).Delete([]byte(id))
	})
	if err != nil {
		return err
	}
	return s.search.Delete(id)
}

// SearchEntries performs full-text search on survey entries.
func (s *SurveyStoreImpl) SearchEntries(queryStr string, opts survey.SearchOptions) ([]*survey.SearchResult, error) {
	if s.search == nil {
		return nil, errSurveySearchClosed
	}
	limit := opts.Limit
	if limit == 0 {
		limit = survey.DefaultSearchLimit
	} else if limit < 0 {
		limit = 100_000 // Effectively unlimited for bleve.
	}

	// Build compound query
	var queries []query.Query
	if queryStr != "" {
		queries = append(queries, bleve.NewQueryStringQuery(queryStr))
	}
	if opts.Analyzer != "" {
		q := bleve.NewTermQuery(opts.Analyzer)
		q.SetField("analyzer")
		queries = append(queries, q)
	}
	if opts.Kind != "" {
		q := bleve.NewTermQuery(opts.Kind)
		q.SetField("kind")
		queries = append(queries, q)
	}
	if opts.FilePath != "" {
		q := bleve.NewTermQuery(opts.FilePath)
		q.SetField("file")
		queries = append(queries, q)
	}

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
	result, err := s.search.Search(req)
	if err != nil {
		return nil, fmt.Errorf("survey search failed: %w", err)
	}

	var results []*survey.SearchResult
	for _, hit := range result.Hits {
		e, err := s.GetEntry(hit.ID)
		if err != nil {
			continue
		}
		results = append(results, &survey.SearchResult{
			Entry: e,
			Score: hit.Score,
		})
	}

	return results, nil
}

// ListEntries returns survey entries filtered by options (no full-text search).
func (s *SurveyStoreImpl) ListEntries(opts survey.SearchOptions) ([]*survey.Entry, error) {
	limit := opts.Limit
	if limit == 0 {
		limit = survey.DefaultListLimit
	} else if limit < 0 {
		limit = 0 // Negative means no limit; the >0 check below will never break.
	}

	var result []*survey.Entry
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketSurvey)
		c := b.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var e survey.Entry
			if err := json.Unmarshal(v, &e); err != nil {
				continue
			}
			if opts.Analyzer != "" && e.Analyzer != opts.Analyzer {
				continue
			}
			if opts.Kind != "" && e.Kind != opts.Kind {
				continue
			}
			if opts.FilePath != "" && !strings.Contains(e.FilePath, opts.FilePath) {
				continue
			}
			result = append(result, &e)
			if limit > 0 && len(result) >= limit {
				break
			}
		}
		return nil
	})
	return result, err
}

// GetFileEntries returns all survey entries for a specific file.
func (s *SurveyStoreImpl) GetFileEntries(filePath string) ([]*survey.Entry, error) {
	return s.ListEntries(survey.SearchOptions{FilePath: filePath, Limit: 1000})
}

// ClearAnalyzer removes all survey entries for a specific analyzer.
func (s *SurveyStoreImpl) ClearAnalyzer(analyzer string) (int, error) {
	var toDelete []string
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketSurvey)
		c := b.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var e survey.Entry
			if err := json.Unmarshal(v, &e); err != nil {
				continue
			}
			if e.Analyzer == analyzer {
				toDelete = append(toDelete, e.ID)
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}

	for _, id := range toDelete {
		if err := s.DeleteEntry(id); err != nil {
			return 0, err
		}
	}
	return len(toDelete), nil
}

// Stats returns aggregate survey entry counts, optionally filtering by SearchOptions.
func (s *SurveyStoreImpl) Stats(opts survey.SearchOptions) (*survey.Stats, error) {
	stats := &survey.Stats{
		ByAnalyzer: make(map[string]int),
		ByKind:     make(map[string]int),
	}

	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketSurvey)
		c := b.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var e survey.Entry
			if err := json.Unmarshal(v, &e); err != nil {
				continue
			}
			if opts.Analyzer != "" && e.Analyzer != opts.Analyzer {
				continue
			}
			if opts.Kind != "" && e.Kind != opts.Kind {
				continue
			}
			if opts.FilePath != "" && !strings.Contains(e.FilePath, opts.FilePath) {
				continue
			}
			stats.Total++
			stats.ByAnalyzer[e.Analyzer]++
			stats.ByKind[e.Kind]++
		}
		return nil
	})
	return stats, err
}

// ReplaceEntriesForAnalyzer atomically replaces all entries for an analyzer.
// On success, old entries are gone and new ones are stored.
// On error, old entries remain untouched.
func (s *SurveyStoreImpl) ReplaceEntriesForAnalyzer(analyzer string, newEntries []*survey.Entry) error {
	return s.replaceEntries(func(e *survey.Entry) bool {
		return e.Analyzer == analyzer
	}, newEntries)
}

// ReplaceEntriesForAnalyzerAndFile atomically replaces entries for an analyzer within a specific file.
func (s *SurveyStoreImpl) ReplaceEntriesForAnalyzerAndFile(analyzer, filePath string, newEntries []*survey.Entry) error {
	return s.replaceEntries(func(e *survey.Entry) bool {
		return e.Analyzer == analyzer && e.FilePath == filePath
	}, newEntries)
}

// replaceEntries atomically replaces entries matching shouldDelete with newEntries.
// Collects keys to delete and new data inside the BBolt tx, then applies
// Bleve mutations outside so a tx rollback doesn't leave Bleve inconsistent.
func (s *SurveyStoreImpl) replaceEntries(shouldDelete func(*survey.Entry) bool, newEntries []*survey.Entry) error {
	if s.search == nil {
		return errSurveySearchClosed
	}
	type pendingPut struct {
		id  string
		doc map[string]interface{}
	}
	var deleteIDs []string
	var puts []pendingPut

	err := s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketSurvey)
		c := b.Cursor()

		for k, v := c.First(); k != nil; k, v = c.Next() {
			var e survey.Entry
			if err := json.Unmarshal(v, &e); err != nil {
				continue
			}
			if shouldDelete(&e) {
				// Copy key — cursor keys are only valid for the current position.
				deleteIDs = append(deleteIDs, string(append([]byte(nil), k...)))
			}
		}

		for _, id := range deleteIDs {
			if err := b.Delete([]byte(id)); err != nil {
				return err
			}
		}

		for _, e := range newEntries {
			if e.ID == "" {
				e.ID = ulid.Make().String()
			}
			if e.CreatedAt.IsZero() {
				e.CreatedAt = time.Now()
			}

			data, err := json.Marshal(e)
			if err != nil {
				return fmt.Errorf("marshal survey entry: %w", err)
			}
			if err := b.Put([]byte(e.ID), data); err != nil {
				return err
			}
			puts = append(puts, pendingPut{id: e.ID, doc: surveyEntryToSearchDoc(e)})
		}

		return nil
	})
	if err != nil {
		return err
	}

	// Apply Bleve mutations outside the BBolt tx for atomicity.
	for _, id := range deleteIDs {
		if err := s.search.Delete(id); err != nil {
			log.Printf("store: warning: failed to delete survey entry %s from search index: %v", id, err)
		}
	}
	for _, p := range puts {
		if err := s.search.Index(p.id, p.doc); err != nil {
			return err
		}
	}
	return nil
}

// Clear removes all survey entries.
func (s *SurveyStoreImpl) Clear() error {
	err := s.db.Update(func(tx *bolt.Tx) error {
		if err := tx.DeleteBucket(BucketSurvey); err != nil {
			return err
		}
		_, err := tx.CreateBucket(BucketSurvey)
		return err
	})
	if err != nil {
		return err
	}

	searchPath := s.searchPath
	if err := s.search.Close(); err != nil {
		return fmt.Errorf("failed to close survey search index: %w", err)
	}
	if err := os.RemoveAll(searchPath); err != nil {
		// Reopen the old index so the store remains usable.
		if idx, reopenErr := bleve.Open(searchPath); reopenErr == nil {
			s.search = idx
		} else {
			s.search = nil
		}
		return fmt.Errorf("failed to remove survey search index: %w", err)
	}

	indexMapping, err := buildSurveyIndexMapping()
	if err != nil {
		s.search = nil
		return fmt.Errorf("failed to build survey index mapping after clear: %w", err)
	}
	index, err := bleve.New(searchPath, indexMapping)
	if err != nil {
		s.search = nil
		return fmt.Errorf("failed to recreate survey search index after clear: %w", err)
	}
	s.search = index

	return nil
}
