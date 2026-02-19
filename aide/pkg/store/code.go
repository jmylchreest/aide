// Package store provides storage backends for aide.
// This file implements code symbol storage and search.
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
	"github.com/blevesearch/bleve/v2/analysis/token/edgengram"
	"github.com/blevesearch/bleve/v2/analysis/token/lowercase"
	"github.com/blevesearch/bleve/v2/analysis/tokenizer/unicode"
	"github.com/blevesearch/bleve/v2/mapping"
	"github.com/jmylchreest/aide/aide/pkg/code"
	"github.com/oklog/ulid/v2"
	bolt "go.etcd.io/bbolt"
)

// Code-specific bucket names.
var (
	BucketSymbols    = []byte("symbols")
	BucketReferences = []byte("references")
	BucketFileIndex  = []byte("fileindex")
	BucketRefIndex   = []byte("refindex") // symbol name -> reference IDs
	BucketCodeMeta   = []byte("code_meta")
)

// CodeStore provides symbol storage and search.
type CodeStore struct {
	db     *bolt.DB
	search bleve.Index
	dbPath string
}

// CodeSearchResult represents a symbol search match with score.
type CodeSearchResult struct {
	Symbol *code.Symbol
	Score  float64
}

// NewCodeStore creates a new code store.
// dbPath: path to BBolt database (e.g., .aide/code/index.db)
// searchPath: path to Bleve index (e.g., .aide/code/search.bleve)
func NewCodeStore(dbPath, searchPath string) (*CodeStore, error) {
	// Ensure directories exist
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("failed to create db directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(searchPath), 0o755); err != nil {
		return nil, fmt.Errorf("failed to create search directory: %w", err)
	}

	// Open BBolt database
	db, err := bolt.Open(dbPath, 0o600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("failed to open code db: %w", err)
	}

	// Initialize buckets
	err = db.Update(func(tx *bolt.Tx) error {
		buckets := [][]byte{BucketSymbols, BucketReferences, BucketFileIndex, BucketRefIndex, BucketCodeMeta}
		for _, bucket := range buckets {
			if _, err := tx.CreateBucketIfNotExists(bucket); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize buckets: %w", err)
	}

	if err := RunCodeMigrations(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("code schema migration failed: %w", err)
	}

	// Open or create Bleve search index (auto-recovers from corruption)
	index, err := openOrCreateCodeSearchIndex(searchPath)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create/open code search index: %w", err)
	}

	cs := &CodeStore{
		db:     db,
		search: index,
		dbPath: dbPath,
	}

	if err := cs.ensureCodeSearchMapping(searchPath); err != nil {
		index.Close()
		db.Close()
		return nil, fmt.Errorf("code search mapping check failed: %w", err)
	}

	return cs, nil
}

// openOrCreateCodeSearchIndex opens an existing code search index or creates a new one.
// If the existing index is corrupted, it is removed and recreated from scratch.
func openOrCreateCodeSearchIndex(path string) (bleve.Index, error) {
	if _, statErr := os.Stat(path); os.IsNotExist(statErr) {
		return createCodeSearchIndex(path)
	}

	index, err := bleve.Open(path)
	if err == nil {
		return index, nil
	}

	// Index exists but is corrupted — recover by recreating
	log.Printf("code search index corrupted at %s (%v), rebuilding", path, err)
	if removeErr := os.RemoveAll(path); removeErr != nil {
		return nil, fmt.Errorf("failed to remove corrupted code search index: %w (original error: %v)", removeErr, err)
	}
	return createCodeSearchIndex(path)
}

// createCodeSearchIndex creates a fresh code bleve index at the given path.
func createCodeSearchIndex(path string) (bleve.Index, error) {
	indexMapping, err := buildCodeIndexMapping()
	if err != nil {
		return nil, err
	}
	return bleve.New(path, indexMapping)
}

// buildCodeIndexMapping creates a mapping for symbol search.
func buildCodeIndexMapping() (mapping.IndexMapping, error) {
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

	// Edge n-gram for prefix matching (get -> getUser)
	err = indexMapping.AddCustomTokenFilter("edge_ngram_filter", map[string]interface{}{
		"type": edgengram.Name,
		"min":  2.0,
		"max":  15.0,
	})
	if err != nil {
		return nil, err
	}

	err = indexMapping.AddCustomAnalyzer("edge_ngram", map[string]interface{}{
		"type":      custom.Name,
		"tokenizer": unicode.Name,
		"token_filters": []string{
			lowercase.Name,
			"edge_ngram_filter",
		},
	})
	if err != nil {
		return nil, err
	}

	// Document mapping for symbols
	symbolMapping := bleve.NewDocumentMapping()

	// Name field (most important - standard + edge ngram)
	nameField := bleve.NewTextFieldMapping()
	nameField.Analyzer = "standard_lower"
	nameField.Store = true
	symbolMapping.AddFieldMappingsAt("name", nameField)

	nameEdgeField := bleve.NewTextFieldMapping()
	nameEdgeField.Analyzer = "edge_ngram"
	nameEdgeField.Store = false
	nameEdgeField.IncludeInAll = false
	symbolMapping.AddFieldMappingsAt("name_edge", nameEdgeField)

	// Signature field (searchable)
	sigField := bleve.NewTextFieldMapping()
	sigField.Analyzer = "standard_lower"
	sigField.Store = true
	symbolMapping.AddFieldMappingsAt("signature", sigField)

	// Doc comment field
	docField := bleve.NewTextFieldMapping()
	docField.Analyzer = "standard_lower"
	docField.Store = false
	symbolMapping.AddFieldMappingsAt("doc", docField)

	// Keyword fields (exact match filtering)
	kindField := bleve.NewTextFieldMapping()
	kindField.Analyzer = keyword.Name
	symbolMapping.AddFieldMappingsAt("kind", kindField)

	langField := bleve.NewTextFieldMapping()
	langField.Analyzer = keyword.Name
	symbolMapping.AddFieldMappingsAt("lang", langField)

	fileField := bleve.NewTextFieldMapping()
	fileField.Analyzer = keyword.Name
	symbolMapping.AddFieldMappingsAt("file", fileField)

	indexMapping.AddDocumentMapping("symbol", symbolMapping)
	indexMapping.DefaultMapping = symbolMapping

	return indexMapping, nil
}

// ensureCodeSearchMapping checks if the code search index mapping has changed and rebuilds if needed.
func (s *CodeStore) ensureCodeSearchMapping(searchPath string) error {
	m, err := buildCodeIndexMapping()
	if err != nil {
		return err
	}
	hash := MappingHash(m)

	// Read stored hash from code meta bucket.
	var stored string
	s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketCodeMeta)
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
		log.Printf("store: code search mapping changed, rebuilding index")
	}

	// Close current index, remove, recreate.
	s.search.Close()
	os.RemoveAll(searchPath)

	indexMapping, err := buildCodeIndexMapping()
	if err != nil {
		return err
	}
	index, err := bleve.New(searchPath, indexMapping)
	if err != nil {
		return err
	}
	s.search = index

	// Re-index all symbols from BBolt.
	err = s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketSymbols)
		c := b.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var sym code.Symbol
			if err := json.Unmarshal(v, &sym); err != nil {
				continue
			}
			doc := map[string]interface{}{
				"name":      sym.Name,
				"name_edge": sym.Name,
				"signature": sym.Signature,
				"doc":       sym.DocComment,
				"kind":      sym.Kind,
				"lang":      sym.Language,
				"file":      sym.FilePath,
			}
			if err := s.search.Index(sym.ID, doc); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Write new hash.
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketCodeMeta)
		if b == nil {
			return fmt.Errorf("code meta bucket not found")
		}
		return b.Put([]byte("search_mapping_hash"), []byte(hash))
	})
}

