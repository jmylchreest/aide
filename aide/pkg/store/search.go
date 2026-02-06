// Package store provides storage backends for aide.
// This file implements full-text search using bleve (pure Go, self-contained).
package store

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/analysis/analyzer/custom"
	"github.com/blevesearch/bleve/v2/analysis/token/edgengram"
	"github.com/blevesearch/bleve/v2/analysis/token/lowercase"
	"github.com/blevesearch/bleve/v2/analysis/token/ngram"
	"github.com/blevesearch/bleve/v2/analysis/tokenizer/unicode"
	"github.com/blevesearch/bleve/v2/mapping"
	"github.com/jmylchreest/aide/aide/pkg/memory"
)

// SearchStore provides full-text search using bleve.
type SearchStore struct {
	index bleve.Index
	path  string
}

// SearchResult represents a search match with score.
type SearchResult struct {
	ID       string
	Content  string
	Category string
	Score    float64
	Memory   *memory.Memory
}

// SearchConfig configures the search store.
type SearchConfig struct {
	Path string // Path to persist the search index
}

// buildIndexMapping creates a mapping with multiple analyzers for flexible search.
func buildIndexMapping() (mapping.IndexMapping, error) {
	// Create custom analyzers
	indexMapping := bleve.NewIndexMapping()

	// Standard analyzer (default tokenization + lowercase)
	err := indexMapping.AddCustomAnalyzer("standard_lower", map[string]interface{}{
		"type":      custom.Name,
		"tokenizer": unicode.Name,
		"token_filters": []string{
			lowercase.Name,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create standard analyzer: %w", err)
	}

	// Edge n-gram analyzer for prefix matching (au, aut, auth, authe...)
	err = indexMapping.AddCustomTokenFilter("edge_ngram_filter", map[string]interface{}{
		"type": edgengram.Name,
		"min":  2.0,
		"max":  15.0,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create edge ngram filter: %w", err)
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
		return nil, fmt.Errorf("failed to create edge ngram analyzer: %w", err)
	}

	// N-gram analyzer for substring matching anywhere in word
	err = indexMapping.AddCustomTokenFilter("ngram_filter", map[string]interface{}{
		"type": ngram.Name,
		"min":  3.0,
		"max":  8.0,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create ngram filter: %w", err)
	}

	err = indexMapping.AddCustomAnalyzer("ngram", map[string]interface{}{
		"type":      custom.Name,
		"tokenizer": unicode.Name,
		"token_filters": []string{
			lowercase.Name,
			"ngram_filter",
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create ngram analyzer: %w", err)
	}

	// Create document mapping for memories
	memoryMapping := bleve.NewDocumentMapping()

	// Content field with multiple sub-fields for different analysis
	contentFieldStandard := bleve.NewTextFieldMapping()
	contentFieldStandard.Analyzer = "standard_lower"
	contentFieldStandard.Store = true

	contentFieldEdge := bleve.NewTextFieldMapping()
	contentFieldEdge.Analyzer = "edge_ngram"
	contentFieldEdge.Store = false
	contentFieldEdge.IncludeInAll = false

	contentFieldNgram := bleve.NewTextFieldMapping()
	contentFieldNgram.Analyzer = "ngram"
	contentFieldNgram.Store = false
	contentFieldNgram.IncludeInAll = false

	memoryMapping.AddFieldMappingsAt("content", contentFieldStandard)
	memoryMapping.AddFieldMappingsAt("content_edge", contentFieldEdge)
	memoryMapping.AddFieldMappingsAt("content_ngram", contentFieldNgram)

	// Category and other fields
	categoryField := bleve.NewTextFieldMapping()
	categoryField.Analyzer = "standard_lower"
	memoryMapping.AddFieldMappingsAt("category", categoryField)

	tagsField := bleve.NewTextFieldMapping()
	tagsField.Analyzer = "standard_lower"
	memoryMapping.AddFieldMappingsAt("tags", tagsField)

	planField := bleve.NewTextFieldMapping()
	planField.Analyzer = "standard_lower"
	memoryMapping.AddFieldMappingsAt("plan", planField)

	indexMapping.AddDocumentMapping("memory", memoryMapping)
	indexMapping.DefaultMapping = memoryMapping

	return indexMapping, nil
}

// NewSearchStore creates a new bleve-based search store.
func NewSearchStore(config SearchConfig) (*SearchStore, error) {
	var index bleve.Index
	var err error

	if config.Path != "" {
		// Check if index exists
		if _, statErr := os.Stat(config.Path); os.IsNotExist(statErr) {
			// Create new index
			mapping, mapErr := buildIndexMapping()
			if mapErr != nil {
				return nil, mapErr
			}
			index, err = bleve.New(config.Path, mapping)
		} else {
			// Open existing index
			index, err = bleve.Open(config.Path)
		}
	} else {
		// In-memory index
		mapping, mapErr := buildIndexMapping()
		if mapErr != nil {
			return nil, mapErr
		}
		index, err = bleve.NewMemOnly(mapping)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create/open search index: %w", err)
	}

	return &SearchStore{
		index: index,
		path:  config.Path,
	}, nil
}

// searchDocument is the structure we index in bleve.
type searchDocument struct {
	ID           string   `json:"id"`
	Content      string   `json:"content"`
	ContentEdge  string   `json:"content_edge"`
	ContentNgram string   `json:"content_ngram"`
	Category     string   `json:"category"`
	Tags         []string `json:"tags"`
	Plan         string   `json:"plan"`
	Agent        string   `json:"agent"`
}

// IndexMemory adds a memory to the search index.
func (s *SearchStore) IndexMemory(m *memory.Memory) error {
	doc := searchDocument{
		ID:           m.ID,
		Content:      m.Content,
		ContentEdge:  m.Content, // Same content, different analyzer
		ContentNgram: m.Content, // Same content, different analyzer
		Category:     string(m.Category),
		Tags:         m.Tags,
		Plan:         m.Plan,
		Agent:        m.Agent,
	}

	return s.index.Index(m.ID, doc)
}

// DeleteMemory removes a memory from the search index.
func (s *SearchStore) DeleteMemory(id string) error {
	return s.index.Delete(id)
}

// Search performs a flexible search across all analyzers with fuzzy matching.
func (s *SearchStore) Search(queryStr string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 10
	}

	// Build individual queries for each search type
	// Standard match on content
	standardQuery := bleve.NewMatchQuery(queryStr)
	standardQuery.SetField("content")

	// Fuzzy match (handles typos)
	fuzzyQuery := bleve.NewFuzzyQuery(queryStr)
	fuzzyQuery.SetField("content")
	fuzzyQuery.SetFuzziness(1)

	// Match on edge ngram field (prefix matching)
	edgeQuery := bleve.NewMatchQuery(queryStr)
	edgeQuery.SetField("content_edge")

	// Match on ngram field (substring matching)
	ngramQuery := bleve.NewMatchQuery(queryStr)
	ngramQuery.SetField("content_ngram")

	// Combine with disjunction (OR) - any match counts
	disjunction := bleve.NewDisjunctionQuery(standardQuery, fuzzyQuery, edgeQuery, ngramQuery)

	searchRequest := bleve.NewSearchRequest(disjunction)
	searchRequest.Size = limit
	searchRequest.Fields = []string{"content", "category", "tags", "plan"}

	searchResults, err := s.index.Search(searchRequest)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	results := make([]SearchResult, 0, len(searchResults.Hits))
	for _, hit := range searchResults.Hits {
		content := ""
		category := ""
		if c, ok := hit.Fields["content"].(string); ok {
			content = c
		}
		if c, ok := hit.Fields["category"].(string); ok {
			category = c
		}

		results = append(results, SearchResult{
			ID:       hit.ID,
			Content:  content,
			Category: category,
			Score:    hit.Score,
		})
	}

	return results, nil
}

// Count returns the number of documents in the index.
func (s *SearchStore) Count() (uint64, error) {
	return s.index.DocCount()
}

// Close closes the search index.
func (s *SearchStore) Close() error {
	return s.index.Close()
}

// Reindex rebuilds the search index from a list of memories.
func (s *SearchStore) Reindex(memories []*memory.Memory) error {
	batch := s.index.NewBatch()
	for _, m := range memories {
		doc := searchDocument{
			ID:           m.ID,
			Content:      m.Content,
			ContentEdge:  m.Content,
			ContentNgram: m.Content,
			Category:     string(m.Category),
			Tags:         m.Tags,
			Plan:         m.Plan,
			Agent:        m.Agent,
		}
		batch.Index(m.ID, doc)
	}
	return s.index.Batch(batch)
}

// Clear removes all documents from the search index by recreating it.
func (s *SearchStore) Clear() error {
	// Close the existing index
	if err := s.index.Close(); err != nil {
		return err
	}

	// Remove and recreate the index
	if err := os.RemoveAll(s.path); err != nil {
		return err
	}

	indexMapping, err := buildIndexMapping()
	if err != nil {
		return err
	}

	s.index, err = bleve.New(s.path, indexMapping)
	return err
}

// GetSearchPath returns the default search index path given a db path.
func GetSearchPath(dbPath string) string {
	return filepath.Join(filepath.Dir(dbPath), "search.bleve")
}
