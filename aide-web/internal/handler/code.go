package handler

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jmylchreest/aide/aide-web/internal/instance"
	"github.com/jmylchreest/aide/aide/pkg/grpcapi"
)

// CodeStatsOutput is the response body for APIListCode.
type CodeStatsOutput struct {
	Body struct {
		Available bool `json:"available"`
	}
}

// CodeSearchJSON returns JSON symbol search results.
func (h *Handler) CodeSearchJSON(w http.ResponseWriter, r *http.Request) {
	inst := h.getInstance(r)
	if inst == nil {
		http.Error(w, `{"symbols":[]}`, http.StatusNotFound)
		return
	}
	query := r.URL.Query().Get("q")
	if query == "" {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"symbols":[]}`)
		return
	}
	client := inst.Client()
	if client == nil {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"symbols":[]}`)
		return
	}
	limit := int32(100)
	if query == "*" {
		limit = 10000 // return all symbols for browsing
	}
	resp, err := client.Code.Search(r.Context(), &grpcapi.CodeSearchRequest{Query: query, Limit: limit})
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"symbols":[]}`)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, `{"symbols":[`)
	for i, s := range resp.Symbols {
		if i > 0 {
			fmt.Fprint(w, ",")
		}
		fmt.Fprintf(w, `{"name":%q,"kind":%q,"language":%q,"file":%q,"line":%d,"signature":%q}`,
			s.Name, s.Kind, s.Language, s.FilePath, s.StartLine, s.Signature)
	}
	fmt.Fprint(w, `]}`)
}

// TopReferencedSymbolItem is one row of APICodeTopReferences.
type TopReferencedSymbolItem struct {
	Symbol string `json:"symbol"`
	Count  int    `json:"count"`
	Kind   string `json:"kind,omitempty"`
	File   string `json:"file,omitempty"`
}

// TopReferencesOutput is the response body for APICodeTopReferences.
type TopReferencesOutput struct {
	Body struct {
		Symbols []TopReferencedSymbolItem `json:"symbols"`
	}
}

// APICodeTopReferences ranks symbols by call-site count — the survey graph
// view offers these as starting points.
func (h *Handler) APICodeTopReferences(ctx context.Context, input *struct {
	Project string `path:"project"`
	Limit   int    `query:"limit" minimum:"1" maximum:"50" default:"10"`
	Kind    string `query:"kind"`
}) (*TopReferencesOutput, error) {
	inst := h.findInstance(input.Project)
	if inst == nil {
		return nil, huma.Error404NotFound("instance not found")
	}
	cs := inst.CodeStore()
	if cs == nil {
		return nil, huma.Error503ServiceUnavailable("instance not connected")
	}
	rows, err := cs.TopReferencedSymbols(input.Limit, input.Kind)
	if err != nil {
		return nil, huma.Error500InternalServerError(err.Error())
	}
	out := &TopReferencesOutput{}
	for _, r := range rows {
		out.Body.Symbols = append(out.Body.Symbols, TopReferencedSymbolItem{
			Symbol: r.Symbol,
			Count:  r.Count,
			Kind:   r.Kind,
			File:   r.File,
		})
	}
	return out, nil
}

// APIListCode returns code index availability for an instance.
func (h *Handler) APIListCode(ctx context.Context, input *struct {
	Project string `path:"project"`
}) (*CodeStatsOutput, error) {
	inst := h.findInstance(input.Project)
	if inst == nil {
		return nil, huma.Error404NotFound("instance not found")
	}
	out := &CodeStatsOutput{}
	out.Body.Available = inst.Status() == instance.StatusConnected
	return out, nil
}

// CodeIndexOutput is the response body for APIRunCodeIndex.
type CodeIndexOutput struct {
	Body struct {
		FilesIndexed   int32 `json:"files_indexed"`
		SymbolsIndexed int32 `json:"symbols_indexed"`
		FilesSkipped   int32 `json:"files_skipped"`
	}
}

// APIRunCodeIndex triggers a code index on the instance and returns stats.
func (h *Handler) APIRunCodeIndex(ctx context.Context, input *struct {
	Project string `path:"project"`
}) (*CodeIndexOutput, error) {
	inst := h.findInstance(input.Project)
	if inst == nil {
		return nil, huma.Error404NotFound("instance not found")
	}
	client := inst.Client()
	if client == nil {
		return nil, huma.Error503ServiceUnavailable("instance not connected")
	}
	stream, err := client.Code.Index(ctx, &grpcapi.CodeIndexRequest{})
	if err != nil {
		return nil, huma.Error500InternalServerError(fmt.Sprintf("index failed: %v", err))
	}
	out := &CodeIndexOutput{}
	for {
		ev, err := stream.Recv()
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return nil, huma.Error500InternalServerError(fmt.Sprintf("index failed: %v", err))
		}
		if s, ok := ev.Event.(*grpcapi.CodeIndexEvent_Summary); ok {
			out.Body.FilesIndexed = s.Summary.FilesIndexed
			out.Body.SymbolsIndexed = s.Summary.SymbolsIndexed
			out.Body.FilesSkipped = s.Summary.FilesSkipped
		}
	}
}

// ReadFileOutput is the response body for APIReadFile.
type ReadFileOutput struct {
	Body struct {
		Path     string `json:"path"`
		Content  string `json:"content"`
		Language string `json:"language"`
		Lines    int    `json:"lines"`
	}
}

// APIReadFile reads a file from the instance's project root (read-only).
func (h *Handler) APIReadFile(ctx context.Context, input *struct {
	Project string `path:"project"`
	Path    string `query:"path" required:"true"`
}) (*ReadFileOutput, error) {
	inst := h.findInstance(input.Project)
	if inst == nil {
		return nil, huma.Error404NotFound("instance not found")
	}

	// Security: resolve and ensure the path stays within project root
	root := inst.ProjectRoot()
	abs, err := validatePath(root, input.Path)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}

	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, huma.Error404NotFound("file not found")
	}

	// Cap at 500KB to avoid blowing up the response
	if len(data) > 500*1024 {
		return nil, huma.Error400BadRequest("file too large (>500KB)")
	}

	content := string(data)
	lineCount := strings.Count(content, "\n")
	if len(content) > 0 && content[len(content)-1] != '\n' {
		lineCount++
	}

	clean := filepath.Clean(input.Path)
	out := &ReadFileOutput{}
	out.Body.Path = clean
	out.Body.Content = content
	out.Body.Language = langFromExt(filepath.Ext(clean))
	out.Body.Lines = lineCount
	return out, nil
}

// validatePath checks that reqPath stays within root and returns the absolute path.
func validatePath(root, reqPath string) (string, error) {
	clean := filepath.Clean(reqPath)
	if filepath.IsAbs(clean) || strings.HasPrefix(clean, "..") {
		return "", fmt.Errorf("invalid path")
	}
	abs := filepath.Join(root, clean)
	if !strings.HasPrefix(abs, root) {
		return "", fmt.Errorf("path traversal denied")
	}
	return abs, nil
}

func langFromExt(ext string) string {
	switch strings.ToLower(ext) {
	case ".go":
		return "go"
	case ".ts", ".tsx":
		return "typescript"
	case ".js", ".jsx", ".mjs", ".cjs":
		return "javascript"
	case ".rs":
		return "rust"
	case ".py":
		return "python"
	case ".rb":
		return "ruby"
	case ".java":
		return "java"
	case ".c", ".h":
		return "c"
	case ".cpp", ".cc", ".cxx", ".hpp":
		return "cpp"
	case ".css":
		return "css"
	case ".html", ".htm":
		return "html"
	case ".json":
		return "json"
	case ".yaml", ".yml":
		return "yaml"
	case ".toml":
		return "toml"
	case ".md":
		return "markdown"
	case ".sh", ".bash", ".zsh":
		return "shell"
	case ".sql":
		return "sql"
	case ".proto":
		return "protobuf"
	default:
		return "text"
	}
}