// Close closes the code store.
func (s *CodeStore) Close() error {
	if s.search != nil {
		s.search.Close()
	}
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// AddSymbol stores a symbol and indexes it for search.
func (s *CodeStore) AddSymbol(sym *code.Symbol) error {
	// Generate ID if not set
	if sym.ID == "" {
		sym.ID = ulid.Make().String()
	}
	if sym.CreatedAt.IsZero() {
		sym.CreatedAt = time.Now()
	}

	// Store in BBolt
	err := s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketSymbols)
		data, err := json.Marshal(sym)
		if err != nil {
			return err
		}
		return b.Put([]byte(sym.ID), data)
	})
	if err != nil {
		return fmt.Errorf("failed to store symbol: %w", err)
	}

	// Index for search
	doc := map[string]interface{}{
		"name":      sym.Name,
		"name_edge": sym.Name,
		"signature": sym.Signature,
		"doc":       sym.DocComment,
		"kind":      sym.Kind,
		"lang":      sym.Language,
		"file":      sym.FilePath,
	}
	if err := s.search.Index(sym.ID, doc); err != nil {
		return fmt.Errorf("failed to index symbol: %w", err)
	}

	return nil
}

// GetSymbol retrieves a symbol by ID.
func (s *CodeStore) GetSymbol(id string) (*code.Symbol, error) {
	var sym code.Symbol
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketSymbols)
		data := b.Get([]byte(id))
		if data == nil {
			return ErrNotFound
		}
		return json.Unmarshal(data, &sym)
	})
	if err != nil {
		return nil, err
	}
	return &sym, nil
}

