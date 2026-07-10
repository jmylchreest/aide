package adapter

import (
	"context"
	"fmt"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/code"
	"github.com/jmylchreest/aide/aide/pkg/grpcapi"
	"github.com/jmylchreest/aide/aide/pkg/store"
)

// CodeAdapter implements the read side of store.CodeIndexStore over gRPC, so
// a client-mode MCP process can serve the code_* tools and survey_graph
// against the daemon's index. Write and bulk-scan operations (indexing,
// clearing, ListAll*) return errors: indexing runs only where the store
// lives, and the daemon-side analyzers cover the bulk scans.
type CodeAdapter struct {
	client *grpcapi.Client
}

// errCodeClientMode is returned for operations that must run on the daemon.
var errCodeClientMode = fmt.Errorf("not supported in gRPC client mode: operation runs on the daemon")

func NewCodeAdapter(client *grpcapi.Client) *CodeAdapter {
	return &CodeAdapter{client: client}
}

// Compile-time check: the adapter must satisfy the full interface.
var _ store.CodeIndexStore = (*CodeAdapter)(nil)

func (a *CodeAdapter) ctx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), RPCTimeout)
}

func (a *CodeAdapter) SearchSymbols(query string, opts code.SearchOptions) ([]*store.CodeSearchResult, error) {
	ctx, cancel := a.ctx()
	defer cancel()
	resp, err := a.client.Code.Search(ctx, &grpcapi.CodeSearchRequest{
		Query:    query,
		Kind:     opts.Kind,
		Language: opts.Language,
		FilePath: opts.FilePath,
		Limit:    int32(opts.Limit),
	})
	if err != nil {
		return nil, err
	}
	results := make([]*store.CodeSearchResult, 0, len(resp.Symbols))
	for _, s := range resp.Symbols {
		results = append(results, &store.CodeSearchResult{Symbol: ProtoToSymbol(s)})
	}
	return results, nil
}

func (a *CodeAdapter) GetFileSymbols(filePath string) ([]*code.Symbol, error) {
	ctx, cancel := a.ctx()
	defer cancel()
	resp, err := a.client.Code.Symbols(ctx, &grpcapi.CodeSymbolsRequest{FilePath: filePath})
	if err != nil {
		return nil, err
	}
	return ProtoToSymbols(resp.Symbols), nil
}

func (a *CodeAdapter) GetContainingSymbol(filePath string, line int) (*code.Symbol, error) {
	ctx, cancel := a.ctx()
	defer cancel()
	resp, err := a.client.Code.GetContainingSymbol(ctx, &grpcapi.CodeGetContainingSymbolRequest{
		FilePath: filePath,
		Line:     int32(line),
	})
	if err != nil {
		return nil, err
	}
	if !resp.Found || resp.Symbol == nil {
		return nil, nil
	}
	return ProtoToSymbol(resp.Symbol), nil
}

func (a *CodeAdapter) GetFileInfo(path string) (*code.FileInfo, error) {
	ctx, cancel := a.ctx()
	defer cancel()
	resp, err := a.client.Code.GetFileInfo(ctx, &grpcapi.CodeGetFileInfoRequest{Path: path})
	if err != nil {
		return nil, err
	}
	if !resp.Found {
		return nil, fmt.Errorf("file not indexed: %s", path)
	}
	fi := &code.FileInfo{
		Path:      path,
		SymbolIDs: resp.SymbolIds,
		Tokens:    int(resp.Tokens),
		SizeBytes: resp.SizeBytes,
	}
	if resp.ModTime != nil {
		fi.ModTime = resp.ModTime.AsTime().Local()
	}
	return fi, nil
}

func (a *CodeAdapter) SearchReferences(opts code.ReferenceSearchOptions) ([]*code.Reference, error) {
	ctx, cancel := a.ctx()
	defer cancel()
	resp, err := a.client.Code.SearchReferences(ctx, &grpcapi.CodeSearchReferencesRequest{
		SymbolName: opts.SymbolName,
		Kind:       opts.Kind,
		FilePath:   opts.FilePath,
		Limit:      int32(opts.Limit),
	})
	if err != nil {
		return nil, err
	}
	return ProtoToReferences(resp.References), nil
}

func (a *CodeAdapter) GetFileReferences(filePath string) ([]*code.Reference, error) {
	ctx, cancel := a.ctx()
	defer cancel()
	resp, err := a.client.Code.GetFileReferences(ctx, &grpcapi.CodeGetFileReferencesRequest{FilePath: filePath})
	if err != nil {
		return nil, err
	}
	return ProtoToReferences(resp.References), nil
}

func (a *CodeAdapter) TopReferencedSymbols(limit int, kind string) ([]*code.SymbolRefCount, error) {
	ctx, cancel := a.ctx()
	defer cancel()
	resp, err := a.client.Code.TopReferences(ctx, &grpcapi.CodeTopReferencesRequest{
		Limit: int32(limit),
		Kind:  kind,
	})
	if err != nil {
		return nil, err
	}
	out := make([]*code.SymbolRefCount, 0, len(resp.Symbols))
	for _, s := range resp.Symbols {
		out = append(out, &code.SymbolRefCount{
			Symbol: s.Symbol,
			Count:  int(s.Count),
			Kind:   s.Kind,
			File:   s.File,
		})
	}
	return out, nil
}

func (a *CodeAdapter) Stats() (*code.IndexStats, error) {
	ctx, cancel := a.ctx()
	defer cancel()
	resp, err := a.client.Code.Stats(ctx, &grpcapi.CodeStatsRequest{})
	if err != nil {
		return nil, err
	}
	return &code.IndexStats{
		Files:      int(resp.Files),
		Symbols:    int(resp.Symbols),
		References: int(resp.References),
	}, nil
}

func (a *CodeAdapter) GetSymbol(string) (*code.Symbol, error)     { return nil, errCodeClientMode }
func (a *CodeAdapter) DeleteSymbol(string) error                  { return errCodeClientMode }
func (a *CodeAdapter) ListAllFileInfo() ([]*code.FileInfo, error) { return nil, errCodeClientMode }
func (a *CodeAdapter) ClearFile(string) error                     { return errCodeClientMode }
func (a *CodeAdapter) IndexFileBatch(string, []*code.Symbol, []*code.Reference, time.Time, int64) error {
	return errCodeClientMode
}
func (a *CodeAdapter) ClearFileReferences(string) error           { return errCodeClientMode }
func (a *CodeAdapter) ListAllSymbols(int) ([]*code.Symbol, error) { return nil, errCodeClientMode }
func (a *CodeAdapter) ListAllReferences(int) ([]*code.Reference, error) {
	return nil, errCodeClientMode
}
func (a *CodeAdapter) Clear() error { return errCodeClientMode }
func (a *CodeAdapter) Close() error { return nil }
