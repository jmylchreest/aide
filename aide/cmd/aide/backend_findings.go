package main

import (
	"context"
	"fmt"

	"github.com/jmylchreest/aide/aide/pkg/code"
	"github.com/jmylchreest/aide/aide/pkg/findings"
	"github.com/jmylchreest/aide/aide/pkg/grammar"
	"github.com/jmylchreest/aide/aide/pkg/grpcapi"
	"github.com/jmylchreest/aide/aide/pkg/grpcapi/adapter"
	"github.com/jmylchreest/aide/aide/pkg/store"
)

// DeadCodeAnalysisOptions controls behaviour of RunDeadCodeAnalysis.
type DeadCodeAnalysisOptions struct {
	IncludeExported bool
	Progress        func(checked, found int)
}

// DeadCodeAnalysisResult is the CLI-facing result of a dead-code analyzer run.
type DeadCodeAnalysisResult struct {
	SymbolsChecked int
	SymbolsSkipped int
	FindingsCount  int
	DurationMs     int64
}

// RunDeadCodeAnalysis runs the dead-code analyzer.
func (b *Backend) RunDeadCodeAnalysis(opts DeadCodeAnalysisOptions) (*DeadCodeAnalysisResult, error) {
	if b.useGRPC {
		ctx, cancel := context.WithTimeout(context.Background(), DeadCodeAnalysisRPCTimeout)
		defer cancel()

		resp, err := b.grpcClient.Code.RunDeadCodeAnalysis(ctx, &grpcapi.CodeRunDeadCodeAnalysisRequest{
			IncludeExported: opts.IncludeExported,
		})
		if err != nil {
			return nil, err
		}
		return &DeadCodeAnalysisResult{
			SymbolsChecked: int(resp.SymbolsChecked),
			SymbolsSkipped: int(resp.SymbolsSkipped),
			FindingsCount:  int(resp.FindingsCount),
			DurationMs:     resp.DurationMs,
		}, nil
	}

	codeStore, err := b.openCodeStore()
	if err != nil {
		return nil, fmt.Errorf("code index required: %w", err)
	}
	defer codeStore.Close()

	stats, err := codeStore.Stats()
	if err != nil || stats.Symbols == 0 {
		return nil, fmt.Errorf("code index is empty — run 'aide code index' first")
	}

	registry := grammar.DefaultPackRegistry()
	cfg := findings.DeadCodeConfig{
		GetAllSymbols: func() ([]*code.Symbol, error) {
			return codeStore.ListAllSymbols(-1)
		},
		GetRefCount: func(name string) (int, error) {
			refs, err := codeStore.SearchReferences(code.ReferenceSearchOptions{
				SymbolName: name,
				Limit:      1,
			})
			if err != nil {
				return 0, err
			}
			return len(refs), nil
		},
		ProjectRoot:        projectRoot(b.dbPath),
		ProgressFn:         opts.Progress,
		PackProvider:       registry.Get,
		IncludeExported:    opts.IncludeExported,
		ConsumerExtensions: registry.ConsumerExtensions(),
	}

	ff, result, err := findings.AnalyzeDeadCode(cfg)
	if err != nil {
		return nil, err
	}

	if err := b.ReplaceFindingsForAnalyzer(findings.AnalyzerDeadCode, ff); err != nil {
		return nil, fmt.Errorf("failed to store findings: %w", err)
	}

	return &DeadCodeAnalysisResult{
		SymbolsChecked: result.SymbolsChecked,
		SymbolsSkipped: result.SymbolsSkipped,
		FindingsCount:  result.FindingsCount,
		DurationMs:     result.Duration.Milliseconds(),
	}, nil
}

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
				Finding: adapter.ProtoToFinding(pf),
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
			result = append(result, adapter.ProtoToFinding(pf))
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

func (b *Backend) GetFindingsStats(opts findings.SearchOptions) (*findings.Stats, error) {
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

	return fs.Stats(opts)
}

func (b *Backend) AcceptFindings(ids []string) (int, error) {
	ctx := context.Background()

	if b.useGRPC {
		resp, err := b.grpcClient.Findings.Accept(ctx, &grpcapi.FindingAcceptRequest{
			Ids: ids,
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

	return fs.AcceptFindings(ids)
}

func (b *Backend) AcceptFindingsByFilter(opts findings.SearchOptions) (int, error) {
	ctx := context.Background()

	if b.useGRPC {
		resp, err := b.grpcClient.Findings.AcceptByFilter(ctx, &grpcapi.FindingAcceptByFilterRequest{
			Analyzer: opts.Analyzer,
			Severity: opts.Severity,
			FilePath: opts.FilePath,
			Category: opts.Category,
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

	return fs.AcceptFindingsByFilter(opts)
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