// DeleteSymbol removes a symbol by ID.
func (s *CodeStore) DeleteSymbol(id string) error {
	err := s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketSymbols)
		return b.Delete([]byte(id))
	})
	if err != nil {
		return err
	}
	return s.search.Delete(id)
}

// SearchSymbols performs full-text search on symbols.
func (s *CodeStore) SearchSymbols(query string, opts code.SearchOptions) ([]*CodeSearchResult, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = 20
	}

	// Build query - try multiple strategies to find matches
	lowerQuery := strings.ToLower(query)

	// 1. Exact prefix match on name (for camelCase like getUser)
	prefixQuery := bleve.NewPrefixQuery(lowerQuery)
	prefixQuery.SetField("name")

	// 2. Wildcard containing match (for partial matches)
	wildcardQuery := bleve.NewWildcardQuery("*" + lowerQuery + "*")
	wildcardQuery.SetField("name")

	// 3. Match in signature
	sigQuery := bleve.NewMatchQuery(query)
	sigQuery.SetField("signature")

	// 4. Match in doc comments
	docQuery := bleve.NewMatchQuery(query)
	docQuery.SetField("doc")

	// Combine with OR (any match)
	q := bleve.NewDisjunctionQuery(prefixQuery, wildcardQuery, sigQuery, docQuery)

	// Create search request
	searchReq := bleve.NewSearchRequest(q)
	searchReq.Size = limit * 2 // Request more since we filter after

	// Execute search
	searchResult, err := s.search.Search(searchReq)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	// Fetch symbols and apply filters
	results := make([]*CodeSearchResult, 0, len(searchResult.Hits))
	for _, hit := range searchResult.Hits {
		sym, err := s.GetSymbol(hit.ID)
		if err != nil {
			continue
		}

		// Apply filters
		if opts.Kind != "" && sym.Kind != opts.Kind {
			continue
		}
		if opts.Language != "" && sym.Language != opts.Language {
			continue
		}
		if opts.FilePath != "" && !strings.Contains(sym.FilePath, opts.FilePath) {
			continue
		}

		results = append(results, &CodeSearchResult{
			Symbol: sym,
			Score:  hit.Score,
		})
	}

	return results, nil
}

