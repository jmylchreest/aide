package adapter

import (
	"context"
	"errors"

	"github.com/jmylchreest/aide/aide/pkg/grpcapi"
	"github.com/jmylchreest/aide/aide/pkg/store"
	"github.com/jmylchreest/aide/aide/pkg/survey"
)

// SurveyAdapter implements store.SurveyStore by delegating to a gRPC client.
type SurveyAdapter struct {
	client *grpcapi.Client
}

// Compile-time check that SurveyAdapter implements store.SurveyStore.
var _ store.SurveyStore = (*SurveyAdapter)(nil)

// NewSurveyAdapter creates a new gRPC-backed survey adapter.
func NewSurveyAdapter(client *grpcapi.Client) *SurveyAdapter {
	return &SurveyAdapter{client: client}
}

func (g *SurveyAdapter) rpcCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), RPCTimeout)
}

func (g *SurveyAdapter) AddEntry(e *survey.Entry) error {
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

	if resp.Entry != nil {
		e.ID = resp.Entry.Id
		e.CreatedAt = resp.Entry.CreatedAt.AsTime()
	}
	return nil
}

func (g *SurveyAdapter) GetEntry(id string) (*survey.Entry, error) {
	ctx, cancel := g.rpcCtx()
	defer cancel()

	resp, err := g.client.Survey.Get(ctx, &grpcapi.SurveyGetRequest{Id: id})
	if err != nil {
		return nil, err
	}
	if !resp.Found {
		return nil, store.ErrNotFound
	}

	return ProtoToSurveyEntry(resp.Entry), nil
}

func (g *SurveyAdapter) DeleteEntry(id string) error {
	ctx, cancel := g.rpcCtx()
	defer cancel()

	_, err := g.client.Survey.Delete(ctx, &grpcapi.SurveyDeleteRequest{Id: id})
	return err
}

func (g *SurveyAdapter) SearchEntries(query string, opts survey.SearchOptions) ([]*survey.SearchResult, error) {
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
			Entry: ProtoToSurveyEntry(pe),
			Score: 1.0,
		}
	}
	return results, nil
}

func (g *SurveyAdapter) ListEntries(opts survey.SearchOptions) ([]*survey.Entry, error) {
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
		result[i] = ProtoToSurveyEntry(pe)
	}
	return result, nil
}

func (g *SurveyAdapter) GetFileEntries(filePath string) ([]*survey.Entry, error) {
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
		result[i] = ProtoToSurveyEntry(pe)
	}
	return result, nil
}

func (g *SurveyAdapter) ClearAnalyzer(analyzer string) (int, error) {
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

func (g *SurveyAdapter) ReplaceEntriesForAnalyzer(analyzer string, newEntries []*survey.Entry) error {
	return errors.New("ReplaceEntriesForAnalyzer not supported in gRPC client mode")
}

func (g *SurveyAdapter) ReplaceEntriesForAnalyzerAndFile(analyzer, filePath string, newEntries []*survey.Entry) error {
	return errors.New("ReplaceEntriesForAnalyzerAndFile not supported in gRPC client mode")
}

func (g *SurveyAdapter) Stats(_ survey.SearchOptions) (*survey.Stats, error) {
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

func (g *SurveyAdapter) Clear() error {
	ctx, cancel := g.rpcCtx()
	defer cancel()

	_, err := g.client.Survey.Clear(ctx, &grpcapi.SurveyClearRequest{})
	return err
}

func (g *SurveyAdapter) Close() error {
	return nil
}
