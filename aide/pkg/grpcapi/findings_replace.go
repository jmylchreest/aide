package grpcapi

import (
	"context"

	"github.com/jmylchreest/aide/aide/pkg/findings"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ReplaceAnalyzer lets CLI analysis use the same bounded search batches as the
// watcher, instead of creating a search segment and Bolt transaction per finding.
func (s *findingsServiceImpl) ReplaceAnalyzer(ctx context.Context, req *FindingReplaceAnalyzerRequest) (*FindingReplaceAnalyzerResponse, error) {
	scoped, release, err := s.server.checkoutFor(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	fs := scoped.GetFindingsStore()
	if fs == nil {
		return nil, status.Error(codes.Unavailable, "findings store not available")
	}
	items := make([]*findings.Finding, len(req.Findings))
	for i, f := range req.Findings {
		if f == nil || f.Analyzer != req.Analyzer {
			return nil, status.Error(codes.InvalidArgument, "replacement findings must match the analyzer")
		}
		items[i] = &findings.Finding{Analyzer: f.Analyzer, Severity: f.Severity, Category: f.Category, FilePath: f.FilePath, Line: int(f.Line), EndLine: int(f.EndLine), Title: f.Title, Detail: f.Detail, Metadata: f.Metadata}
	}
	if err := ctx.Err(); err != nil {
		return nil, status.FromContextError(err).Err()
	}
	if err := fs.ReplaceFindingsForAnalyzer(req.Analyzer, items); err != nil {
		return nil, err
	}
	return &FindingReplaceAnalyzerResponse{}, nil
}

// AddBatch bounds each append independently for results exceeding one RPC.
// As with the legacy clear/add protocol, a failed run can leave partial results.
func (s *findingsServiceImpl) AddBatch(ctx context.Context, req *FindingAddBatchRequest) (*FindingReplaceAnalyzerResponse, error) {
	scoped, release, err := s.server.checkoutFor(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	fs := scoped.GetFindingsStore()
	if fs == nil {
		return nil, status.Error(codes.Unavailable, "findings store not available")
	}
	if len(req.Findings) > 128 {
		return nil, status.Error(codes.InvalidArgument, "findings batch exceeds 128 records")
	}
	items := make([]*findings.Finding, len(req.Findings))
	for i, f := range req.Findings {
		if f == nil {
			return nil, status.Error(codes.InvalidArgument, "finding is required")
		}
		items[i] = &findings.Finding{Analyzer: f.Analyzer, Severity: f.Severity, Category: f.Category, FilePath: f.FilePath, Line: int(f.Line), EndLine: int(f.EndLine), Title: f.Title, Detail: f.Detail, Metadata: f.Metadata}
	}
	if err := ctx.Err(); err != nil {
		return nil, status.FromContextError(err).Err()
	}
	if batcher, ok := fs.(interface {
		AddFindings([]*findings.Finding) error
	}); ok {
		if err := batcher.AddFindings(items); err != nil {
			return nil, err
		}
	} else {
		for _, f := range items {
			if err := fs.AddFinding(f); err != nil {
				return nil, err
			}
		}
	}
	return &FindingReplaceAnalyzerResponse{}, nil
}
