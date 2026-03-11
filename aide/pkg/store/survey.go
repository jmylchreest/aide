package store

import (
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
)

var (
	BucketSurvey     = []byte("survey_entries")
	BucketSurveyMeta = []byte("survey_meta")
)

// SurveyStoreImpl implements SurveyStore using BoltDB + Bleve,
// backed by the generic searchableStore.
type SurveyStoreImpl struct {
	*searchableStore[survey.Entry]
}

// NewSurveyStore opens or creates a survey store at the given directory.
func NewSurveyStore(dir string) (*SurveyStoreImpl, error) {
	ss, err := newSearchableStore(searchableStoreConfig[survey.Entry]{
		Dir:            dir,
		DBFilename:     "survey.db",
		BucketName:     BucketSurvey,
		MetaBucketName: BucketSurveyMeta,
		StoreName:      "survey",

		BuildIndexMapping: buildSurveyIndexMapping,
		ToSearchDoc:       surveyEntryToSearchDoc,

		GetID:        func(e *survey.Entry) string { return e.ID },
		SetID:        func(e *survey.Entry, id string) { e.ID = id },
		GetCreatedAt: func(e *survey.Entry) time.Time { return e.CreatedAt },
		SetCreatedAt: func(e *survey.Entry, t time.Time) { e.CreatedAt = t },
		GetAnalyzer:  func(e *survey.Entry) string { return e.Analyzer },

		RunMigrations: RunSurveyMigrations,
	})
	if err != nil {
		return nil, err
	}
	return &SurveyStoreImpl{searchableStore: ss}, nil
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

// surveyMatchFn returns a predicate that filters survey entries by the given options.
func surveyMatchFn(opts survey.SearchOptions) func(*survey.Entry) bool {
	return func(e *survey.Entry) bool {
		if opts.Analyzer != "" && e.Analyzer != opts.Analyzer {
			return false
		}
		if opts.Kind != "" && e.Kind != opts.Kind {
			return false
		}
		if opts.FilePath != "" && !strings.Contains(e.FilePath, opts.FilePath) {
			return false
		}
		return true
	}
}

// Close closes the survey store.
func (s *SurveyStoreImpl) Close() error {
	return s.searchableStore.Close()
}

// AddEntry stores a survey entry and indexes it for search.
func (s *SurveyStoreImpl) AddEntry(e *survey.Entry) error {
	return s.Add(e)
}

// GetEntry retrieves a survey entry by ID.
func (s *SurveyStoreImpl) GetEntry(id string) (*survey.Entry, error) {
	return s.Get(id)
}

// DeleteEntry removes a survey entry by ID.
func (s *SurveyStoreImpl) DeleteEntry(id string) error {
	return s.Delete(id)
}

// SearchEntries performs full-text search on survey entries.
func (s *SurveyStoreImpl) SearchEntries(queryStr string, opts survey.SearchOptions) ([]*survey.SearchResult, error) {
	// Build bleve filter queries for the search-level filters.
	var filters []query.Query
	if opts.Analyzer != "" {
		q := bleve.NewTermQuery(opts.Analyzer)
		q.SetField("analyzer")
		filters = append(filters, q)
	}
	if opts.Kind != "" {
		q := bleve.NewTermQuery(opts.Kind)
		q.SetField("kind")
		filters = append(filters, q)
	}
	if opts.FilePath != "" {
		q := bleve.NewTermQuery(opts.FilePath)
		q.SetField("file")
		filters = append(filters, q)
	}

	hits, err := s.searchBleve(queryStr, filters, opts.Limit, survey.DefaultSearchLimit)
	if err != nil {
		return nil, err
	}

	var results []*survey.SearchResult
	for _, hit := range hits {
		e, err := s.Get(hit.ID)
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
	return s.list(surveyMatchFn(opts), opts.Limit, survey.DefaultListLimit)
}

// GetFileEntries returns all survey entries for a specific file.
func (s *SurveyStoreImpl) GetFileEntries(filePath string) ([]*survey.Entry, error) {
	return s.ListEntries(survey.SearchOptions{FilePath: filePath, Limit: 1000})
}

// ClearAnalyzer removes all survey entries for a specific analyzer.
func (s *SurveyStoreImpl) ClearAnalyzer(analyzer string) (int, error) {
	return s.searchableStore.ClearAnalyzer(analyzer)
}

// Stats returns aggregate survey entry counts, optionally filtering by SearchOptions.
func (s *SurveyStoreImpl) Stats(opts survey.SearchOptions) (*survey.Stats, error) {
	all, err := s.allMatching(surveyMatchFn(opts))
	if err != nil {
		return nil, err
	}

	stats := &survey.Stats{
		ByAnalyzer: make(map[string]int),
		ByKind:     make(map[string]int),
	}
	for _, e := range all {
		stats.Total++
		stats.ByAnalyzer[e.Analyzer]++
		stats.ByKind[e.Kind]++
	}
	return stats, nil
}

// ReplaceEntriesForAnalyzer atomically replaces all entries for an analyzer.
// On success, old entries are gone and new ones are stored.
// On error, old entries remain untouched.
func (s *SurveyStoreImpl) ReplaceEntriesForAnalyzer(analyzer string, newEntries []*survey.Entry) error {
	return s.replace(func(e *survey.Entry) bool {
		return e.Analyzer == analyzer
	}, newEntries)
}

// ReplaceEntriesForAnalyzerAndFile atomically replaces entries for an analyzer within a specific file.
func (s *SurveyStoreImpl) ReplaceEntriesForAnalyzerAndFile(analyzer, filePath string, newEntries []*survey.Entry) error {
	return s.replace(func(e *survey.Entry) bool {
		return e.Analyzer == analyzer && e.FilePath == filePath
	}, newEntries)
}

// Clear removes all survey entries.
func (s *SurveyStoreImpl) Clear() error {
	return s.searchableStore.Clear()
}
