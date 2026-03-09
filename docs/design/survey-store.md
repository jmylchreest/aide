# Design: Codebase Survey System

## Overview

A codebase survey system that lets agents quickly understand large, unfamiliar codebases. It produces structural facts (what the codebase IS) — distinct from findings (code PROBLEMS). Survey entries are stored in a separate BoltDB+Bleve store at `.aide/survey/`, exposed via gRPC, MCP tools, CLI commands, and agent skills.

## Key Decisions

| Decision       | Choice                                     | Rationale                                                                                                                 |
| -------------- | ------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------- |
| Git operations | go-git (github.com/go-git/go-git/v5)       | Portability: no git binary required on PATH. Performance penalty acceptable — survey runs once, results cached in BoltDB. |
| Store location | Separate `.aide/survey/` directory         | Same pattern as findings. Not in main aide.db (survey is structural facts, not memories/decisions).                       |
| Call graph     | Computed on demand via code index          | No persisted graph. BFS over existing Symbol+Reference data at query time.                                                |
| Entry scope    | File-level or repo-level, never line-level | Survey describes structure (modules, entrypoints, tech stacks), not specific code locations.                              |
| Memory decay   | Not applied to survey entries              | Survey data is structural facts — doesn't lose value over time. Decay is a separate concern.                              |

## 1. Domain Types — `pkg/survey/types.go`

```go
package survey

import "time"

// Kind classifies what a survey entry describes.
const (
    KindModule      = "module"      // Package/module/workspace member
    KindEntrypoint  = "entrypoint"  // main(), HTTP handler mount, CLI root, etc.
    KindDependency  = "dependency"  // External dependency or internal module relationship
    KindTechStack   = "tech_stack"  // Language, framework, build system detected
    KindChurn       = "churn"       // Git history hotspot (high-change file/dir)
    KindSubmodule   = "submodule"   // Git submodule reference
    KindWorkspace   = "workspace"   // Monorepo workspace root (npm, go, cargo, etc.)
    KindArchPattern = "arch_pattern" // Detected architectural pattern (MVC, hexagonal, etc.)
)

// Analyzer names — who produced this entry.
const (
    AnalyzerTopology    = "topology"    // Repo structure, build systems, workspaces
    AnalyzerEntrypoints = "entrypoints" // Entry point detection
    AnalyzerChurn       = "churn"       // Git history analysis
)

// Entry represents a single survey observation about the codebase.
type Entry struct {
    ID        string            `json:"id"`                 // ULID
    Analyzer  string            `json:"analyzer"`           // Who produced this: topology, entrypoints, churn
    Kind      string            `json:"kind"`               // What this describes: module, entrypoint, dependency, ...
    Name      string            `json:"name"`               // Human-readable label (e.g. "aide/pkg/store", "main.go:main()")
    FilePath  string            `json:"file,omitempty"`     // Relative file or directory path (empty for repo-level)
    Title     string            `json:"title"`              // Short summary
    Detail    string            `json:"detail,omitempty"`   // Extended explanation
    Metadata  map[string]string `json:"metadata,omitempty"` // Analyzer-specific data (e.g. "language":"go", "commits":"142")
    CreatedAt time.Time         `json:"createdAt"`
}

// SearchOptions for filtering survey entries.
type SearchOptions struct {
    Analyzer string // Filter by analyzer name
    Kind     string // Filter by kind
    FilePath string // Filter by file path pattern (substring)
    Limit    int    // Max results (0 = default)
}

// Stats holds aggregate counts of survey entries.
type Stats struct {
    Total      int            `json:"total"`
    ByAnalyzer map[string]int `json:"byAnalyzer"`
    ByKind     map[string]int `json:"byKind"`
}

// SearchResult pairs a survey entry with its search relevance score.
type SearchResult struct {
    Entry *Entry
    Score float64
}
```

### Design notes on Entry vs Finding

| Field        | Finding | Survey Entry | Reason                                          |
| ------------ | ------- | ------------ | ----------------------------------------------- |
| Severity     | Yes     | **No**       | Survey is not flagging problems                 |
| Line/EndLine | Yes     | **No**       | Survey describes file/module-level structure    |
| Accepted     | Yes     | **No**       | Nothing to "accept" — it's a fact               |
| Kind         | No      | **Yes**      | Survey classifies WHAT is described             |
| Name         | No      | **Yes**      | Human-readable label for the structural element |

