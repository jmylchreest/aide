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
	"github.com/jmylchreest/aide/aide/pkg/findings"
	"github.com/oklog/ulid/v2"
	bolt "go.etcd.io/bbolt"
)

var (
	BucketFindings     = []byte("findings")
	BucketFindingsMeta = []byte("findings_meta")
)

// FindingsStoreImpl implements FindingsStore using BoltDB + Bleve.
type FindingsStoreImpl struct {
	db     *bolt.DB
	search bleve.Index
	dbPath string
}

// NewFindingsStore opens or creates a findings store at the given directory.
func NewFindingsStore(dir string) (*FindingsStoreImpl, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create findings directory: %w", err)
	}

	dbPath := filepath.Join(dir, "findings.db")
	searchPath := filepath.Join(dir, "findings.idx")

	// Open BBolt database
	db, err := bolt.Open(dbPath, 0o600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("failed to open findings db: %w", err)
	}

	// Initialize buckets
	err = db.Update(func(tx *bolt.Tx) error {
		buckets := [][]byte{BucketFindings, BucketFindingsMeta}
		for _, bucket := range buckets {
			if _, err := tx.CreateBucketIfNotExists(bucket); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize findings buckets: %w", err)
	}

	if err := RunFindingsMigrations(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("findings schema migration failed: %w", err)
	}

	// Open or create Bleve search index
	index, err := openOrCreateFindingsSearchIndex(searchPath)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create/open findings search index: %w", err)
	}

	fs := &FindingsStoreImpl{
		db:     db,
		search: index,
		dbPath: dbPath,
	}

	if err := fs.ensureFindingsSearchMapping(searchPath); err != nil {
		index.Close()
		db.Close()
		return nil, fmt.Errorf("findings search mapping check failed: %w", err)
	}

	return fs, nil
}

func openOrCreateFindingsSearchIndex(path string) (bleve.Index, error) {
	if _, statErr := os.Stat(path); os.IsNotExist(statErr) {
		return createFindingsSearchIndex(path)
	}

	index, err := bleve.Open(path)
	if err == nil {
		return index, nil
	}

	log.Printf("findings search index corrupted at %s (%v), rebuilding", path, err)
	if removeErr := os.RemoveAll(path); removeErr != nil {
		return nil, fmt.Errorf("failed to remove corrupted findings search index: %w (original error: %v)", removeErr, err)
	}
	return createFindingsSearchIndex(path)
}

func createFindingsSearchIndex(path string) (bleve.Index, error) {
	indexMapping, err := buildFindingsIndexMapping()
	if err != nil {
		return nil, err
	}
	return bleve.New(path, indexMapping)
}

func buildFindingsIndexMapping() (mapping.IndexMapping, error) {
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

	// Document mapping for findings
	findingMapping := bleve.NewDocumentMapping()

	// Title field (most important)
	titleField := bleve.NewTextFieldMapping()
	titleField.Analyzer = "standard_lower"
	titleField.Store = true
	findingMapping.AddFieldMappingsAt("title", titleField)

	// Detail field
	detailField := bleve.NewTextFieldMapping()
	detailField.Analyzer = "standard_lower"
	detailField.Store = false
	findingMapping.AddFieldMappingsAt("detail", detailField)

	// Keyword fields (exact match filtering)
	analyzerField := bleve.NewTextFieldMapping()
	analyzerField.Analyzer = keyword.Name
	findingMapping.AddFieldMappingsAt("analyzer", analyzerField)

	severityField := bleve.NewTextFieldMapping()
	severityField.Analyzer = keyword.Name
	findingMapping.AddFieldMappingsAt("severity", severityField)

	categoryField := bleve.NewTextFieldMapping()
	categoryField.Analyzer = keyword.Name
	findingMapping.AddFieldMappingsAt("category", categoryField)

	fileField := bleve.NewTextFieldMapping()
	fileField.Analyzer = keyword.Name
	findingMapping.AddFieldMappingsAt("file", fileField)

	indexMapping.AddDocumentMapping("finding", findingMapping)
	indexMapping.DefaultMapping = findingMapping

	return indexMapping, nil
}

func (s *FindingsStoreImpl) ensureFindingsSearchMapping(searchPath string) error {
	m, err := buildFindingsIndexMapping()
	if err != nil {
		return err
	}
	hash := MappingHash(m)

	var stored string
	s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketFindingsMeta)
		if b == nil {
			return nil
		}
		data := b.Get([]byte("search_mapping_hash"))
		if data != nil {
			stored = string(data)
		}
		return nil
	})

	if hash == stored {
		return nil
	}

	if stored != "" {
		log.Printf("store: findings search mapping changed, rebuilding index")
	}

	// Close current index, remove, recreate.
	s.search.Close()
	os.RemoveAll(searchPath)

	indexMapping, err := buildFindingsIndexMapping()
	if err != nil {
		return err
	}
	index, err := bleve.New(searchPath, indexMapping)
	if err != nil {
		return err
	}
	s.search = index

	// Re-index all findings from BBolt.
	err = s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketFindings)
		c := b.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var f findings.Finding
			if err := json.Unmarshal(v, &f); err != nil {
				continue
			}
			doc := findingToSearchDoc(&f)
			if err := s.search.Index(f.ID, doc); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketFindingsMeta)
		if b == nil {
			return fmt.Errorf("findings meta bucket not found")
		}
		return b.Put([]byte("search_mapping_hash"), []byte(hash))
	})
}

