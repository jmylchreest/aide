package main

import (
	"context"

	"github.com/jmylchreest/aide/aide/pkg/findings"
	"github.com/jmylchreest/aide/aide/pkg/grpcapi"
	"github.com/jmylchreest/aide/aide/pkg/store"
)

// =============================================================================
// Findings Backend Operations
// =============================================================================

// openFindingsStore opens the findings store for direct access.
func (b *Backend) openFindingsStore() (store.FindingsStore, error) {
	findingsDir := getFindingsStorePath(b.dbPath)
	return store.NewFindingsStore(findingsDir)
}

func (b *Backend) SearchFindings(query string, opts findings.SearchOptions) ([]*findings.SearchResult, error) {
	ctx := context.Background()

	if b.useGRPC {
		resp, err := b.grpcClient.Findings.Search(ctx, &grpcapi.FindingSearchRequest{
			Query:    query,
			Analyzer: opts.Analyzer,
			Severity: opts.Severity,
			FilePath: opts.FilePath,
			Category: opts.Category,
			Limit:    int32(opts.Limit),
		})
		if err != nil {
			return nil, err
		}
		results := make([]*findings.SearchResult, 0, len(resp.Findings))
		for _, pf := range resp.Findings {
			results = append(results, &findings.SearchResult{
				Finding: protoToFinding(pf),
			})
		}
		return results, nil
	}

	fs, err := b.openFindingsStore()
	if err != nil {
		return nil, err
	}
	defer fs.Close()

	return fs.SearchFindings(query, opts)
}

func (b *Backend) ListFindings(opts findings.SearchOptions) ([]*findings.Finding, error) {
	ctx := context.Background()

	if b.useGRPC {
		resp, err := b.grpcClient.Findings.List(ctx, &grpcapi.FindingListRequest{
			Analyzer: opts.Analyzer,
			Severity: opts.Severity,
			FilePath: opts.FilePath,
			Category: opts.Category,
			Limit:    int32(opts.Limit),
		})
		if err != nil {
			return nil, err
		}
		result := make([]*findings.Finding, 0, len(resp.Findings))
		for _, pf := range resp.Findings {
			result = append(result, protoToFinding(pf))
		}
		return result, nil
	}

	fs, err := b.openFindingsStore()
	if err != nil {
		return nil, err
	}
	defer fs.Close()

	return fs.ListFindings(opts)
}

func (b *Backend) GetFindingsStats() (*findings.Stats, error) {
	ctx := context.Background()

	if b.useGRPC {
		resp, err := b.grpcClient.Findings.Stats(ctx, &grpcapi.FindingStatsRequest{})
		if err != nil {
			return nil, err
		}
		byAnalyzer := make(map[string]int, len(resp.ByAnalyzer))
		for k, v := range resp.ByAnalyzer {
			byAnalyzer[k] = int(v)
		}
		bySeverity := make(map[string]int, len(resp.BySeverity))
		for k, v := range resp.BySeverity {
			bySeverity[k] = int(v)
		}
		return &findings.Stats{
			Total:      int(resp.Total),
			ByAnalyzer: byAnalyzer,
			BySeverity: bySeverity,
		}, nil
	}

	fs, err := b.openFindingsStore()
	if err != nil {
		return nil, err
	}
	defer fs.Close()

	return fs.Stats()
}

func (b *Backend) ClearFindings() error {
	ctx := context.Background()

	if b.useGRPC {
		_, err := b.grpcClient.Findings.Clear(ctx, &grpcapi.FindingClearRequest{})
		return err
	}

	fs, err := b.openFindingsStore()
	if err != nil {
		return err
	}
	defer fs.Close()

	return fs.Clear()
}

func (b *Backend) ClearFindingsAnalyzer(analyzer string) (int, error) {
	ctx := context.Background()

	if b.useGRPC {
		resp, err := b.grpcClient.Findings.ClearAnalyzer(ctx, &grpcapi.FindingClearAnalyzerRequest{
			Analyzer: analyzer,
		})
		if err != nil {
			return 0, err
		}
		return int(resp.Count), nil
	}

	fs, err := b.openFindingsStore()
	if err != nil {
		return 0, err
	}
	defer fs.Close()

	return fs.ClearAnalyzer(analyzer)
}

// AddFinding stores a single finding via gRPC or direct access.
func (b *Backend) AddFinding(f *findings.Finding) error {
	ctx := context.Background()

	if b.useGRPC {
		_, err := b.grpcClient.Findings.Add(ctx, &grpcapi.FindingAddRequest{
			Analyzer: f.Analyzer,
			Severity: f.Severity,
			Category: f.Category,
			FilePath: f.FilePath,
			Line:     int32(f.Line),
			EndLine:  int32(f.EndLine),
			Title:    f.Title,
			Detail:   f.Detail,
			Metadata: f.Metadata,
		})
		return err
	}

	fs, err := b.openFindingsStore()
	if err != nil {
		return err
	}
	defer fs.Close()

	return fs.AddFinding(f)
}

func (b *Backend) ReplaceFindingsForAnalyzer(analyzer string, ff []*findings.Finding) error {
	if b.useGRPC {
		return b.grpcFindingsReplaceForAnalyzer(analyzer, ff)
	}

	fs, err := b.openFindingsStore()
	if err != nil {
		return err
	}
	defer fs.Close()

	return fs.ReplaceFindingsForAnalyzer(analyzer, ff)
}

// grpcFindingsReplaceForAnalyzer replaces all findings for an analyzer over gRPC.
// NOTE: This is non-atomic — it clears then re-adds one by one. If the process
// crashes mid-way, the analyzer's findings will be partially populated. This is
// acceptable for the CLI use-case where the server holds the authoritative store.
func (b *Backend) grpcFindingsReplaceForAnalyzer(analyzer string, ff []*findings.Finding) error {
	ctx := context.Background()

	if _, err := b.grpcClient.Findings.ClearAnalyzer(ctx, &grpcapi.FindingClearAnalyzerRequest{
		Analyzer: analyzer,
	}); err != nil {
		return err
	}

	for _, f := range ff {
		_, err := b.grpcClient.Findings.Add(ctx, &grpcapi.FindingAddRequest{
			Analyzer: f.Analyzer,
			Severity: f.Severity,
			Category: f.Category,
			FilePath: f.FilePath,
			Line:     int32(f.Line),
			EndLine:  int32(f.EndLine),
			Title:    f.Title,
			Detail:   f.Detail,
			Metadata: f.Metadata,
		})
		if err != nil {
			return err
		}
	}
	return nil
}