## 2. Store Interface — add to `pkg/store/interfaces.go`

```go
// SurveyStore manages codebase survey entries in a separate database.
type SurveyStore interface {
    AddEntry(e *survey.Entry) error
    GetEntry(id string) (*survey.Entry, error)
    DeleteEntry(id string) error
    SearchEntries(query string, opts survey.SearchOptions) ([]*survey.SearchResult, error)
    ListEntries(opts survey.SearchOptions) ([]*survey.Entry, error)
    GetFileEntries(filePath string) ([]*survey.Entry, error)
    ClearAnalyzer(analyzer string) (int, error)
    ReplaceEntriesForAnalyzer(analyzer string, newEntries []*survey.Entry) error
    ReplaceEntriesForAnalyzerAndFile(analyzer, filePath string, newEntries []*survey.Entry) error
    Stats(opts survey.SearchOptions) (*survey.Stats, error)
    Clear() error
    Close() error
}

var _ SurveyStore = (*SurveyStoreImpl)(nil)
```

## 3. BoltDB + Bleve Implementation — `pkg/store/survey.go`

Follow the `findings.go` pattern exactly:

- **Database path**: `.aide/survey/survey.db`
- **Search index path**: `.aide/survey/search.bleve`
- **Buckets**: `survey_entries`, `survey_meta`
- **IDs**: ULID via `ulid.Make().String()`
- **Bleve mapping**: custom analyzer with unicode tokenizer + lowercase filter. Fields indexed:
  - `name` — text (custom analyzer)
  - `title` — text (custom analyzer)
  - `detail` — text (custom analyzer)
  - `analyzer` — keyword
  - `kind` — keyword
  - `file_path` — keyword
- **Mapping hash**: SHA256 of mapping JSON stored in meta bucket. On mismatch, rebuild index.
- **`ReplaceEntriesForAnalyzer`**: Delete all entries for analyzer, add new ones, re-index — all in one bolt transaction.

```go
type SurveyStoreImpl struct {
    db         *bolt.DB
    search     bleve.Index
    dbPath     string
    searchPath string
}

func NewSurveyStore(dir string) (*SurveyStoreImpl, error) { ... }
```

## 4. Proto Service — add to `proto/aidememory.proto`

```protobuf
// =============================================================================
// Survey Service — codebase structural observations
// =============================================================================

service SurveyService {
  rpc Add(SurveyAddRequest) returns (SurveyAddResponse);
  rpc Get(SurveyGetRequest) returns (SurveyGetResponse);
  rpc Delete(SurveyDeleteRequest) returns (SurveyDeleteResponse);
  rpc Search(SurveySearchRequest) returns (SurveySearchResponse);
  rpc List(SurveyListRequest) returns (SurveySearchResponse);
  rpc GetFileEntries(SurveyFileRequest) returns (SurveySearchResponse);
  rpc ClearAnalyzer(SurveyClearAnalyzerRequest) returns (SurveyClearAnalyzerResponse);
  rpc Stats(SurveyStatsRequest) returns (SurveyStatsResponse);
  rpc Clear(SurveyClearRequest) returns (SurveyClearResponse);
}

message SurveyEntry {
  string id = 1;
  string analyzer = 2;
  string kind = 3;
  string name = 4;
  string file_path = 5;
  string title = 6;
  string detail = 7;
  map<string, string> metadata = 8;
  google.protobuf.Timestamp created_at = 9;
}

message SurveyAddRequest {
  string analyzer = 1;
  string kind = 2;
  string name = 3;
  string file_path = 4;
  string title = 5;
  string detail = 6;
  map<string, string> metadata = 7;
}

message SurveyAddResponse {
  SurveyEntry entry = 1;
}

message SurveyGetRequest {
  string id = 1;
}

message SurveyGetResponse {
  SurveyEntry entry = 1;
  bool found = 2;
}

message SurveyDeleteRequest {
  string id = 1;
}

message SurveyDeleteResponse {
  bool success = 1;
}

message SurveySearchRequest {
  string query = 1;
  string analyzer = 2;
  string kind = 3;
  string file_path = 4;
  int32 limit = 5;
}

message SurveySearchResponse {
  repeated SurveyEntry entries = 1;
}

message SurveyListRequest {
  string analyzer = 1;
  string kind = 2;
  string file_path = 3;
  int32 limit = 4;
}

message SurveyFileRequest {
  string file_path = 1;
}

message SurveyClearAnalyzerRequest {
  string analyzer = 1;
}

message SurveyClearAnalyzerResponse {
  int32 count = 1;
}

message SurveyStatsRequest {}

message SurveyStatsResponse {
  int32 total = 1;
  map<string, int32> by_analyzer = 2;
  map<string, int32> by_kind = 3;
}

message SurveyClearRequest {}

message SurveyClearResponse {
  bool success = 1;
}
```

