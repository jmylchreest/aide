// grpc_findings_adapter.go provides a gRPC-backed store.FindingsStore adapter.
// This allows secondary MCP instances to access findings through the gRPC socket
// when another MCP instance is already the primary.
package main

import (
	"context"
	"errors"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/findings"
	"github.com/jmylchreest/aide/aide/pkg/grpcapi"
	"github.com/jmylchreest/aide/aide/pkg/store"
)

// grpcFindingsAdapter implements store.FindingsStore by delegating to a gRPC client.
type grpcFindingsAdapter struct {
	client *grpcapi.Client
}

// Compile-time check that grpcFindingsAdapter implements store.FindingsStore.
var _ store.FindingsStore = (*grpcFindingsAdapter)(nil)

func newGRPCFindingsAdapter(client *grpcapi.Client) *grpcFindingsAdapter {
	return &grpcFindingsAdapter{client: client}
}

// rpcCtx returns a context with the standard gRPC timeout.
func (g *grpcFindingsAdapter) rpcCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), grpcRPCTimeout)
}

func (g *grpcFindingsAdapter) AddFinding(f *findings.Finding) error {
	ctx, cancel := g.rpcCtx()
	defer cancel()

	resp, err := g.client.Findings.Add(ctx, &grpcapi.FindingAddRequest{
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

	// Update the finding with the server-assigned ID and timestamp.
	if resp.Finding != nil {
		f.ID = resp.Finding.Id
		f.CreatedAt = resp.Finding.CreatedAt.AsTime()
	}
	return nil
}

func (g *grpcFindingsAdapter) GetFinding(id string) (*findings.Finding, error) {
	ctx, cancel := g.rpcCtx()
	defer cancel()

	resp, err := g.client.Findings.Get(ctx, &grpcapi.FindingGetRequest{Id: id})
	if err != nil {
		return nil, err
	}
	if !resp.Found {
		return nil, store.ErrNotFound
	}

	return protoToFinding(resp.Finding), nil
}

func (g *grpcFindingsAdapter) DeleteFinding(id string) error {
	ctx, cancel := g.rpcCtx()
	defer cancel()

	_, err := g.client.Findings.Delete(ctx, &grpcapi.FindingDeleteRequest{Id: id})
	return err
}

func (g *grpcFindingsAdapter) SearchFindings(query string, opts findings.SearchOptions) ([]*findings.SearchResult, error) {
	ctx, cancel := g.rpcCtx()
	defer cancel()

	resp, err := g.client.Findings.Search(ctx, &grpcapi.FindingSearchRequest{
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

	results := make([]*findings.SearchResult, len(resp.Findings))
	for i, pf := range resp.Findings {
		results[i] = &findings.SearchResult{
			Finding: protoToFinding(pf),
			Score:   1.0, // gRPC doesn't carry scores; default to 1.0
		}
	}
	return results, nil
}

func (g *grpcFindingsAdapter) ListFindings(opts findings.SearchOptions) ([]*findings.Finding, error) {
	ctx, cancel := g.rpcCtx()
	defer cancel()

	resp, err := g.client.Findings.List(ctx, &grpcapi.FindingListRequest{
		Analyzer: opts.Analyzer,
		Severity: opts.Severity,
		FilePath: opts.FilePath,
		Category: opts.Category,
		Limit:    int32(opts.Limit),
	})
	if err != nil {
		return nil, err
	}

	result := make([]*findings.Finding, len(resp.Findings))
	for i, pf := range resp.Findings {
		result[i] = protoToFinding(pf)
	}
	return result, nil
}

func (g *grpcFindingsAdapter) GetFileFindings(filePath string) ([]*findings.Finding, error) {
	ctx, cancel := g.rpcCtx()
	defer cancel()

	resp, err := g.client.Findings.GetFileFindings(ctx, &grpcapi.FindingFileRequest{
		FilePath: filePath,
	})
	if err != nil {
		return nil, err
	}

	result := make([]*findings.Finding, len(resp.Findings))
	for i, pf := range resp.Findings {
		result[i] = protoToFinding(pf)
	}
	return result, nil
}

func (g *grpcFindingsAdapter) ClearAnalyzer(analyzer string) (int, error) {
	ctx, cancel := g.rpcCtx()
	defer cancel()

	resp, err := g.client.Findings.ClearAnalyzer(ctx, &grpcapi.FindingClearAnalyzerRequest{
		Analyzer: analyzer,
	})
	if err != nil {
		return 0, err
	}
	return int(resp.Count), nil
}

func (g *grpcFindingsAdapter) ReplaceFindingsForAnalyzer(analyzer string, newFindings []*findings.Finding) error {
	return errors.New("ReplaceFindingsForAnalyzer not supported in gRPC client mode")
}

func (g *grpcFindingsAdapter) ReplaceFindingsForAnalyzerAndFile(analyzer, filePath string, newFindings []*findings.Finding) error {
	return errors.New("ReplaceFindingsForAnalyzerAndFile not supported in gRPC client mode")
}

func (g *grpcFindingsAdapter) Stats() (*findings.Stats, error) {
	ctx, cancel := g.rpcCtx()
	defer cancel()

	resp, err := g.client.Findings.Stats(ctx, &grpcapi.FindingStatsRequest{})
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

func (g *grpcFindingsAdapter) Clear() error {
	ctx, cancel := g.rpcCtx()
	defer cancel()

	_, err := g.client.Findings.Clear(ctx, &grpcapi.FindingClearRequest{})
	return err
}

func (g *grpcFindingsAdapter) Close() error {
	// The gRPC connection is owned by the main adapter; don't close here.
	return nil
}

// protoToFinding converts a protobuf Finding to the domain Finding type.
func protoToFinding(pf *grpcapi.Finding) *findings.Finding {
	if pf == nil {
		return nil
	}
	var createdAt time.Time
	if pf.CreatedAt != nil {
		createdAt = pf.CreatedAt.AsTime()
	}
	return &findings.Finding{
		ID:        pf.Id,
		Analyzer:  pf.Analyzer,
		Severity:  pf.Severity,
		Category:  pf.Category,
		FilePath:  pf.FilePath,
		Line:      int(pf.Line),
		EndLine:   int(pf.EndLine),
		Title:     pf.Title,
		Detail:    pf.Detail,
		Metadata:  pf.Metadata,
		CreatedAt: createdAt,
	}
}
