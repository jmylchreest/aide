package main

import (
	"context"
	"os"
	"path/filepath"

	"github.com/jmylchreest/aide/aide/pkg/code"
	"github.com/jmylchreest/aide/aide/pkg/grpcapi"
)

// =============================================================================
// Code Operations
// =============================================================================

// CodeSearchResult represents a symbol search result.
type CodeSearchResult struct {
	Symbol *code.Symbol
	Score  float64
}

func (b *Backend) SearchCode(query string, kind, language, filePath string, limit int) ([]*CodeSearchResult, error) {
	ctx := context.Background()

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
		return protoToCodeSearchResults(resp.Symbols), nil
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
	ctx := context.Background()

	if b.useGRPC {
		resp, err := b.grpcClient.Code.Symbols(ctx, &grpcapi.CodeSymbolsRequest{
			FilePath: filePath,
		})
		if err != nil {
			return nil, err
		}
		return protoToSymbols(resp.Symbols), nil
	}

	codeStore, err := b.openCodeStore()
	if err != nil {
		return nil, err
	}
	defer codeStore.Close()

	symbols, err := codeStore.GetFileSymbols(filePath)
	if err != nil {
		parser := code.NewParser()
		return parser.ParseFile(filePath)
	}
	return symbols, nil
}

func (b *Backend) GetCodeStats() (*code.IndexStats, error) {
	ctx := context.Background()

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
func (b *Backend) IndexCodeWithProgress(paths []string, force bool, progress func(path string, symbols int)) (*CodeIndexResult, error) {
	ctx := context.Background()

	if b.useGRPC {
		resp, err := b.grpcClient.Code.Index(ctx, &grpcapi.CodeIndexRequest{
			Paths: paths,
			Force: force,
		})
		if err != nil {
			return nil, err
		}
		return &CodeIndexResult{
			FilesIndexed:   int(resp.FilesIndexed),
			SymbolsIndexed: int(resp.SymbolsIndexed),
			FilesSkipped:   int(resp.FilesSkipped),
		}, nil
	}

	codeStore, err := b.openCodeStore()
	if err != nil {
		return nil, err
	}
	defer codeStore.Close()

	parser := code.NewParser()
	if len(paths) == 0 {
		paths = []string{"."}
	}

	result := &CodeIndexResult{}

	for _, root := range paths {
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}

			if info.IsDir() {
				name := info.Name()
				if name == "node_modules" || name == ".git" || name == "vendor" ||
					name == "__pycache__" || name == ".venv" || name == "dist" ||
					name == "build" || name == ".aide" {
					return filepath.SkipDir
				}
				return nil
			}

			if !code.SupportedFile(path) {
				return nil
			}

			relPath := path
			if cwd, err := os.Getwd(); err == nil {
				if rel, err := filepath.Rel(cwd, path); err == nil {
					relPath = rel
				}
			}

			if !force {
				fileInfo, err := codeStore.GetFileInfo(relPath)
				if err == nil && fileInfo.ModTime.Equal(info.ModTime()) {
					result.FilesSkipped++
					return nil
				}
			}

			symbols, err := parser.ParseFile(path)
			if err != nil {
				return nil
			}

			refs, _ := parser.ParseFileReferences(path)

			codeStore.ClearFile(relPath)
			codeStore.ClearFileReferences(relPath)

			var symbolIDs []string
			for _, sym := range symbols {
				sym.FilePath = relPath
				if err := codeStore.AddSymbol(sym); err != nil {
					continue
				}
				symbolIDs = append(symbolIDs, sym.ID)
				result.SymbolsIndexed++
			}

			for _, ref := range refs {
				ref.FilePath = relPath
				codeStore.AddReference(ref)
			}

			codeStore.SetFileInfo(&code.FileInfo{
				Path:      relPath,
				ModTime:   info.ModTime(),
				SymbolIDs: symbolIDs,
			})

			result.FilesIndexed++

			if progress != nil {
				progress(relPath, len(symbols))
			}

			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	return result, nil
}

func (b *Backend) ClearCode() (int, int, error) {
	ctx := context.Background()

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
