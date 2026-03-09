package main

import (
	"context"

	"github.com/jmylchreest/aide/aide/pkg/grpcapi"
	"github.com/jmylchreest/aide/aide/pkg/store"
	"github.com/jmylchreest/aide/aide/pkg/survey"
)

// =============================================================================
// Survey Backend Operations
// =============================================================================

// openSurveyStore opens the survey store for direct access.
func (b *Backend) openSurveyStore() (store.SurveyStore, error) {
	surveyDir := getSurveyStorePath(b.dbPath)
	return store.NewSurveyStore(surveyDir)
}

func (b *Backend) SearchSurvey(query string, opts survey.SearchOptions) ([]*survey.SearchResult, error) {
	ctx := context.Background()

	if b.useGRPC {
		resp, err := b.grpcClient.Survey.Search(ctx, &grpcapi.SurveySearchRequest{
			Query:    query,
			Analyzer: opts.Analyzer,
			Kind:     opts.Kind,
			FilePath: opts.FilePath,
			Limit:    int32(opts.Limit),
		})
		if err != nil {
			return nil, err
		}
		results := make([]*survey.SearchResult, 0, len(resp.Entries))
		for _, pe := range resp.Entries {
			results = append(results, &survey.SearchResult{
				Entry: protoToSurveyEntry(pe),
			})
		}
		return results, nil
	}

	ss, err := b.openSurveyStore()
	if err != nil {
		return nil, err
	}
	defer ss.Close()

	return ss.SearchEntries(query, opts)
}

func (b *Backend) ListSurvey(opts survey.SearchOptions) ([]*survey.Entry, error) {
	ctx := context.Background()

	if b.useGRPC {
		resp, err := b.grpcClient.Survey.List(ctx, &grpcapi.SurveyListRequest{
			Analyzer: opts.Analyzer,
			Kind:     opts.Kind,
			FilePath: opts.FilePath,
			Limit:    int32(opts.Limit),
		})
		if err != nil {
			return nil, err
		}
		result := make([]*survey.Entry, 0, len(resp.Entries))
		for _, pe := range resp.Entries {
			result = append(result, protoToSurveyEntry(pe))
		}
		return result, nil
	}

	ss, err := b.openSurveyStore()
	if err != nil {
		return nil, err
	}
	defer ss.Close()

	return ss.ListEntries(opts)
}

func (b *Backend) GetSurveyStats(opts survey.SearchOptions) (*survey.Stats, error) {
	ctx := context.Background()

	if b.useGRPC {
		resp, err := b.grpcClient.Survey.Stats(ctx, &grpcapi.SurveyStatsRequest{})
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

	ss, err := b.openSurveyStore()
	if err != nil {
		return nil, err
	}
	defer ss.Close()

	return ss.Stats(opts)
}

func (b *Backend) ClearSurvey() error {
	ctx := context.Background()

	if b.useGRPC {
		_, err := b.grpcClient.Survey.Clear(ctx, &grpcapi.SurveyClearRequest{})
		return err
	}

	ss, err := b.openSurveyStore()
	if err != nil {
		return err
	}
	defer ss.Close()

	return ss.Clear()
}

func (b *Backend) ClearSurveyAnalyzer(analyzer string) (int, error) {
	ctx := context.Background()

	if b.useGRPC {
		resp, err := b.grpcClient.Survey.ClearAnalyzer(ctx, &grpcapi.SurveyClearAnalyzerRequest{
			Analyzer: analyzer,
		})
		if err != nil {
			return 0, err
		}
		return int(resp.Count), nil
	}

	ss, err := b.openSurveyStore()
	if err != nil {
		return 0, err
	}
	defer ss.Close()

	return ss.ClearAnalyzer(analyzer)
}

func (b *Backend) ReplaceSurveyForAnalyzer(analyzer string, entries []*survey.Entry) error {
	if b.useGRPC {
		return b.grpcSurveyReplaceForAnalyzer(analyzer, entries)
	}

	ss, err := b.openSurveyStore()
	if err != nil {
		return err
	}
	defer ss.Close()

	return ss.ReplaceEntriesForAnalyzer(analyzer, entries)
}

// grpcSurveyReplaceForAnalyzer replaces all survey entries for an analyzer over gRPC.
// NOTE: This is non-atomic — it clears then re-adds one by one. If the process
// crashes mid-way, the analyzer's entries will be partially populated. This is
// acceptable for the CLI use-case where the server holds the authoritative store.
func (b *Backend) grpcSurveyReplaceForAnalyzer(analyzer string, entries []*survey.Entry) error {
	ctx := context.Background()

	if _, err := b.grpcClient.Survey.ClearAnalyzer(ctx, &grpcapi.SurveyClearAnalyzerRequest{
		Analyzer: analyzer,
	}); err != nil {
		return err
	}

	for _, e := range entries {
		_, err := b.grpcClient.Survey.Add(ctx, &grpcapi.SurveyAddRequest{
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
	}
	return nil
}