## 5. Generated gRPC Code

Run `protoc` (or project's code generation command) after proto changes. Produces:

- `pkg/grpcapi/aidememory.pb.go` — message types
- `pkg/grpcapi/aidememory_grpc.pb.go` — service client/server interfaces

## 6. gRPC Server — modify `pkg/grpcapi/server.go`

Add `surveyServiceImpl` following the `findingsServiceImpl` pattern:

```go
// In Server struct — add surveyStore field + RWMutex
surveyStore   SurveyStore
surveyStoreMu sync.RWMutex

// SetSurveyStore — called by daemon to inject the store
func (s *Server) SetSurveyStore(store SurveyStore) { ... }

// GetSurveyStore — called by RPC handlers
func (s *Server) GetSurveyStore() (SurveyStore, error) { ... }

// surveyServiceImpl — implements SurveyServiceServer
type surveyServiceImpl struct {
    aidememory.UnimplementedSurveyServiceServer
    server *Server
}

// Register in Start():
aidememory.RegisterSurveyServiceServer(grpcServer, &surveyServiceImpl{server: s})
```

Proto ↔ domain converters (in server.go):

- `surveyEntryToProto(e *survey.Entry) *aidememory.SurveyEntry`
- `protoToSurveyEntry(p *aidememory.SurveyEntry) *survey.Entry` (in adapter)

## 7. gRPC Client Adapter — `cmd/aide/grpc_survey_adapter.go`

```go
// grpcSurveyAdapter implements store.SurveyStore over gRPC.
type grpcSurveyAdapter struct {
    client aidememory.SurveyServiceClient
}

var _ store.SurveyStore = (*grpcSurveyAdapter)(nil)

func (a *grpcSurveyAdapter) AddEntry(e *survey.Entry) error { ... }
func (a *grpcSurveyAdapter) GetEntry(id string) (*survey.Entry, error) { ... }
// ... all SurveyStore methods, converting proto ↔ domain
```

## 8. gRPC Client — modify `pkg/grpcapi/client.go`

```go
// Add to Client struct:
Surveys aidememory.SurveyServiceClient

// In NewClient():
c.Surveys = aidememory.NewSurveyServiceClient(conn)
```

## 9. MCP Tools — `cmd/aide/cmd_mcp_survey.go`

Four tools, clearly distinguished from findings:

### `survey_search`

```
Search codebase survey entries by keyword using full-text search.

Survey entries describe WHAT the codebase IS — its structure, modules,
entry points, tech stack, and dependencies. This is different from
findings which flag code PROBLEMS.

**Examples:**
- "auth" → find auth-related modules and entry points
- "grpc" → find gRPC service definitions and handlers
- "main" → find all entry points

Filter by analyzer (topology, entrypoints, churn),
kind (module, entrypoint, dependency, tech_stack, churn, ...),
or file path.

**Tip:** Use survey_list to browse by kind without a keyword.
Survey is populated by running 'aide survey run'.
```

### `survey_list`

```
List codebase survey entries with optional filters.

Returns structural observations about the codebase — modules, entry points,
tech stack, dependencies, change hotspots. NOT code quality issues (use
findings_list for those).

**When to use:**
- "What modules exist?" → filter by kind=module
- "Where are the entry points?" → filter by kind=entrypoint
- "What tech stack?" → filter by kind=tech_stack
- "What changes most?" → filter by kind=churn
```

### `survey_stats`

```
Get a structural overview of the codebase from survey analysis.

Returns total survey entry count with breakdowns by analyzer and kind.
**Start here** — call this first when onboarding to a new codebase or
when asked "what is this codebase?" Then use survey_list to drill into
specific structural areas.

NOTE: For code quality/health, use findings_stats instead.
```

### `survey_run`

```
Run codebase survey analyzers to discover structure.

Analyzers: topology (repo structure, build systems, workspaces),
entrypoints (main functions, HTTP handlers, CLI roots),
churn (git history hotspots).

Specify analyzer to run just one, or omit to run all.
Results are cached — re-running replaces previous results for that analyzer.
```

Input types:

```go
type SurveySearchInput struct {
    Query    string `json:"query" jsonschema:"Search query for survey entry names, titles, and details."`
    Analyzer string `json:"analyzer,omitempty" jsonschema:"Filter by analyzer: topology, entrypoints, churn"`
    Kind     string `json:"kind,omitempty" jsonschema:"Filter by kind: module, entrypoint, dependency, tech_stack, churn, submodule, workspace, arch_pattern"`
    FilePath string `json:"file,omitempty" jsonschema:"Filter by file path pattern (substring match)"`
    Limit    int    `json:"limit,omitempty" jsonschema:"Maximum results (default 20)"`
}

type SurveyListInput struct {
    Analyzer string `json:"analyzer,omitempty" jsonschema:"Filter by analyzer: topology, entrypoints, churn"`
    Kind     string `json:"kind,omitempty" jsonschema:"Filter by kind: module, entrypoint, dependency, tech_stack, churn, submodule, workspace, arch_pattern"`
    FilePath string `json:"file,omitempty" jsonschema:"Filter by file path pattern (substring match)"`
    Limit    int    `json:"limit,omitempty" jsonschema:"Maximum results (default 100)"`
}

type SurveyStatsInput struct{}

type SurveyRunInput struct {
    Analyzer string `json:"analyzer,omitempty" jsonschema:"Run specific analyzer: topology, entrypoints, churn. Omit to run all."`
}
```

## 10. MCP Wiring — modify `cmd/aide/cmd_mcp.go`

```go
// Add to MCPServer struct:
surveyStore store.SurveyStore

// Add initMCPSurveyStore() following initMCPFindingsStore() pattern:
// - Primary mode: open SurveyStoreImpl directly, set on gRPC server
// - Client mode: wrap gRPC client with grpcSurveyAdapter

// In registerTools():
s.registerSurveyTools()
```

## 11. CLI Backend — `cmd/aide/backend_survey.go`

```go
// Backend methods for CLI, following backend_findings.go pattern:
func (b *Backend) SurveyRun(analyzer string) error
func (b *Backend) SurveySearch(query string, opts survey.SearchOptions) ([]*survey.SearchResult, error)
func (b *Backend) SurveyList(opts survey.SearchOptions) ([]*survey.Entry, error)
func (b *Backend) SurveyStats(opts survey.SearchOptions) (*survey.Stats, error)
func (b *Backend) SurveyClear(analyzer string) (int, error)
```

Each method: if gRPC client available, use it; else open local store.

## 12. CLI Commands — `cmd/aide/cmd_survey.go`

```go
func cmdSurveyDispatcher(args []string, b *Backend) {
    // Subcommands:
    // "run" [--analyzer=X]         — run survey analyzers
    // "search" <query> [filters]   — full-text search
    // "list" [filters]             — list with filters
    // "stats"                      — aggregate counts
    // "clear" [--analyzer=X]       — clear entries
}
```

Register in `main.go` command dispatch.

## 13. Survey Analyzers

### 13a. Topology Analyzer — `pkg/survey/topology.go`

Detects repo structure by filesystem inspection (no git needed):

```go
func RunTopology(rootDir string, store SurveyStore) error
```

**Detects:**

- Go modules (`go.mod` → workspace/module entries, parse `module` directive)
- Node.js (`package.json` → workspace entries, parse `name`, `workspaces`)
- Python (`pyproject.toml`, `setup.py`, `setup.cfg`)
- Rust (`Cargo.toml`, `[workspace]`)
- Build systems (`Makefile`, `CMakeLists.txt`, `BUILD.bazel`, `Justfile`, `Taskfile.yml`)
- Docker (`Dockerfile`, `docker-compose.yml`)
- CI/CD (`.github/workflows/`, `.gitlab-ci.yml`, `Jenkinsfile`)
- Monorepo tools (`nx.json`, `lerna.json`, `turbo.json`, `pnpm-workspace.yaml`)

**Output**: `Entry{Kind: KindTechStack | KindModule | KindWorkspace, Analyzer: "topology"}`

### 13b. Entrypoints Analyzer — `pkg/survey/entrypoints.go`

Uses the existing code index (tree-sitter symbols) to find entry points:

```go
func RunEntrypoints(rootDir string, codeStore store.CodeIndexStore, surveyStore SurveyStore) error
```

**Detection strategy (language-aware):**

- Go: `func main()` in `package main`, `func init()`
- Go (HTTP): `http.HandleFunc`, `mux.Handle`, `gin.GET/POST`, `echo.GET/POST`
- Go (gRPC): `Register*Server()` calls
- JS/TS: `export default`, files matching `pages/`, `app/`, `routes/`
- Python: `if __name__ == "__main__"`, `@app.route`
- Rust: `fn main()`
- CLI: cobra `&cobra.Command{}`, click decorators, argparse

**Output**: `Entry{Kind: KindEntrypoint, Analyzer: "entrypoints"}`

### 13c. Churn Analyzer — `pkg/survey/churn.go`

Uses **go-git** to analyze commit history:

```go
func RunChurn(rootDir string, store SurveyStore) error
```

**Strategy:**

1. Open repo via `git.PlainOpen(rootDir)`
2. Walk commit log (configurable depth, default 500 commits)
3. For each commit, get diff stats (files changed + lines added/removed)
4. Aggregate: file → {commits, totalLinesChanged}
5. Rank by commit frequency × change magnitude
6. Emit top-N files as `Entry{Kind: KindChurn, Metadata: {"commits": "142", "lines_changed": "5230"}}`

**go-git specifics:**

```go
import (
    "github.com/go-git/go-git/v5"
    "github.com/go-git/go-git/v5/plumbing/object"
)

repo, err := git.PlainOpen(rootDir)
// handle: ErrRepositoryNotExists → graceful skip

logIter, err := repo.Log(&git.LogOptions{All: true})
// Walk commits, compute stats
```

**Submodule detection** (also in churn or topology):

```go
worktree, err := repo.Worktree()
subs, err := worktree.Submodules()
for _, sub := range subs {
    // Emit Entry{Kind: KindSubmodule, Name: sub.Config().Name, FilePath: sub.Config().Path}
}
```

## 14. go-git Integration — `pkg/survey/gitrepo.go`

Thin wrapper for common go-git operations used by analyzers:

```go
package survey

import (
    "github.com/go-git/go-git/v5"
    "github.com/go-git/go-git/v5/plumbing/object"
)

// GitRepo wraps go-git for survey-specific operations.
type GitRepo struct {
    repo *git.Repository
}

// OpenGitRepo opens a git repository. Returns nil, nil if not a git repo.
func OpenGitRepo(dir string) (*GitRepo, error) {
    repo, err := git.PlainOpen(dir)
    if err == git.ErrRepositoryNotExists {
        return nil, nil // Not a git repo — not an error for survey purposes
    }
    if err != nil {
        return nil, fmt.Errorf("failed to open git repo: %w", err)
    }
    return &GitRepo{repo: repo}, nil
}

func (g *GitRepo) Submodules() ([]*config.Submodule, error) { ... }
func (g *GitRepo) CommitLog(maxCommits int) ([]*object.Commit, error) { ... }
func (g *GitRepo) FileChurnStats(maxCommits int) (map[string]*ChurnStat, error) { ... }
```

### Unhappy paths handled:

| Scenario                       | Handling                                                               |
| ------------------------------ | ---------------------------------------------------------------------- |
| Not a git repo                 | `OpenGitRepo` returns `(nil, nil)` — analyzers skip git-dependent work |
| Corrupt `.git` directory       | Return wrapped error, survey continues with non-git analyzers          |
| Submodule checkout missing     | Emit entry with `Metadata: {"status": "not_checked_out"}`              |
| Shallow clone (no history)     | Churn analyzer emits fewer entries, logs warning                       |
| Very large repo (>10k commits) | Configurable `maxCommits` cap (default 500)                            |

## 15. `code_graph` MCP Tool — modify `cmd/aide/cmd_mcp_code.go`

### Add `GetContainingSymbol` to CodeIndexStore

```go
// In pkg/store/interfaces.go — add to CodeIndexStore:
GetContainingSymbol(filePath string, line int) (*code.Symbol, error)

// Implementation in pkg/store/code.go:
// Get all symbols for file, find the one whose BodyStartLine <= line <= BodyEndLine
// Return the most specific (innermost) match
```

### `code_graph` MCP tool

```
Explore the call graph starting from a symbol.

Given a function or method name, traces callers or callees using the code index.
Returns a tree of call relationships up to a configurable depth.

**Direction:**
- "callees" (default) — what does this function call?
- "callers" — what calls this function?

**How it works:**
- Uses the code index (symbol definitions + references) to build the graph on demand
- No pre-computed graph is stored
- BFS traversal: find references → for each call site → find containing symbol → recurse

**Tip:** Index must be populated first (aide code index). Use code_search to find
the exact symbol name before graphing.
```

```go
type CodeGraphInput struct {
    Symbol    string `json:"symbol" jsonschema:"Symbol name to start from (e.g. 'handleRequest', 'UserService.Create')"`
    Direction string `json:"direction,omitempty" jsonschema:"'callees' (what it calls) or 'callers' (what calls it). Default: callees"`
    Depth     int    `json:"depth,omitempty" jsonschema:"Max traversal depth (default 3, max 10)"`
    File      string `json:"file,omitempty" jsonschema:"Filter to symbol in specific file (substring match)"`
}
```

## 16. Skills

### `skills/survey/SKILL.md` — Survey Orchestration

Prompts the agent to:

1. Run `survey_run` to populate the store
2. Call `survey_stats` to see what was found
3. Present a structured overview to the user

### `skills/orient/SKILL.md` — "Where Do I Start?"

Prompts the agent to:

1. Check `survey_list kind=entrypoint` for entry points
2. Check `survey_list kind=module` for module structure
3. Suggest a reading order based on entry points + most-churn files
4. Use `code_graph` to show key call chains from entry points

### `skills/summarize/SKILL.md` — Multi-Scale Summary

Prompts the agent to:

1. Survey stats for overview
2. List modules and entry points
3. Produce a layered summary: 1-sentence → 1-paragraph → detailed

### `skills/glossary/SKILL.md` — Domain Language

Prompts the agent to:

1. Survey list for tech stack and modules
2. Code search for domain-specific types/interfaces
3. Build a glossary of domain terms with definitions

### `skills/compare/SKILL.md` — Codebase Comparison

Prompts the agent to:

1. Run survey on both repos/branches
2. Diff the survey entries
3. Present structural differences

## Data Flow

```
                    ┌──────────────┐
                    │  Analyzers   │
                    │  (topology,  │
                    │  entrypoints,│
                    │  churn)      │
                    └──────┬───────┘
                           │ survey.Entry
                           ▼
┌──────────┐    ┌──────────────────┐    ┌────────────┐
│ CLI cmds │───▶│   SurveyStore    │◀───│ MCP tools  │
│          │    │  (BoltDB+Bleve)  │    │ (4 tools)  │
└──────────┘    └────────┬─────────┘    └────────────┘
                         │
              ┌──────────┴──────────┐
              │                     │
    ┌─────────▼────────┐  ┌────────▼─────────┐
    │ Direct (primary)  │  │ gRPC (secondary) │
    │ SurveyStoreImpl   │  │ grpcSurveyAdapter│
    └──────────────────┘  └──────────────────┘
```

## Files to Create

| File                                   | Purpose                                   |
| -------------------------------------- | ----------------------------------------- |
| `aide/pkg/survey/types.go`             | Domain types: Entry, SearchOptions, Stats |
| `aide/pkg/survey/gitrepo.go`           | go-git wrapper for survey operations      |
| `aide/pkg/survey/topology.go`          | Topology analyzer                         |
| `aide/pkg/survey/entrypoints.go`       | Entry point detection analyzer            |
| `aide/pkg/survey/churn.go`             | Git churn analyzer                        |
| `aide/pkg/store/survey.go`             | SurveyStoreImpl (BoltDB + Bleve)          |
| `aide/pkg/store/survey_test.go`        | Store unit tests                          |
| `aide/cmd/aide/grpc_survey_adapter.go` | gRPC client adapter                       |
| `aide/cmd/aide/cmd_mcp_survey.go`      | MCP tool registration + handlers          |
| `aide/cmd/aide/cmd_survey.go`          | CLI subcommand dispatcher                 |
| `aide/cmd/aide/backend_survey.go`      | CLI backend methods                       |
| `skills/survey/SKILL.md`               | Survey orchestration skill                |
| `skills/orient/SKILL.md`               | "Where to start" skill                    |
| `skills/summarize/SKILL.md`            | Multi-scale summary skill                 |
| `skills/glossary/SKILL.md`             | Domain language skill                     |
| `skills/compare/SKILL.md`              | Codebase comparison skill                 |

## Files to Modify

| File                            | Change                                            |
| ------------------------------- | ------------------------------------------------- |
| `aide/pkg/store/interfaces.go`  | Add `SurveyStore` interface                       |
| `aide/proto/aidememory.proto`   | Add `SurveyService` + messages                    |
| `aide/pkg/grpcapi/server.go`    | Add `surveyServiceImpl`, converters, registration |
| `aide/pkg/grpcapi/client.go`    | Add `Surveys` client field                        |
| `aide/cmd/aide/cmd_mcp.go`      | Wire survey store, register tools                 |
| `aide/cmd/aide/cmd_mcp_code.go` | Add `code_graph` MCP tool                         |
| `aide/pkg/store/interfaces.go`  | Add `GetContainingSymbol` to `CodeIndexStore`     |
| `aide/pkg/store/code.go`        | Implement `GetContainingSymbol`                   |
| `aide/cmd/aide/main.go`         | Register `survey` CLI subcommand                  |
| `aide/go.mod`                   | Add `github.com/go-git/go-git/v5` dependency      |

## Dependencies

- `github.com/go-git/go-git/v5` — pure Go git implementation (NEW)
- `github.com/oklog/ulid/v2` — already in project
- `github.com/blevesearch/bleve/v2` — already in project
- `go.etcd.io/bbolt` — already in project
- `google.golang.org/grpc` — already in project

## Out of Scope

- **Memory decay** — not applied to survey, and decay for memories is a separate concern
- **Persisted call graph** — `code_graph` computes on demand
- **"Onboard" meta-command** — future work combining survey + orient + summarize
- **Cross-repo survey** — survey operates on single repo root (submodules enumerated but not deeply analyzed)
- **Survey diff/history** — entries are replaced per-analyzer, no versioning

## Acceptance Criteria

- [ ] `aide survey run` populates the store with topology, entrypoint, and churn entries
- [ ] `aide survey run --analyzer=topology` runs only the topology analyzer
- [ ] `aide survey list` returns all survey entries
- [ ] `aide survey list --kind=entrypoint` filters by kind
- [ ] `aide survey search "auth"` finds entries matching "auth" by full-text search
- [ ] `aide survey stats` shows totals by analyzer and kind
- [ ] `aide survey clear` removes all entries; `--analyzer=X` removes only that analyzer's entries
- [ ] MCP tool `survey_search` works via gRPC in secondary daemon mode
- [ ] MCP tool `survey_list` works via gRPC in secondary daemon mode
- [ ] MCP tool `survey_stats` works via gRPC in secondary daemon mode
- [ ] MCP tool `survey_run` works via gRPC in secondary daemon mode
- [ ] MCP tool descriptions clearly distinguish survey (structure) from findings (problems)
- [ ] Topology analyzer detects Go modules, Node.js packages, and build systems present in the repo
- [ ] Entrypoints analyzer finds `func main()` and HTTP handler registrations
- [ ] Churn analyzer reports top-N most-changed files from git history
- [ ] Churn analyzer gracefully handles non-git directories (skips, no error)
- [ ] Churn analyzer handles shallow clones (partial results, no crash)
- [ ] Submodule enumeration works for repos with submodules
- [ ] `code_graph` MCP tool traces callees from a given symbol to configurable depth
- [ ] `code_graph` MCP tool traces callers from a given symbol
- [ ] `GetContainingSymbol` returns the correct enclosing function for a given file+line
- [ ] Survey store uses separate `.aide/survey/` directory (not main aide.db)
- [ ] All gRPC adapter methods work correctly (compile-time interface check passes)
- [ ] `go build ./...` succeeds
- [ ] All new tests pass