func findingToSearchDoc(f *findings.Finding) map[string]interface{} {
	return map[string]interface{}{
		"title":    f.Title,
		"detail":   f.Detail,
		"analyzer": f.Analyzer,
		"severity": f.Severity,
		"category": f.Category,
		"file":     f.FilePath,
	}
}

// Close closes the findings store.
func (s *FindingsStoreImpl) Close() error {
	if s.search != nil {
		s.search.Close()
	}
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// AddFinding stores a finding and indexes it for search.
func (s *FindingsStoreImpl) AddFinding(f *findings.Finding) error {
	if f.ID == "" {
		f.ID = ulid.Make().String()
	}
	if f.CreatedAt.IsZero() {
		f.CreatedAt = time.Now()
	}

	data, err := json.Marshal(f)
	if err != nil {
		return fmt.Errorf("failed to marshal finding: %w", err)
	}

	err = s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(BucketFindings).Put([]byte(f.ID), data)
	})
	if err != nil {
		return err
	}

	return s.search.Index(f.ID, findingToSearchDoc(f))
}

// GetFinding retrieves a finding by ID.
func (s *FindingsStoreImpl) GetFinding(id string) (*findings.Finding, error) {
	var f findings.Finding
	err := s.db.View(func(tx *bolt.Tx) error {
		data := tx.Bucket(BucketFindings).Get([]byte(id))
		if data == nil {
			return ErrNotFound
		}
		return json.Unmarshal(data, &f)
	})
	if err != nil {
		return nil, err
	}
	return &f, nil
}

// DeleteFinding removes a finding by ID.
func (s *FindingsStoreImpl) DeleteFinding(id string) error {
	err := s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(BucketFindings).Delete([]byte(id))
	})
	if err != nil {
		return err
	}
	return s.search.Delete(id)
}

// SearchFindings performs full-text search on findings.
func (s *FindingsStoreImpl) SearchFindings(queryStr string, opts findings.SearchOptions) ([]*findings.SearchResult, error) {
	limit := opts.Limit
	if limit == 0 {
		limit = 20
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
	if opts.Severity != "" {
		q := bleve.NewTermQuery(opts.Severity)
		q.SetField("severity")
		queries = append(queries, q)
	}
	if opts.FilePath != "" {
		q := bleve.NewTermQuery(opts.FilePath)
		q.SetField("file")
		queries = append(queries, q)
	}
	if opts.Category != "" {
		q := bleve.NewTermQuery(opts.Category)
		q.SetField("category")
		queries = append(queries, q)
	}

	var searchQuery query.Query
	if len(queries) == 0 {
		searchQuery = bleve.NewMatchAllQuery()
	} else if len(queries) == 1 {
		searchQuery = queries[0]
	} else {
		searchQuery = bleve.NewConjunctionQuery(queries...)
	}

	req := bleve.NewSearchRequestOptions(searchQuery, limit, 0, false)
	result, err := s.search.Search(req)
	if err != nil {
		return nil, fmt.Errorf("findings search failed: %w", err)
	}

	var results []*findings.SearchResult
	for _, hit := range result.Hits {
		f, err := s.GetFinding(hit.ID)
		if err != nil {
			continue
		}
		results = append(results, &findings.SearchResult{
			Finding: f,
			Score:   hit.Score,
		})
	}

	return results, nil
}

// ListFindings returns findings filtered by options (no full-text search).
func (s *FindingsStoreImpl) ListFindings(opts findings.SearchOptions) ([]*findings.Finding, error) {
	limit := opts.Limit
	if limit == 0 {
		limit = 100
	}

	var result []*findings.Finding
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketFindings)
		c := b.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var f findings.Finding
			if err := json.Unmarshal(v, &f); err != nil {
				continue
			}
			if opts.Analyzer != "" && f.Analyzer != opts.Analyzer {
				continue
			}
			if opts.Severity != "" && f.Severity != opts.Severity {
				continue
			}
			if opts.FilePath != "" && !strings.Contains(f.FilePath, opts.FilePath) {
				continue
			}
			if opts.Category != "" && f.Category != opts.Category {
				continue
			}
			result = append(result, &f)
			if len(result) >= limit {
				break
			}
		}
		return nil
	})
	return result, err
}

// GetFileFindings returns all findings for a specific file.
func (s *FindingsStoreImpl) GetFileFindings(filePath string) ([]*findings.Finding, error) {
	return s.ListFindings(findings.SearchOptions{FilePath: filePath, Limit: 1000})
}