// AddReference stores a reference.
func (s *CodeStore) AddReference(ref *code.Reference) error {
	// Generate ID if not set
	if ref.ID == "" {
		ref.ID = ulid.Make().String()
	}
	if ref.CreatedAt.IsZero() {
		ref.CreatedAt = time.Now()
	}

	// Store in BBolt
	err := s.db.Update(func(tx *bolt.Tx) error {
		// Store reference
		b := tx.Bucket(BucketReferences)
		data, err := json.Marshal(ref)
		if err != nil {
			return err
		}
		if err := b.Put([]byte(ref.ID), data); err != nil {
			return err
		}

		// Update reference index (symbol name -> reference IDs)
		refIdx := tx.Bucket(BucketRefIndex)
		key := []byte(ref.SymbolName)
		ids := make([]string, 0, 1)
		if existing := refIdx.Get(key); existing != nil {
			json.Unmarshal(existing, &ids)
		}
		ids = append(ids, ref.ID)
		idsData, err := json.Marshal(ids)
		if err != nil {
			return err
		}
		return refIdx.Put(key, idsData)
	})
	if err != nil {
		return fmt.Errorf("failed to store reference: %w", err)
	}

	return nil
}

// GetReference retrieves a reference by ID.
func (s *CodeStore) GetReference(id string) (*code.Reference, error) {
	var ref code.Reference
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketReferences)
		data := b.Get([]byte(id))
		if data == nil {
			return ErrNotFound
		}
		return json.Unmarshal(data, &ref)
	})
	if err != nil {
		return nil, err
	}
	return &ref, nil
}

// SearchReferences finds all references to a symbol by name.
func (s *CodeStore) SearchReferences(opts code.ReferenceSearchOptions) ([]*code.Reference, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = 100
	}

	var refs []*code.Reference

	err := s.db.View(func(tx *bolt.Tx) error {
		refIdx := tx.Bucket(BucketRefIndex)
		refBucket := tx.Bucket(BucketReferences)

		// Get reference IDs for this symbol
		key := []byte(opts.SymbolName)
		data := refIdx.Get(key)
		if data == nil {
			return nil // No references found
		}

		var ids []string
		if err := json.Unmarshal(data, &ids); err != nil {
			return err
		}

		// Fetch each reference and apply filters
		for _, id := range ids {
			if len(refs) >= limit {
				break
			}

			refData := refBucket.Get([]byte(id))
			if refData == nil {
				continue
			}

			var ref code.Reference
			if err := json.Unmarshal(refData, &ref); err != nil {
				continue
			}

			// Apply filters
			if opts.Kind != "" && ref.Kind != opts.Kind {
				continue
			}
			if opts.FilePath != "" && !strings.Contains(ref.FilePath, opts.FilePath) {
				continue
			}

			refs = append(refs, &ref)
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to search references: %w", err)
	}

	return refs, nil
}

// ClearFileReferences removes all references from a file.
func (s *CodeStore) ClearFileReferences(filePath string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		refBucket := tx.Bucket(BucketReferences)
		refIdx := tx.Bucket(BucketRefIndex)

		// Find all references in this file and remove them
		c := refBucket.Cursor()
		var toDelete [][]byte

		for k, v := c.First(); k != nil; k, v = c.Next() {
			var ref code.Reference
			if err := json.Unmarshal(v, &ref); err != nil {
				continue
			}
			if ref.FilePath == filePath {
				toDelete = append(toDelete, k)

				// Also remove from index
				indexKey := []byte(ref.SymbolName)
				if data := refIdx.Get(indexKey); data != nil {
					var ids []string
					json.Unmarshal(data, &ids)
					// Remove this ID from the list
					newIds := make([]string, 0, len(ids)-1)
					for _, id := range ids {
						if id != string(k) {
							newIds = append(newIds, id)
						}
					}
					if len(newIds) > 0 {
						newData, _ := json.Marshal(newIds)
						refIdx.Put(indexKey, newData)
					} else {
						refIdx.Delete(indexKey)
					}
				}
			}
		}

		for _, k := range toDelete {
			refBucket.Delete(k)
		}

		return nil
	})
}

