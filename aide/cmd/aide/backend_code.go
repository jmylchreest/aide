package main

import (
	"context"
	"io"
	"path/filepath"

	"github.com/jmylchreest/aide/aide/pkg/checkout"
	"github.com/jmylchreest/aide/aide/pkg/code"
	"github.com/jmylchreest/aide/aide/pkg/codeindex"
	"github.com/jmylchreest/aide/aide/pkg/grpcapi"
	"github.com/jmylchreest/aide/aide/pkg/grpcapi/adapter"
	"github.com/jmylchreest/aide/aide/pkg/store"
	"github.com/jmylchreest/aide/aide/pkg/survey"
)

// =============================================================================
// Code Operations
// =============================================================================

// CodeSearchResult represents a symbol search result.
type CodeSearchResult struct {
	Symbol *code.Symbol
	Score  float64
}

func protoToLocalCodeSearchResults(ps []*grpcapi.Symbol) []*CodeSearchResult {
	result := make([]*CodeSearchResult, len(ps))
	for i, p := range ps {
		result[i] = &CodeSearchResult{
			Symbol: adapter.ProtoToSymbol(p),
			Score:  0,
		}
	}
	return result
}

func (b *Backend) SearchCode(query string, kind, language, filePath string, limit int) ([]*CodeSearchResult, error) {
	ctx, cancel := b.rpcCtx()
	defer cancel()

	if b.useGRPC {
		resp, err := b.grpcClient.Code.Search(ctx, &grpcapi.CodeSearchRequest{
			Query:    query,
			Kind:     kind,
			Language: language,
			FilePath: filePath,
			Limit:    int32(limit),
		})
		if err != nil {
			return nil, err
		}
		return protoToLocalCodeSearchResults(resp.Symbols), nil
	}

	codeStore, err := b.openCodeStore()
	if err != nil {
		return nil, err
	}
	defer codeStore.Close()

	opts := code.SearchOptions{
		Kind:     kind,
		Language: language,
		FilePath: filePath,
		Limit:    limit,
	}
	results, err := codeStore.SearchSymbols(query, opts)
	if err != nil {
		return nil, err
	}

	codeResults := make([]*CodeSearchResult, 0, len(results))
	for _, r := range results {
		codeResults = append(codeResults, &CodeSearchResult{
			Symbol: r.Symbol,
			Score:  r.Score,
		})
	}
	return codeResults, nil
}

func (b *Backend) GetFileSymbols(filePath string) ([]*code.Symbol, error) {
	ctx, cancel := b.rpcCtx()
	defer cancel()

	if b.useGRPC {
		resp, err := b.grpcClient.Code.Symbols(ctx, &grpcapi.CodeSymbolsRequest{
			FilePath: filePath,
		})
		if err != nil {
			return nil, err
		}
		return adapter.ProtoToSymbols(resp.Symbols), nil
	}

	codeStore, err := b.openCodeStore()
	if err != nil {
		return nil, err
	}
	defer codeStore.Close()

	abs, rel, err := checkout.SourcePath(store.CheckoutRoot(b.dbPath), filePath)
	if err != nil {
		return nil, err
	}
	symbols, err := codeStore.GetFileSymbols(rel)
	if err != nil {
		parser := code.NewParser(newGrammarLoader(b.dbPath, nil))
		defer parser.Close()
		return parser.ParseFile(abs)
	}
	return symbols, nil
}

func (b *Backend) GetCodeStats() (*code.IndexStats, error) {
	ctx, cancel := b.rpcCtx()
	defer cancel()

	if b.useGRPC {
		resp, err := b.grpcClient.Code.Stats(ctx, &grpcapi.CodeStatsRequest{})
		if err != nil {
			return nil, err
		}
		return &code.IndexStats{
			Files:      int(resp.Files),
			Symbols:    int(resp.Symbols),
			References: int(resp.References),
		}, nil
	}

	codeStore, err := b.openCodeStore()
	if err != nil {
		return nil, err
	}
	defer codeStore.Close()

	return codeStore.Stats()
}

// SearchReferences finds all references/call sites for a symbol.
func (b *Backend) SearchReferences(symbolName, kind, filePath string, limit int) ([]*code.Reference, error) {
	ctx, cancel := b.rpcCtx()
	defer cancel()

	if b.useGRPC {
		resp, err := b.grpcClient.Code.SearchReferences(ctx, &grpcapi.CodeSearchReferencesRequest{
			SymbolName: symbolName,
			Kind:       kind,
			FilePath:   filePath,
			Limit:      int32(limit),
		})
		if err != nil {
			return nil, err
		}
		return adapter.ProtoToReferences(resp.References), nil
	}

	codeStore, err := b.openCodeStore()
	if err != nil {
		return nil, err
	}
	defer codeStore.Close()

	return codeStore.SearchReferences(code.ReferenceSearchOptions{
		SymbolName: symbolName,
		Kind:       kind,
		FilePath:   filePath,
		Limit:      limit,
	})
}

type CodeIndexResult struct {
	FilesIndexed   int
	SymbolsIndexed int
	FilesSkipped   int
}