// ClearAnalyzer removes all findings for a specific analyzer.
func (s *FindingsStoreImpl) ClearAnalyzer(analyzer string) (int, error) {
	var toDelete []string
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketFindings)
		c := b.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var f findings.Finding
			if err := json.Unmarshal(v, &f); err != nil {
				continue
			}
			if f.Analyzer == analyzer {
				toDelete = append(toDelete, f.ID)
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}

	for _, id := range toDelete {
		if err := s.DeleteFinding(id); err != nil {
			return 0, err
		}
	}
	return len(toDelete), nil
}

// Stats returns aggregate finding counts.
func (s *FindingsStoreImpl) Stats() (*findings.Stats, error) {
	stats := &findings.Stats{
		ByAnalyzer: make(map[string]int),
		BySeverity: make(map[string]int),
	}

	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketFindings)
		c := b.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var f findings.Finding
			if err := json.Unmarshal(v, &f); err != nil {
				continue
			}
			stats.Total++
			stats.ByAnalyzer[f.Analyzer]++
			stats.BySeverity[f.Severity]++
		}
		return nil
	})
	return stats, err
}

// ReplaceFindingsForAnalyzer atomically replaces all findings for an analyzer.
// On success, old findings are gone and new ones are stored.
// On error, old findings remain untouched.
func (s *FindingsStoreImpl) ReplaceFindingsForAnalyzer(analyzer string, newFindings []*findings.Finding) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketFindings)
		c := b.Cursor()

		var toDelete [][]byte
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var f findings.Finding
			if err := json.Unmarshal(v, &f); err != nil {
				continue
			}
			if f.Analyzer == analyzer {
				toDelete = append(toDelete, k)
			}
		}

		for _, k := range toDelete {
			if err := b.Delete(k); err != nil {
				return err
			}
			s.search.Delete(string(k))
		}

		for _, f := range newFindings {
			if f.ID == "" {
				f.ID = ulid.Make().String()
			}
			if f.CreatedAt.IsZero() {
				f.CreatedAt = time.Now()
			}

			data, err := json.Marshal(f)
			if err != nil {
				return fmt.Errorf("marshal finding: %w", err)
			}
			if err := b.Put([]byte(f.ID), data); err != nil {
				return err
			}
			if err := s.search.Index(f.ID, findingToSearchDoc(f)); err != nil {
				return err
			}
		}

		return nil
	})
}

// ReplaceFindingsForAnalyzerAndFile atomically replaces findings for an analyzer within a specific file.
// Used for per-file incremental updates (complexity, secrets).
func (s *FindingsStoreImpl) ReplaceFindingsForAnalyzerAndFile(analyzer, filePath string, newFindings []*findings.Finding) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketFindings)
		c := b.Cursor()

		var toDelete [][]byte
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var f findings.Finding
			if err := json.Unmarshal(v, &f); err != nil {
				continue
			}
			if f.Analyzer == analyzer && f.FilePath == filePath {
				toDelete = append(toDelete, k)
			}
		}

		for _, k := range toDelete {
			if err := b.Delete(k); err != nil {
				return err
			}
			s.search.Delete(string(k))
		}

		for _, f := range newFindings {
			if f.ID == "" {
				f.ID = ulid.Make().String()
			}
			if f.CreatedAt.IsZero() {
				f.CreatedAt = time.Now()
			}

			data, err := json.Marshal(f)
			if err != nil {
				return fmt.Errorf("marshal finding: %w", err)
			}
			if err := b.Put([]byte(f.ID), data); err != nil {
				return err
			}
			if err := s.search.Index(f.ID, findingToSearchDoc(f)); err != nil {
				return err
			}
		}

		return nil
	})
}

// Clear removes all findings.
func (s *FindingsStoreImpl) Clear() error {
	err := s.db.Update(func(tx *bolt.Tx) error {
		if err := tx.DeleteBucket(BucketFindings); err != nil {
			return err
		}
		_, err := tx.CreateBucket(BucketFindings)
		return err
	})
	if err != nil {
		return err
	}

	searchPath := strings.TrimSuffix(s.dbPath, "index.db") + "findings.idx"
	if err := s.search.Close(); err != nil {
		return fmt.Errorf("failed to close findings search index: %w", err)
	}
	if err := os.RemoveAll(searchPath); err != nil {
		if idx, reopenErr := bleve.Open(searchPath); reopenErr == nil {
			s.search = idx
		}
		return fmt.Errorf("failed to remove findings search index: %w", err)
	}

	indexMapping, err := buildFindingsIndexMapping()
	if err != nil {
		return fmt.Errorf("failed to build findings index mapping after clear: %w", err)
	}
	index, err := bleve.New(searchPath, indexMapping)
	if err != nil {
		return fmt.Errorf("failed to recreate findings search index after clear: %w", err)
	}
	s.search = index

	return nil
}