// GetFileInfo retrieves file tracking info.
func (s *CodeStore) GetFileInfo(path string) (*code.FileInfo, error) {
	var info code.FileInfo
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketFileIndex)
		data := b.Get([]byte(path))
		if data == nil {
			return ErrNotFound
		}
		return json.Unmarshal(data, &info)
	})
	if err != nil {
		return nil, err
	}
	return &info, nil
}

// SetFileInfo stores file tracking info.
func (s *CodeStore) SetFileInfo(info *code.FileInfo) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketFileIndex)
		data, err := json.Marshal(info)
		if err != nil {
			return err
		}
		return b.Put([]byte(info.Path), data)
	})
}

// ClearFile removes all symbols for a file and its tracking info.
func (s *CodeStore) ClearFile(filePath string) error {
	// Get existing file info to find symbol IDs
	info, err := s.GetFileInfo(filePath)
	if err != nil && err != ErrNotFound {
		return err
	}

	// Delete symbols
	if info != nil {
		for _, id := range info.SymbolIDs {
			s.DeleteSymbol(id)
		}
	}

	// Delete file info
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketFileIndex)
		return b.Delete([]byte(filePath))
	})
}

// Clear removes all symbols, references, and file tracking data.
func (s *CodeStore) Clear() error {
	// Clear BBolt buckets
	err := s.db.Update(func(tx *bolt.Tx) error {
		for _, bucket := range [][]byte{BucketSymbols, BucketReferences, BucketFileIndex, BucketRefIndex} {
			b := tx.Bucket(bucket)
			c := b.Cursor()
			for k, _ := c.First(); k != nil; k, _ = c.Next() {
				if err := b.Delete(k); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Recreate search index
	searchPath := strings.TrimSuffix(s.dbPath, "index.db") + "search.bleve"
	s.search.Close()
	os.RemoveAll(searchPath)

	indexMapping, err := buildCodeIndexMapping()
	if err != nil {
		return err
	}
	index, err := bleve.New(searchPath, indexMapping)
	if err != nil {
		return err
	}
	s.search = index

	return nil
}

// Stats returns indexing statistics.
func (s *CodeStore) Stats() (*code.IndexStats, error) {
	stats := &code.IndexStats{}

	err := s.db.View(func(tx *bolt.Tx) error {
		// Count symbols
		b := tx.Bucket(BucketSymbols)
		stats.Symbols = b.Stats().KeyN

		// Count references
		b = tx.Bucket(BucketReferences)
		stats.References = b.Stats().KeyN

		// Count files
		b = tx.Bucket(BucketFileIndex)
		stats.Files = b.Stats().KeyN

		return nil
	})
	if err != nil {
		return nil, err
	}

	return stats, nil
}

// GetFileSymbols returns all symbols for a given file.
func (s *CodeStore) GetFileSymbols(filePath string) ([]*code.Symbol, error) {
	info, err := s.GetFileInfo(filePath)
	if err != nil {
		return nil, err
	}

	symbols := make([]*code.Symbol, 0, len(info.SymbolIDs))
	for _, id := range info.SymbolIDs {
		sym, err := s.GetSymbol(id)
		if err != nil {
			continue
		}
		symbols = append(symbols, sym)
	}

	return symbols, nil
}

// ListAllSymbols returns all symbols (up to limit).
func (s *CodeStore) ListAllSymbols(limit int) ([]*code.Symbol, error) {
	if limit <= 0 {
		limit = 1000
	}

	var symbols []*code.Symbol
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketSymbols)
		c := b.Cursor()
		count := 0
		for k, v := c.First(); k != nil && count < limit; k, v = c.Next() {
			var sym code.Symbol
			if err := json.Unmarshal(v, &sym); err != nil {
				continue
			}
			symbols = append(symbols, &sym)
			count++
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return symbols, nil
}
