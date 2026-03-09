// grpc_survey_adapter.go provides a gRPC-backed store.SurveyStore adapter.
// This allows secondary MCP instances to access survey data through the gRPC socket
// when another MCP instance is already the primary.
package main

import (
	"context"
	"errors"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/grpcapi"
	"github.com/jmylchreest/aide/aide/pkg/store"
	"github.com/jmylchreest/aide/aide/pkg/survey"
)

// grpcSurveyAdapter implements store.SurveyStore by delegating to a gRPC client.
type grpcSurveyAdapter struct {
	client *grpcapi.Client
}

// Compile-time check that grpcSurveyAdapter implements store.SurveyStore.
var _ store.SurveyStore = (*grpcSurveyAdapter)(nil)

func newGRPCSurveyAdapter(client *grpcapi.Client) *grpcSurveyAdapter {
	return &grpcSurveyAdapter{client: client}
}

// rpcCtx returns a context with the standard gRPC timeout.
func (g *grpcSurveyAdapter) rpcCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), grpcRPCTimeout)
}

func (g *grpcSurveyAdapter) AddEntry(e *survey.Entry) error {
	ctx, cancel := g.rpcCtx()
	defer cancel()

	resp, err := g.client.Survey.Add(ctx, &grpcapi.SurveyAddRequest{
		Analyzer: e.Analyzer,
		Kind:     e.Kind,
		Name:     e.Name,
		FilePath: e.FilePath,
		Title:    e.Title,
		Detail:   e.Detail,
		Metadata: e.Metadata,
	})
	if err != nil {
		return err
	}

	// Update the entry with the server-assigned ID and timestamp.
	if resp.Entry != nil {
		e.ID = resp.Entry.Id
		e.CreatedAt = resp.Entry.CreatedAt.AsTime()
	}
	return nil
}

func (g *grpcSurveyAdapter) GetEntry(id string) (*survey.Entry, error) {
	ctx, cancel := g.rpcCtx()
	defer cancel()

	resp, err := g.client.Survey.Get(ctx, &grpcapi.SurveyGetRequest{Id: id})
	if err != nil {
		return nil, err
	}
	if !resp.Found {
		return nil, store.ErrNotFound
	}

	return protoToSurveyEntry(resp.Entry), nil
}

func (g *grpcSurveyAdapter) DeleteEntry(id string) error {
	ctx, cancel := g.rpcCtx()
	defer cancel()

	_, err := g.client.Survey.Delete(ctx, &grpcapi.SurveyDeleteRequest{Id: id})
	return err
}

func (g *grpcSurveyAdapter) SearchEntries(query string, opts survey.SearchOptions) ([]*survey.SearchResult, error) {
	ctx, cancel := g.rpcCtx()
	defer cancel()

	resp, err := g.client.Survey.Search(ctx, &grpcapi.SurveySearchRequest{
		Query:    query,
		Analyzer: opts.Analyzer,
		Kind:     opts.Kind,
		FilePath: opts.FilePath,
		Limit:    int32(opts.Limit),
	})
	if err != nil {
		return nil, err
	}

	results := make([]*survey.SearchResult, len(resp.Entries))
	for i, pe := range resp.Entries {
		results[i] = &survey.SearchResult{
			Entry: protoToSurveyEntry(pe),
			Score: 1.0, // gRPC doesn't carry scores; default to 1.0
		}
	}
	return results, nil
}

func (g *grpcSurveyAdapter) ListEntries(opts survey.SearchOptions) ([]*survey.Entry, error) {
	ctx, cancel := g.rpcCtx()
	defer cancel()

	resp, err := g.client.Survey.List(ctx, &grpcapi.SurveyListRequest{
		Analyzer: opts.Analyzer,
		Kind:     opts.Kind,
		FilePath: opts.FilePath,
		Limit:    int32(opts.Limit),
	})
	if err != nil {
		return nil, err
	}

	result := make([]*survey.Entry, len(resp.Entries))
	for i, pe := range resp.Entries {
		result[i] = protoToSurveyEntry(pe)
	}
	return result, nil
}

func (g *grpcSurveyAdapter) GetFileEntries(filePath string) ([]*survey.Entry, error) {
	ctx, cancel := g.rpcCtx()
	defer cancel()

	resp, err := g.client.Survey.GetFileEntries(ctx, &grpcapi.SurveyFileRequest{
		FilePath: filePath,
	})
	if err != nil {
		return nil, err
	}

	result := make([]*survey.Entry, len(resp.Entries))
	for i, pe := range resp.Entries {
		result[i] = protoToSurveyEntry(pe)
	}
	return result, nil
}

func (g *grpcSurveyAdapter) ClearAnalyzer(analyzer string) (int, error) {
	ctx, cancel := g.rpcCtx()
	defer cancel()

	resp, err := g.client.Survey.ClearAnalyzer(ctx, &grpcapi.SurveyClearAnalyzerRequest{
		Analyzer: analyzer,
	})
	if err != nil {
		return 0, err
	}
	return int(resp.Count), nil
}

func (g *grpcSurveyAdapter) ReplaceEntriesForAnalyzer(analyzer string, newEntries []*survey.Entry) error {
	return errors.New("ReplaceEntriesForAnalyzer not supported in gRPC client mode")
}

func (g *grpcSurveyAdapter) ReplaceEntriesForAnalyzerAndFile(analyzer, filePath string, newEntries []*survey.Entry) error {
	return errors.New("ReplaceEntriesForAnalyzerAndFile not supported in gRPC client mode")
}

func (g *grpcSurveyAdapter) Stats(_ survey.SearchOptions) (*survey.Stats, error) {
	ctx, cancel := g.rpcCtx()
	defer cancel()

	resp, err := g.client.Survey.Stats(ctx, &grpcapi.SurveyStatsRequest{})
	if err != nil {
		return nil, err
	}

	byAnalyzer := make(map[string]int, len(resp.ByAnalyzer))
	for k, v := range resp.ByAnalyzer {
		byAnalyzer[k] = int(v)
	}
	byKind := make(map[string]int, len(resp.ByKind))
	for k, v := range resp.ByKind {
		byKind[k] = int(v)
	}

	return &survey.Stats{
		Total:      int(resp.Total),
		ByAnalyzer: byAnalyzer,
		ByKind:     byKind,
	}, nil
}

func (g *grpcSurveyAdapter) Clear() error {
	ctx, cancel := g.rpcCtx()
	defer cancel()

	_, err := g.client.Survey.Clear(ctx, &grpcapi.SurveyClearRequest{})
	return err
}

func (g *grpcSurveyAdapter) Close() error {
	// The gRPC connection is owned by the main adapter; don't close here.
	return nil
}

// protoToSurveyEntry converts a protobuf SurveyEntry to the domain Entry type.
func protoToSurveyEntry(pe *grpcapi.SurveyEntry) *survey.Entry {
	if pe == nil {
		return nil
	}
	var createdAt time.Time
	if pe.CreatedAt != nil {
		createdAt = pe.CreatedAt.AsTime()
	}
	return &survey.Entry{
		ID:        pe.Id,
		Analyzer:  pe.Analyzer,
		Kind:      pe.Kind,
		Name:      pe.Name,
		FilePath:  pe.FilePath,
		Title:     pe.Title,
		Detail:    pe.Detail,
		Metadata:  pe.Metadata,
		CreatedAt: createdAt,
	}
}
