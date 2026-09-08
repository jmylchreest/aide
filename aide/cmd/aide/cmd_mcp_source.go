package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jmylchreest/aide/aide/pkg/code"
	"github.com/jmylchreest/aide/aide/pkg/observe"
	"github.com/jmylchreest/aide/aide/pkg/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type sourceSnapshot struct {
	path    string
	content []byte
	symbols []*code.Symbol
}

func (s *MCPServer) sourcePath(file string) (string, string) {
	root := store.ProjectRootFromDB(s.dbPath)
	abs := file
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(root, file)
	}
	abs = filepath.Clean(abs)
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		rel = abs
	}
	return abs, rel
}

func (s *MCPServer) readSourceSnapshot(file string) (*sourceSnapshot, error) {
	abs, rel := s.sourcePath(file)
	content, err := os.ReadFile(abs)
	if err != nil {
		return nil, err
	}
	parser := code.NewParser(s.grammarLoader)
	defer parser.Close()
	symbols, err := parser.ParseContent(content, code.DetectLanguage(abs, content), rel)
	if err != nil {
		return nil, err
	}
	return &sourceSnapshot{path: rel, content: content, symbols: symbols}, nil
}

// These are conditional full-file reference bytes, not observed avoided reads.
// Keep the versioned evidence out of the historical TokensSaved accumulator.
// Rendered output bytes are measured separately by the tool middleware.
type sourceReference struct {
	File   string `json:"file"`
	SHA256 string `json:"sha256"`
	Bytes  int    `json:"bytes"`
}

func recordSourceReferences(span *observe.Span, snapshots map[string]*sourceSnapshot) []sourceReference {
	if len(snapshots) == 0 {
		return nil
	}
	refs := make([]sourceReference, 0, len(snapshots))
	for _, snapshot := range snapshots {
		refs = append(refs, sourceReference{snapshot.path, fmt.Sprintf("%x", sha256.Sum256(snapshot.content)), len(snapshot.content)})
	}
	sort.Slice(refs, func(i, j int) bool { return refs[i].File < refs[j].File })
	encoded, _ := json.Marshal(refs)
	span.Attr("reference_kind", "full_file").Attr("source_references", string(encoded))
	return refs
}

// The host may preserve protocol metadata or drop it. Only a surviving receipt
// with a matching text digest can attach these references to a host call/window.
// No session identity is inferred here and no receipt is added to model text.
func sourceResult(span *observe.Span, tool string, snapshots map[string]*sourceSnapshot, text string) *mcp.CallToolResult {
	result := textResult(text)
	refs := recordSourceReferences(span, snapshots)
	if len(refs) == 0 {
		return result
	}
	id := rand.Text()
	span.Attr("retrieval_id", id)
	result.Meta = mcp.Meta{"aide/retrieval": map[string]any{
		"version": 1, "id": id, "tool": tool,
		"text_sha256": fmt.Sprintf("%x", sha256.Sum256([]byte(text))),
		"references":  refs,
	}}
	return result
}

// Candidate locations come from the index; definitions and ranges come only
// from current source. An explicit file bypasses index availability entirely.
func (s *MCPServer) readOneSymbol(cs store.CodeIndexStore, name string, input CodeReadSymbolInput, snapshots map[string]*sourceSnapshot) (*code.Symbol, *sourceSnapshot, string) {
	fail := func(message string) (*code.Symbol, *sourceSnapshot, string) {
		return nil, nil, fmt.Sprintf("## `%s` — %s\n\n", name, message)
	}
	files := make(map[string]bool)
	if input.File != "" {
		_, file := s.sourcePath(input.File)
		files[file] = true
	} else {
		const candidateLimit = 100
		results, err := cs.SearchSymbols(name, code.SearchOptions{Kind: input.Kind, Limit: candidateLimit})
		if err != nil {
			return fail(fmt.Sprintf("search error: %v", err))
		}
		if len(results) >= candidateLimit {
			return fail("too many search candidates; set file to select the definition")
		}
		for _, r := range results {
			if r.Symbol != nil && r.Symbol.Name == name {
				_, file := s.sourcePath(r.Symbol.FilePath)
				files[file] = true
			}
		}
	}
	paths := make([]string, 0, len(files))
	for file := range files {
		paths = append(paths, file)
	}
	sort.Strings(paths)
	var matches []*code.Symbol
	for _, file := range paths {
		snapshot := snapshots[file]
		if snapshot == nil {
			var err error
			snapshot, err = s.readSourceSnapshot(file)
			if err != nil {
				return fail(fmt.Sprintf("cannot verify candidate in %s: %v", file, err))
			}
			snapshots[file] = snapshot
		}
		for _, sym := range snapshot.symbols {
			if sym.Name == name && (input.Kind == "" || sym.Kind == input.Kind) && (input.StartLine == 0 || sym.StartLine == input.StartLine) {
				matches = append(matches, sym)
			}
		}
	}
	if len(matches) == 0 {
		return fail("not found in current source; check file/name or refresh the code index")
	}
	if len(matches) != 1 {
		var choices strings.Builder
		choices.WriteString("ambiguous; set file and, if needed, start_line:\n")
		for _, sym := range matches {
			fmt.Fprintf(&choices, "- %s:%d [%s] %s\n", sym.FilePath, sym.StartLine, sym.Kind, sym.Signature)
		}
		return fail(choices.String())
	}
	match := matches[0]
	snapshot := snapshots[match.FilePath]
	lines := strings.Split(string(snapshot.content), "\n")
	if match.StartLine < 1 || match.EndLine < match.StartLine || match.EndLine > len(lines) {
		return fail("invalid source range")
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "## `%s` [%s]\n**File:** `%s:%d-%d`", match.Name, match.Kind, match.FilePath, match.StartLine, match.EndLine)
	if match.Signature != "" {
		fmt.Fprintf(&sb, " | **Signature:** `%s`", match.Signature)
	}
	sb.WriteString("\n")
	if match.DocComment != "" {
		fmt.Fprintf(&sb, "**Doc:** %s\n", match.DocComment)
	}
	sb.WriteString("\n```\n")
	for i := match.StartLine; i <= match.EndLine; i++ {
		fmt.Fprintf(&sb, "%-4d: %s\n", i, lines[i-1])
	}
	sb.WriteString("```\n\n")
	return match, snapshot, sb.String()
}
