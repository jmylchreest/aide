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
	"github.com/jmylchreest/aide/aide/pkg/findings"
)

var (
	BucketFindings     = []byte("findings")
	BucketFindingsMeta = []byte("findings_meta")
)

// FindingsStoreImpl implements FindingsStore using BoltDB + Bleve,
// backed by the generic searchableStore.
type FindingsStoreImpl struct {
	*searchableStore[findings.Finding]
}

// NewFindingsStore opens or creates a findings store at the given directory.
func NewFindingsStore(dir string) (*FindingsStoreImpl, error) {
	ss, err := newSearchableStore(searchableStoreConfig[findings.Finding]{
		Dir:            dir,
		DBFilename:     "findings.db",
		BucketName:     BucketFindings,
		MetaBucketName: BucketFindingsMeta,
		StoreName:      "findings",

		BuildIndexMapping: buildFindingsIndexMapping,
		ToSearchDoc:       findingToSearchDoc,

		GetID:        func(f *findings.Finding) string { return f.ID },
		SetID:        func(f *findings.Finding, id string) { f.ID = id },
		GetCreatedAt: func(f *findings.Finding) time.Time { return f.CreatedAt },
		SetCreatedAt: func(f *findings.Finding, t time.Time) { f.CreatedAt = t },
		GetAnalyzer:  func(f *findings.Finding) string { return f.Analyzer },

		RunMigrations:     RunFindingsMigrations,
		LegacySearchPaths: []string{"findings.idx"},
	})
	if err != nil {
		return nil, err
	}
	return &FindingsStoreImpl{searchableStore: ss}, nil
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

func findingToSearchDoc(f *findings.Finding) map[string]interface{} {
	return map[string]interface{}{
		"title":    f.Title,
		"detail":   f.Detail,
		"analyzer": f.Analyzer,
		"severity": f.Severity,
		"category": f.Category,
		"file":     f.FilePath,
		"accepted": f.Accepted,
	}
}

// findingsMatchFn returns a predicate that filters findings by the given options.
func findingsMatchFn(opts findings.SearchOptions) func(*findings.Finding) bool {
	return func(f *findings.Finding) bool {
		if !opts.IncludeAccepted && f.Accepted {
			return false
		}
		if opts.Analyzer != "" && f.Analyzer != opts.Analyzer {
			return false
		}
		if opts.Severity != "" && f.Severity != opts.Severity {
			return false
		}
		if opts.FilePath != "" && !strings.Contains(f.FilePath, opts.FilePath) {
			return false
		}
		if opts.Category != "" && f.Category != opts.Category {
			return false
		}
		return true
	}
}

// Close closes the findings store.
func (s *FindingsStoreImpl) Close() error {
	return s.searchableStore.Close()
}

// AddFinding stores a finding and indexes it for search.
func (s *FindingsStoreImpl) AddFinding(f *findings.Finding) error {
	return s.Add(f)
}

// GetFinding retrieves a finding by ID.
func (s *FindingsStoreImpl) GetFinding(id string) (*findings.Finding, error) {
	return s.Get(id)
}

// DeleteFinding removes a finding by ID.
func (s *FindingsStoreImpl) DeleteFinding(id string) error {
	return s.Delete(id)
}

// SearchFindings performs full-text search on findings.
func (s *FindingsStoreImpl) SearchFindings(queryStr string, opts findings.SearchOptions) ([]*findings.SearchResult, error) {
	// Build bleve filter queries for the search-level filters.
	var filters []query.Query
	if opts.Analyzer != "" {
		q := bleve.NewTermQuery(opts.Analyzer)
		q.SetField("analyzer")
		filters = append(filters, q)
	}
	if opts.Severity != "" {
		q := bleve.NewTermQuery(opts.Severity)
		q.SetField("severity")
		filters = append(filters, q)
	}
	if opts.FilePath != "" {
		q := bleve.NewTermQuery(opts.FilePath)
		q.SetField("file")
		filters = append(filters, q)
	}
	if opts.Category != "" {
		q := bleve.NewTermQuery(opts.Category)
		q.SetField("category")
		filters = append(filters, q)
	}

	hits, err := s.searchBleve(queryStr, filters, opts.Limit, findings.DefaultSearchLimit)
	if err != nil {
		return nil, err
	}

	var results []*findings.SearchResult
	for _, hit := range hits {
		f, err := s.Get(hit.ID)
		if err != nil {
			continue
		}
		if !opts.IncludeAccepted && f.Accepted {
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
	return s.list(findingsMatchFn(opts), opts.Limit, findings.DefaultListLimit)
}

// GetFileFindings returns all findings for a specific file.
func (s *FindingsStoreImpl) GetFileFindings(filePath string) ([]*findings.Finding, error) {
	return s.ListFindings(findings.SearchOptions{FilePath: filePath, Limit: 1000})
}

// ClearAnalyzer removes all findings for a specific analyzer.
func (s *FindingsStoreImpl) ClearAnalyzer(analyzer string) (int, error) {
	return s.searchableStore.ClearAnalyzer(analyzer)
}

// AcceptFindings marks findings with the given IDs as accepted.
func (s *FindingsStoreImpl) AcceptFindings(ids []string) (int, error) {
	accepted := 0
	for _, id := range ids {
		f, err := s.Get(id)
		if err != nil {
			continue // skip missing IDs
		}
		if f.Accepted {
			continue // already accepted
		}
		f.Accepted = true
		if err := s.updateItem(f); err != nil {
			return accepted, err
		}
		accepted++
	}
	return accepted, nil
}

// AcceptFindingsByFilter marks all findings matching the filter as accepted.
func (s *FindingsStoreImpl) AcceptFindingsByFilter(opts findings.SearchOptions) (int, error) {
	// Collect all matching findings (no limit, include already-accepted).
	filterOpts := opts
	filterOpts.Limit = -1
	filterOpts.IncludeAccepted = true

	all, err := s.ListFindings(filterOpts)
	if err != nil {
		return 0, err
	}

	var ids []string
	for _, f := range all {
		if !f.Accepted {
			ids = append(ids, f.ID)
		}
	}

	return s.AcceptFindings(ids)
}

// Stats returns aggregate finding counts, optionally filtering by SearchOptions.
func (s *FindingsStoreImpl) Stats(opts findings.SearchOptions) (*findings.Stats, error) {
	all, err := s.allMatching(findingsMatchFn(opts))
	if err != nil {
		return nil, err
	}

	stats := &findings.Stats{
		ByAnalyzer: make(map[string]int),
		BySeverity: make(map[string]int),
	}
	for _, f := range all {
		stats.Total++
		stats.ByAnalyzer[f.Analyzer]++
		stats.BySeverity[f.Severity]++
	}
	return stats, nil
}

// ReplaceFindingsForAnalyzer atomically replaces all findings for an analyzer.
// On success, old findings are gone and new ones are stored.
// On error, old findings remain untouched.
func (s *FindingsStoreImpl) ReplaceFindingsForAnalyzer(analyzer string, newFindings []*findings.Finding) error {
	return s.replace(func(f *findings.Finding) bool {
		return f.Analyzer == analyzer
	}, newFindings)
}

// ReplaceFindingsForAnalyzerAndFile atomically replaces findings for an analyzer within a specific file.
// Used for per-file incremental updates (complexity, secrets).
func (s *FindingsStoreImpl) ReplaceFindingsForAnalyzerAndFile(analyzer, filePath string, newFindings []*findings.Finding) error {
	return s.replace(func(f *findings.Finding) bool {
		return f.Analyzer == analyzer && f.FilePath == filePath
	}, newFindings)
}

// Clear removes all findings.
func (s *FindingsStoreImpl) Clear() error {
	return s.searchableStore.Clear()
}