func (b *Backend) IndexCode(paths []string, force bool) (*CodeIndexResult, error) {
	return b.IndexCodeWithProgress(paths, force, nil)
}

// IndexCodeWithProgress indexes code with an optional progress callback.
//
// In daemon (gRPC) mode the index runs server-side and emits one progress
// event per file plus a final summary. The whole-RPC deadline is unbounded
// (server activity keeps the stream alive); cancel via the context returned
// by signal handling if needed. The legacy 10s adapter.RPCTimeout previously
// applied here was the cause of "DeadlineExceeded" failures on large repos.
func (b *Backend) IndexCodeWithProgress(paths []string, force bool, progress func(path string, symbols int)) (*CodeIndexResult, error) {
	if b.useGRPC {
		stream, err := b.grpcClient.Code.Index(context.Background(), &grpcapi.CodeIndexRequest{
			Paths: paths,
			Force: force,
		})
		if err != nil {
			return nil, err
		}
		var result CodeIndexResult
		for {
			ev, err := stream.Recv()
			if err == io.EOF {
				return &result, nil
			}
			if err != nil {
				return nil, err
			}
			switch e := ev.Event.(type) {
			case *grpcapi.CodeIndexEvent_Progress:
				if progress != nil && !e.Progress.Skipped {
					progress(e.Progress.Path, int(e.Progress.Symbols))
				}
			case *grpcapi.CodeIndexEvent_Summary:
				result.FilesIndexed = int(e.Summary.FilesIndexed)
				result.SymbolsIndexed = int(e.Summary.SymbolsIndexed)
				result.FilesSkipped = int(e.Summary.FilesSkipped)
			}
		}
	}

	codeStore, err := b.openCodeStore()
	if err != nil {
		return nil, err
	}
	defer codeStore.Close()

	parser := code.NewParser(newGrammarLoader(b.dbPath, nil))
	defer parser.Close()
	seed, closeSeed := store.CheckoutSeed(b.dbPath, store.CheckoutRoot(b.dbPath))
	defer closeSeed()
	result, err := codeindex.Run(context.Background(), codeStore, parser, store.CheckoutRoot(b.dbPath), paths, force, seed, func(p codeindex.Progress) error {
		if progress != nil && !p.Skipped {
			progress(p.Path, p.Symbols)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &CodeIndexResult{FilesIndexed: result.Indexed, SymbolsIndexed: result.Symbols, FilesSkipped: result.Skipped}, nil
}

func (b *Backend) ClearCode() (int, int, error) {
	ctx, cancel := b.rpcCtx()
	defer cancel()

	if b.useGRPC {
		resp, err := b.grpcClient.Code.Clear(ctx, &grpcapi.CodeClearRequest{})
		if err != nil {
			return 0, 0, err
		}
		return int(resp.SymbolsCleared), int(resp.FilesCleared), nil
	}

	codeStore, err := b.openCodeStore()
	if err != nil {
		return 0, 0, err
	}
	defer codeStore.Close()

	stats, _ := codeStore.Stats()
	if err := codeStore.Clear(); err != nil {
		return 0, 0, err
	}

	if stats != nil {
		return stats.Symbols, stats.Files, nil
	}
	return 0, 0, nil
}

// ReadCheckResult holds the result of a file read-check operation.
type ReadCheckResult = code.ReadCheckResult

// ReadCheck checks whether a file is indexed and whether its index is fresh
// by mtime. A matching timestamp does not verify identical contents.
func (b *Backend) ReadCheck(filePath string) (*ReadCheckResult, error) {
	ctx, cancel := b.rpcCtx()
	defer cancel()

	if b.useGRPC {
		resp, err := b.grpcClient.Code.ReadCheck(ctx, &grpcapi.CodeReadCheckRequest{
			FilePath: filePath,
		})
		if err != nil {
			return &ReadCheckResult{}, nil
		}
		return &ReadCheckResult{
			Indexed:          resp.Indexed,
			Fresh:            resp.Fresh,
			Symbols:          int(resp.Symbols),
			OutlineAvailable: resp.OutlineAvailable,
			EstimatedTokens:  int(resp.EstimatedTokens),
			TextEstimate:     grpcapi.ProtoToReadCheckTextEstimate(resp.TextEstimate),
		}, nil
	}

	// Direct store access fallback
	codeStore, err := b.openCodeStore()
	if err != nil {
		return &ReadCheckResult{}, nil
	}
	defer codeStore.Close()

	root := store.CheckoutRoot(b.dbPath)

	// Resolve paths
	absPath := filePath
	if !filepath.IsAbs(filePath) {
		absPath = filepath.Join(root, filePath)
	}
	relPath := filePath
	if filepath.IsAbs(filePath) {
		if rel, err := filepath.Rel(root, filePath); err == nil {
			relPath = rel
		}
	}

	fileInfo, err := codeStore.GetFileInfo(relPath)
	if err != nil {
		return &ReadCheckResult{}, nil
	}

	return code.CheckIndexedFile(absPath, fileInfo), nil
}

// CodeSearcher returns a survey.CodeSearcher backed by gRPC (when the MCP
// server is running) or by direct code store access. Returns nil and an error
// if the code store is not available. The caller should call the returned
// cleanup function when done.
func (b *Backend) CodeSearcher() (survey.CodeSearcher, func(), error) {
	if b.useGRPC {
		return &grpcCodeSearcher{client: b.grpcClient.Code}, func() {}, nil
	}
	codeStore, err := b.openCodeStore()
	if err != nil {
		return nil, nil, err
	}
	return &codeSearcherAdapter{store: codeStore}, func() { codeStore.Close() }, nil
}

// grpcCodeSearcher implements survey.CodeSearcher via gRPC.
type grpcCodeSearcher struct {
	client grpcapi.CodeServiceClient
}

func (g *grpcCodeSearcher) FindSymbols(query string, kind string, limit int) ([]survey.SymbolHit, error) {
	ctx, cancel := context.WithTimeout(context.Background(), adapter.RPCTimeout)
	defer cancel()

	resp, err := g.client.Search(ctx, &grpcapi.CodeSearchRequest{
		Query: query,
		Kind:  kind,
		Limit: int32(limit),
	})
	if err != nil {
		return nil, err
	}

	hits := make([]survey.SymbolHit, 0, len(resp.Symbols))
	for _, s := range resp.Symbols {
		hits = append(hits, survey.SymbolHit{
			Name:     s.Name,
			Kind:     s.Kind,
			FilePath: s.FilePath,
			Line:     int(s.StartLine),
			EndLine:  int(s.EndLine),
			Language: s.Language,
		})
	}
	return hits, nil
}

func (g *grpcCodeSearcher) FindReferences(symbolName string, kind string, limit int) ([]survey.ReferenceHit, error) {
	ctx, cancel := context.WithTimeout(context.Background(), adapter.RPCTimeout)
	defer cancel()

	resp, err := g.client.SearchReferences(ctx, &grpcapi.CodeSearchReferencesRequest{
		SymbolName: symbolName,
		Kind:       kind,
		Limit:      int32(limit),
	})
	if err != nil {
		return nil, err
	}

	hits := make([]survey.ReferenceHit, 0, len(resp.References))
	for _, r := range resp.References {
		hits = append(hits, survey.ReferenceHit{
			Symbol:   r.SymbolName,
			Kind:     r.Kind,
			FilePath: r.FilePath,
			Line:     int(r.Line),
		})
	}
	return hits, nil
}

// CodeGrapher returns a survey.CodeGrapher backed by gRPC (when the MCP
// server is running) or by direct code store access. The caller should call
// the returned cleanup function when done.
func (b *Backend) CodeGrapher() (survey.CodeGrapher, func(), error) {
	if b.useGRPC {
		return &grpcCodeGrapher{grpcCodeSearcher: grpcCodeSearcher{client: b.grpcClient.Code}}, func() {}, nil
	}
	codeStore, err := b.openCodeStore()
	if err != nil {
		return nil, nil, err
	}
	return &codeGrapherAdapter{codeSearcherAdapter: codeSearcherAdapter{store: codeStore}}, func() { codeStore.Close() }, nil
}

// grpcCodeGrapher implements survey.CodeGrapher via gRPC.
// It embeds grpcCodeSearcher for FindSymbols/FindReferences, and adds
// GetFileReferences and GetContainingSymbol.
type grpcCodeGrapher struct {
	grpcCodeSearcher
}

func (g *grpcCodeGrapher) GetFileReferences(filePath string) ([]survey.ReferenceHit, error) {
	ctx, cancel := context.WithTimeout(context.Background(), adapter.RPCTimeout)
	defer cancel()

	resp, err := g.client.GetFileReferences(ctx, &grpcapi.CodeGetFileReferencesRequest{
		FilePath: filePath,
	})
	if err != nil {
		return nil, err
	}

	hits := make([]survey.ReferenceHit, 0, len(resp.References))
	for _, r := range resp.References {
		hits = append(hits, survey.ReferenceHit{
			Symbol:   r.SymbolName,
			Kind:     r.Kind,
			FilePath: r.FilePath,
			Line:     int(r.Line),
		})
	}
	return hits, nil
}

func (g *grpcCodeGrapher) GetContainingSymbol(filePath string, line int) (*survey.SymbolHit, error) {
	ctx, cancel := context.WithTimeout(context.Background(), adapter.RPCTimeout)
	defer cancel()

	resp, err := g.client.GetContainingSymbol(ctx, &grpcapi.CodeGetContainingSymbolRequest{
		FilePath: filePath,
		Line:     int32(line),
	})
	if err != nil {
		return nil, err
	}

	if !resp.Found || resp.Symbol == nil {
		return nil, nil
	}

	return &survey.SymbolHit{
		Name:     resp.Symbol.Name,
		Kind:     resp.Symbol.Kind,
		FilePath: resp.Symbol.FilePath,
		Line:     int(resp.Symbol.StartLine),
		EndLine:  int(resp.Symbol.EndLine),
		Language: resp.Symbol.Language,
	}, nil
}
