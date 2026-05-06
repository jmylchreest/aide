// Package store provides storage backends for aide.
// This file implements code symbol storage and search.
package store

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
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
	// BucketSymbolsByFile is keyed by composeFileKey(filePath, symbolID) with
	// nil value. Lets ClearFile range-scan the prefix in O(per-file) instead
	// of ForEach-scanning the full BucketSymbols.
	BucketSymbolsByFile = []byte("symbols_by_file")
	// BucketReferencesByFile is keyed by composeFileKey(filePath, refID) with
	// the reference's SymbolName as the value, so ClearFileReferences can find
	// every per-file ref in O(per-file) and resolve its reverse-index entry
	// without re-fetching the reference body.
	BucketReferencesByFile = []byte("references_by_file")
	// BucketRefIndex stores composeNameRefKey(symbolName, refID) keys with nil
	// values — a reverse index keyed by the called symbol's name. Range-scan
	// the symbolName prefix to enumerate every reference to that name.
	// Replaces the previous JSON-slice format from schema v1.
	BucketRefIndex = []byte("refindex")
	BucketCodeMeta = []byte("code_meta")
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

	db, err := bolt.Open(dbPath, 0o600, indexerBoltOptions(1*time.Second))
	if err != nil {
		return nil, fmt.Errorf("failed to open code db: %w", err)
	}

	// Initialize buckets
	err = db.Update(func(tx *bolt.Tx) error {
		buckets := [][]byte{BucketSymbols, BucketReferences, BucketFileIndex, BucketRefIndex, BucketSymbolsByFile, BucketReferencesByFile, BucketCodeMeta}
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
// composeFileKey builds a composite bbolt key for the by-file secondary
// indexes — `<filePath>\x00<id>`. The NUL separator gives a fixed-width
// boundary for prefix range scans, and is illegal in both file paths and
// ULIDs so it can never appear inside the components.
func composeFileKey(filePath, id string) []byte {
	key := make([]byte, 0, len(filePath)+1+len(id))
	key = append(key, filePath...)
	key = append(key, 0x00)
	key = append(key, id...)
	return key
}

// composeNameRefKey builds a composite bbolt key for the reference reverse
// index — `<symbolName>\x00<refID>`. Range-scan the symbolName prefix to
// enumerate every reference to that symbol.
func composeNameRefKey(symbolName, refID string) []byte {
	key := make([]byte, 0, len(symbolName)+1+len(refID))
	key = append(key, symbolName...)
	key = append(key, 0x00)
	key = append(key, refID...)
	return key
}

// fileKeyPrefix returns the prefix used to range-scan all entries for
// filePath in a by-file bucket: `<filePath>\x00`.
func fileKeyPrefix(filePath string) []byte {
	prefix := make([]byte, 0, len(filePath)+1)
	prefix = append(prefix, filePath...)
	prefix = append(prefix, 0x00)
	return prefix
}

// nameRefKeyPrefix returns the prefix used to range-scan all references for
// symbolName in BucketRefIndex: `<symbolName>\x00`.
func nameRefKeyPrefix(symbolName string) []byte {
	prefix := make([]byte, 0, len(symbolName)+1)
	prefix = append(prefix, symbolName...)
	prefix = append(prefix, 0x00)
	return prefix
}

// buildSymbolBleveDoc returns the Bleve document for a symbol. Shared between
// per-record AddSymbol and the bulk IndexFileBatch path so search indexing
// stays consistent across both.
func buildSymbolBleveDoc(sym *code.Symbol) map[string]interface{} {
	return map[string]interface{}{
		"name":      sym.Name,
		"name_edge": sym.Name,
		"signature": sym.Signature,
		"doc":       sym.DocComment,
		"kind":      sym.Kind,
		"lang":      sym.Language,
		"file":      sym.FilePath,
	}
}

// addSymbolTx writes a symbol to BucketSymbols and records its file-keyed
// secondary-index entry inside an existing tx. The caller is responsible for
// indexing the symbol into the Bleve search index (or for accumulating it
// into a Bleve batch); this helper only touches bbolt.
func (s *CodeStore) addSymbolTx(tx *bolt.Tx, sym *code.Symbol) error {
	if sym.ID == "" {
		sym.ID = ulid.Make().String()
	}
	if sym.CreatedAt.IsZero() {
		sym.CreatedAt = time.Now()
	}

	b := tx.Bucket(BucketSymbols)
	data, err := json.Marshal(sym)
	if err != nil {
		return err
	}
	if err := b.Put([]byte(sym.ID), data); err != nil {
		return err
	}
	// File-keyed secondary index: empty value, the key carries everything.
	return tx.Bucket(BucketSymbolsByFile).Put(composeFileKey(sym.FilePath, sym.ID), nil)
}

// AddSymbol stores a symbol and indexes it for search. Each call opens its
// own bbolt transaction and a single Bleve Index call — fine for ad-hoc
// callers (CLI helpers, tests) but inefficient for indexer hot paths, which
// should use IndexFileBatch instead.
func (s *CodeStore) AddSymbol(sym *code.Symbol) error {
	err := s.db.Update(func(tx *bolt.Tx) error {
		return s.addSymbolTx(tx, sym)
	})
	if err != nil {
		return fmt.Errorf("failed to store symbol: %w", err)
	}
	if err := s.search.Index(sym.ID, buildSymbolBleveDoc(sym)); err != nil {
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

// DeleteSymbol removes a symbol by ID from both the primary bucket and the
// file-keyed secondary index, then drops it from the Bleve search index.
func (s *CodeStore) DeleteSymbol(id string) error {
	err := s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketSymbols)
		data := b.Get([]byte(id))
		if data == nil {
			return ErrNotFound
		}
		var sym code.Symbol
		if err := json.Unmarshal(data, &sym); err != nil {
			return err
		}
		if err := b.Delete([]byte(id)); err != nil {
			return err
		}
		return tx.Bucket(BucketSymbolsByFile).Delete(composeFileKey(sym.FilePath, id))
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

// addReferenceTx writes a reference and updates two secondary indexes inside
// an existing tx:
//
//   - BucketReferencesByFile (keyed by `<filePath>\x00<refID>`, value =
//     symbolName) lets ClearFileReferences enumerate per-file refs and
//     resolve their reverse-index entries in O(per-file) without touching
//     the main reference bucket.
//   - BucketRefIndex (keyed by `<symbolName>\x00<refID>`) is the reverse
//     index used by SearchReferences; one row per (name, ref) pair, range-
//     scanned by the symbolName prefix. The previous JSON-slice format was
//     O(n) per add and read.
func (s *CodeStore) addReferenceTx(tx *bolt.Tx, ref *code.Reference) error {
	if ref.ID == "" {
		ref.ID = ulid.Make().String()
	}
	if ref.CreatedAt.IsZero() {
		ref.CreatedAt = time.Now()
	}

	b := tx.Bucket(BucketReferences)
	data, err := json.Marshal(ref)
	if err != nil {
		return err
	}
	if err := b.Put([]byte(ref.ID), data); err != nil {
		return err
	}
	if err := tx.Bucket(BucketReferencesByFile).Put(composeFileKey(ref.FilePath, ref.ID), []byte(ref.SymbolName)); err != nil {
		return err
	}
	return tx.Bucket(BucketRefIndex).Put(composeNameRefKey(ref.SymbolName, ref.ID), nil)
}

// AddReference stores a reference. Indexer hot paths should use
// IndexFileBatch instead so all of a file's references commit in one tx.
func (s *CodeStore) AddReference(ref *code.Reference) error {
	err := s.db.Update(func(tx *bolt.Tx) error {
		return s.addReferenceTx(tx, ref)
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

// SearchReferences finds all references to a symbol by name. Range-scans
// BucketRefIndex by the symbolName prefix; each composite key carries the
// reference ID directly so we never reload a slice or count entries upfront.
func (s *CodeStore) SearchReferences(opts code.ReferenceSearchOptions) ([]*code.Reference, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = 100
	}

	var refs []*code.Reference

	err := s.db.View(func(tx *bolt.Tx) error {
		refIdx := tx.Bucket(BucketRefIndex)
		refBucket := tx.Bucket(BucketReferences)

		prefix := nameRefKeyPrefix(opts.SymbolName)
		c := refIdx.Cursor()
		for k, _ := c.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, _ = c.Next() {
			if len(refs) >= limit {
				break
			}
			refID := k[len(prefix):]
			refData := refBucket.Get(refID)
			if refData == nil {
				continue
			}
			var ref code.Reference
			if err := json.Unmarshal(refData, &ref); err != nil {
				continue
			}
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

// GetFileReferences returns all references in a given file. Range-scans
// BucketReferencesByFile by the filePath prefix in O(per-file).
func (s *CodeStore) GetFileReferences(filePath string) ([]*code.Reference, error) {
	var refs []*code.Reference

	err := s.db.View(func(tx *bolt.Tx) error {
		byFile := tx.Bucket(BucketReferencesByFile)
		refBucket := tx.Bucket(BucketReferences)
		prefix := fileKeyPrefix(filePath)
		c := byFile.Cursor()
		for k, _ := c.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, _ = c.Next() {
			refID := k[len(prefix):]
			data := refBucket.Get(refID)
			if data == nil {
				continue
			}
			var ref code.Reference
			if err := json.Unmarshal(data, &ref); err != nil {
				continue
			}
			refs = append(refs, &ref)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get file references: %w", err)
	}

	return refs, nil
}

// clearFileReferencesTx removes every reference whose FilePath equals
// filePath plus its corresponding reverse-index entry, inside an existing tx.
//
// Range-scans BucketReferencesByFile by the filePath prefix to enumerate
// per-file refs in O(per-file). Each by-file row carries the reference's
// SymbolName as its value, which lets us compute the BucketRefIndex
// composite key directly without re-fetching the reference from
// BucketReferences.
func (s *CodeStore) clearFileReferencesTx(tx *bolt.Tx, filePath string) error {
	prefix := fileKeyPrefix(filePath)
	byFile := tx.Bucket(BucketReferencesByFile)
	refBucket := tx.Bucket(BucketReferences)
	refIdx := tx.Bucket(BucketRefIndex)
	c := byFile.Cursor()

	var refIDs []string
	var byFileKeys [][]byte
	var refIdxKeys [][]byte
	for k, v := c.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, v = c.Next() {
		refID := string(k[len(prefix):])
		refIDs = append(refIDs, refID)
		byFileKeys = append(byFileKeys, append([]byte(nil), k...))
		refIdxKeys = append(refIdxKeys, composeNameRefKey(string(v), refID))
	}

	for i, refID := range refIDs {
		if err := refBucket.Delete([]byte(refID)); err != nil {
			return err
		}
		if err := byFile.Delete(byFileKeys[i]); err != nil {
			return err
		}
		if err := refIdx.Delete(refIdxKeys[i]); err != nil {
			return err
		}
	}
	return nil
}

// ClearFileReferences removes all references from a file.
func (s *CodeStore) ClearFileReferences(filePath string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		return s.clearFileReferencesTx(tx, filePath)
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

// ListAllFileInfo returns the FileInfo entries for every file currently
// tracked in the index. Used by reconcile passes that need to compare on-disk
// state to the index (missing files, stale mtimes).
func (s *CodeStore) ListAllFileInfo() ([]*code.FileInfo, error) {
	var infos []*code.FileInfo
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketFileIndex)
		return b.ForEach(func(_, v []byte) error {
			var info code.FileInfo
			if err := json.Unmarshal(v, &info); err != nil {
				return nil
			}
			infos = append(infos, &info)
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return infos, nil
}

// setFileInfoTx writes FileInfo for a path inside an existing tx.
func (s *CodeStore) setFileInfoTx(tx *bolt.Tx, info *code.FileInfo) error {
	b := tx.Bucket(BucketFileIndex)
	data, err := json.Marshal(info)
	if err != nil {
		return err
	}
	return b.Put([]byte(info.Path), data)
}

// SetFileInfo stores file tracking info.
func (s *CodeStore) SetFileInfo(info *code.FileInfo) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		return s.setFileInfoTx(tx, info)
	})
}

// clearFileTx removes all symbols owned by filePath plus the FileInfo entry
// for that path, and returns the symbol IDs removed so the caller can drop
// them from the Bleve search index.
//
// Uses the file-keyed secondary index BucketSymbolsByFile to enumerate
// per-file symbols in O(per-file). Without this index the previous
// implementation had to ForEach the entire BucketSymbols and JSON-decode
// every row, which was O(total_symbols) per call and dominated wall-clock
// on large repositories.
//
// In-file orphan rows from a crashed prior write that exist in BucketSymbols
// but not in BucketSymbolsByFile are caught by the daemon's startup
// reconcile pass — the existing safety net for that class of failure.
func (s *CodeStore) clearFileTx(tx *bolt.Tx, filePath string) ([]string, error) {
	prefix := fileKeyPrefix(filePath)
	byFile := tx.Bucket(BucketSymbolsByFile)
	c := byFile.Cursor()

	var matchingIDs []string
	var byFileKeys [][]byte
	for k, _ := c.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, _ = c.Next() {
		matchingIDs = append(matchingIDs, string(k[len(prefix):]))
		byFileKeys = append(byFileKeys, append([]byte(nil), k...))
	}

	symBucket := tx.Bucket(BucketSymbols)
	for i, id := range matchingIDs {
		if err := symBucket.Delete([]byte(id)); err != nil {
			return nil, err
		}
		if err := byFile.Delete(byFileKeys[i]); err != nil {
			return nil, err
		}
	}
	if err := tx.Bucket(BucketFileIndex).Delete([]byte(filePath)); err != nil {
		return nil, err
	}
	return matchingIDs, nil
}

// ClearFile removes all symbols for a file (in bbolt and Bleve) and its
// FileInfo entry.
func (s *CodeStore) ClearFile(filePath string) error {
	var clearedIDs []string
	err := s.db.Update(func(tx *bolt.Tx) error {
		ids, err := s.clearFileTx(tx, filePath)
		clearedIDs = ids
		return err
	})
	if err != nil {
		return err
	}
	if s.search != nil && len(clearedIDs) > 0 {
		batch := s.search.NewBatch()
		for _, id := range clearedIDs {
			batch.Delete(id)
		}
		if err := s.search.Batch(batch); err != nil {
			return fmt.Errorf("failed to delete cleared symbols from search: %w", err)
		}
	}
	return nil
}

// IndexFileBatch performs all the per-file write work — optional clear,
// symbol writes, reference writes, FileInfo update — inside a single bbolt
// transaction, and applies the corresponding Bleve search updates as a
// single batch after the bbolt commit.
//
// Why this exists: the per-record AddSymbol/AddReference/SetFileInfo path
// opens its own transaction (and pays bbolt's two fdatasyncs per commit) for
// every record. On the indexer hot path with hundreds of symbols and
// thousands of references per file, that fsync amplification dominated
// wall-clock time. IndexFileBatch collapses everything into one commit per
// file regardless of record count, and lets Bleve apply many doc updates
// in one batch (which Bleve documents as the bulk-load fast path).
//
// bbolt and Bleve are independent stores; the bbolt commit happens first,
// the Bleve batch second. If the Bleve apply fails after the bbolt commit
// the search index will be temporarily out of sync — the daemon's startup
// reconcile is the safety net for that case, the same one that handles
// in-file orphans from prior crashed writes.
func (s *CodeStore) IndexFileBatch(
	filePath string,
	symbols []*code.Symbol,
	refs []*code.Reference,
	mtime time.Time,
	sizeBytes int64,
) error {
	var clearedIDs []string
	var symbolIDs []string

	err := s.db.Update(func(tx *bolt.Tx) error {
		// Skip the clear pass when the file has never been indexed before;
		// there is nothing to clear and the bucket scan is pure overhead.
		// Orphans from a prior crashed write are caught by startup reconcile.
		if tx.Bucket(BucketFileIndex).Get([]byte(filePath)) != nil {
			ids, err := s.clearFileTx(tx, filePath)
			if err != nil {
				return err
			}
			clearedIDs = ids
			if err := s.clearFileReferencesTx(tx, filePath); err != nil {
				return err
			}
		}

		for _, sym := range symbols {
			sym.FilePath = filePath
			if err := s.addSymbolTx(tx, sym); err != nil {
				return err
			}
			symbolIDs = append(symbolIDs, sym.ID)
		}
		for _, ref := range refs {
			ref.FilePath = filePath
			if err := s.addReferenceTx(tx, ref); err != nil {
				return err
			}
		}
		return s.setFileInfoTx(tx, &code.FileInfo{
			Path:      filePath,
			ModTime:   mtime,
			SymbolIDs: symbolIDs,
			Tokens:    code.EstimateTokensFromSize(filePath, sizeBytes),
			SizeBytes: sizeBytes,
		})
	})
	if err != nil {
		return fmt.Errorf("failed to index file %q: %w", filePath, err)
	}

	if s.search == nil || (len(clearedIDs) == 0 && len(symbols) == 0) {
		return nil
	}
	batch := s.search.NewBatch()
	for _, id := range clearedIDs {
		batch.Delete(id)
	}
	for _, sym := range symbols {
		batch.Index(sym.ID, buildSymbolBleveDoc(sym))
	}
	if err := s.search.Batch(batch); err != nil {
		return fmt.Errorf("failed to apply bleve batch for %q: %w", filePath, err)
	}
	return nil
}

// Clear removes all symbols, references, and file tracking data.
func (s *CodeStore) Clear() error {
	// Clear BBolt buckets
	err := s.db.Update(func(tx *bolt.Tx) error {
		for _, bucket := range [][]byte{BucketSymbols, BucketReferences, BucketFileIndex, BucketRefIndex, BucketSymbolsByFile, BucketReferencesByFile} {
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

	// Recreate search index.
	// If recreation fails, attempt to reopen the existing index to avoid
	// leaving s.search in a closed/nil state that would panic on next use.
	searchPath := strings.TrimSuffix(s.dbPath, "index.db") + "search.bleve"
	if err := s.search.Close(); err != nil {
		return fmt.Errorf("failed to close search index: %w", err)
	}
	if err := os.RemoveAll(searchPath); err != nil {
		// Try to reopen old index before returning error.
		// If reopen also fails, set nil to avoid operating on a closed index.
		if idx, reopenErr := bleve.Open(searchPath); reopenErr == nil {
			s.search = idx
		} else {
			s.search = nil
		}
		return fmt.Errorf("failed to remove search index: %w", err)
	}

	indexMapping, err := buildCodeIndexMapping()
	if err != nil {
		s.search = nil
		return fmt.Errorf("failed to build code index mapping after clear: %w", err)
	}
	index, err := bleve.New(searchPath, indexMapping)
	if err != nil {
		s.search = nil
		return fmt.Errorf("failed to recreate code search index after clear: %w", err)
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

// GetContainingSymbol returns the narrowest symbol whose line range contains the given line.
// Returns ErrNotFound if no symbol spans the given line.
func (s *CodeStore) GetContainingSymbol(filePath string, line int) (*code.Symbol, error) {
	symbols, err := s.GetFileSymbols(filePath)
	if err != nil {
		return nil, err
	}

	var best *code.Symbol
	bestSpan := int(^uint(0) >> 1) // max int

	for _, sym := range symbols {
		if sym.StartLine <= line && line <= sym.EndLine {
			span := sym.EndLine - sym.StartLine
			if span < bestSpan {
				best = sym
				bestSpan = span
			}
		}
	}

	if best == nil {
		return nil, ErrNotFound
	}
	return best, nil
}

// TopReferencedSymbols returns symbols ranked by reference count (descending).
func (s *CodeStore) TopReferencedSymbols(limit int, kind string) ([]*code.SymbolRefCount, error) {
	if limit <= 0 {
		limit = 25
	}

	type entry struct {
		name  string
		count int
	}

	var entries []entry

	err := s.db.View(func(tx *bolt.Tx) error {
		// Composite keys are `<name>\x00<refID>`; bbolt iterates in key order
		// so all entries for a given name appear consecutively. Count by
		// walking the cursor and emitting an entry on each name boundary.
		refIdx := tx.Bucket(BucketRefIndex)
		c := refIdx.Cursor()
		var currentName string
		var currentCount int
		for k, _ := c.First(); k != nil; k, _ = c.Next() {
			sep := bytes.IndexByte(k, 0x00)
			if sep < 0 {
				continue
			}
			name := string(k[:sep])
			if name != currentName {
				if currentName != "" && currentCount > 0 {
					entries = append(entries, entry{name: currentName, count: currentCount})
				}
				currentName = name
				currentCount = 1
			} else {
				currentCount++
			}
		}
		if currentName != "" && currentCount > 0 {
			entries = append(entries, entry{name: currentName, count: currentCount})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Sort descending by count
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].count > entries[j].count
	})

	// Build results, resolving symbol metadata where possible
	var results []*code.SymbolRefCount
	for _, e := range entries {
		// Try to resolve symbol metadata via Bleve search
		rc := &code.SymbolRefCount{
			Symbol: e.name,
			Count:  e.count,
		}

		hits, err := s.SearchSymbols(e.name, code.SearchOptions{Kind: kind, Limit: 1})
		if err == nil && len(hits) > 0 && hits[0].Symbol.Name == e.name {
			rc.Kind = hits[0].Symbol.Kind
			rc.File = hits[0].Symbol.FilePath
		}

		// Apply kind filter: skip if kind requested but not matched
		if kind != "" && rc.Kind != kind {
			continue
		}

		results = append(results, rc)
		if len(results) >= limit {
			break
		}
	}

	return results, nil
}

// ListAllSymbols returns all symbols. limit semantics follow the project
// max-depth convention: 0 → default (1000), positive → cap, negative → unlimited.
func (s *CodeStore) ListAllSymbols(limit int) ([]*code.Symbol, error) {
	if limit == 0 {
		limit = 1000
	}
	unlimited := limit < 0

	var symbols []*code.Symbol
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketSymbols)
		c := b.Cursor()
		count := 0
		for k, v := c.First(); k != nil && (unlimited || count < limit); k, v = c.Next() {
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

// ListAllReferences returns every reference in the index. Used by Reconcile
// to find orphan references whose file has been ignored or deleted but whose
// fileinfo entry was already cleared (so the fileinfo-driven pass misses
// them). Pass limit < 0 for unbounded iteration.
func (s *CodeStore) ListAllReferences(limit int) ([]*code.Reference, error) {
	if limit == 0 {
		limit = 1000
	}
	unlimited := limit < 0

	var refs []*code.Reference
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketReferences)
		if b == nil {
			return nil
		}
		c := b.Cursor()
		count := 0
		for k, v := c.First(); k != nil && (unlimited || count < limit); k, v = c.Next() {
			var ref code.Reference
			if err := json.Unmarshal(v, &ref); err != nil {
				continue
			}
			refs = append(refs, &ref)
			count++
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return refs, nil
}
