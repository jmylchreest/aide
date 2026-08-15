package survey

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// All fixtures in this file are synthetic: scratch git repositories built
// inside b.TempDir() and in-memory CodeGrapher/CodeSearcher fixtures (the
// same mocks used by the package's unit tests). No real repository, code
// index, or project checkout is ever read.

const benchSvcFileTmpl = `package svc

// Handler%d handles a synthetic request.
func Handler%d(id int) string {
	if id <= 0 {
		return ""
	}
	return "ok"
}

func helper%d() int {
	return %d
}
`

const benchHotFileTmpl = `package svc

// hot function, updated in every churn commit (version %d)
func hotPath(v int) int {
	sum := 0
	for i := 0; i < v; i++ {
		sum += i
	}
	return sum
}
`

func benchWriteFile(b *testing.B, root, name, content string) {
	b.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		b.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		b.Fatalf("WriteFile: %v", err)
	}
}

func benchCommitAll(b *testing.B, repo *git.Repository, msg string, when time.Time) {
	b.Helper()
	wt, err := repo.Worktree()
	if err != nil {
		b.Fatalf("Worktree: %v", err)
	}
	if _, err := wt.Add("."); err != nil {
		b.Fatalf("wt.Add: %v", err)
	}
	if _, err := wt.Commit(msg, &git.CommitOptions{
		Author: &object.Signature{
			Name:  "bench",
			Email: "bench@example.com",
			When:  when,
		},
	}); err != nil {
		b.Fatalf("Commit: %v", err)
	}
}

// buildBenchSurveyRepo creates a scratch git repository in b.TempDir() with
// 5 service files committed individually plus 6 update commits to a single
// hot file, giving churn analysis real history to walk (11 commits).
// It returns the repo dir and the HEAD hash captured just before the last 3
// commits, so freshness benchmarks can measure a "3 commits behind" walk.
func buildBenchSurveyRepo(b *testing.B) (string, string) {
	b.Helper()

	dir := b.TempDir()
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		b.Fatalf("git.PlainInit: %v", err)
	}

	when := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	benchWriteFile(b, dir, "README.md", "# synthetic bench repo\n")
	benchCommitAll(b, repo, "initial commit", when)

	for i := 0; i < 5; i++ {
		name := fmt.Sprintf("pkg/svc%d.go", i)
		benchWriteFile(b, dir, name, fmt.Sprintf(benchSvcFileTmpl, i, i, i, i))
		when = when.Add(time.Second)
		benchCommitAll(b, repo, "add "+name, when)
	}

	// Capture a run commit 3 commits behind the eventual HEAD.
	head, err := repo.Head()
	if err != nil {
		b.Fatalf("repo.Head: %v", err)
	}
	runCommit := head.Hash().String()

	for i := 0; i < 3; i++ {
		benchWriteFile(b, dir, "pkg/hot.go", fmt.Sprintf(benchHotFileTmpl, i))
		when = when.Add(time.Second)
		benchCommitAll(b, repo, fmt.Sprintf("update hot.go v%d", i), when)
	}

	return dir, runCommit
}

// benchCallGraphFixture builds an in-memory CodeGrapher over a small
// synthetic call graph:
//
//	routeSetup -> handleRequest -> {validateInput, fetchUser, renderResponse}
//	fetchUser -> dbQuery
//
// It reuses the mockCodeGrapher from code_graph_test.go.
func benchCallGraphFixture() *mockCodeGrapher {
	cg := newMockCodeGrapher()

	cg.symbols["handleRequest"] = []SymbolHit{
		{Name: "handleRequest", Kind: "function", FilePath: "server.go", Line: 10, EndLine: 80, Language: "go"},
	}
	cg.symbols["validateInput"] = []SymbolHit{
		{Name: "validateInput", Kind: "function", FilePath: "validate.go", Line: 5, EndLine: 30, Language: "go"},
	}
	cg.symbols["fetchUser"] = []SymbolHit{
		{Name: "fetchUser", Kind: "function", FilePath: "user.go", Line: 5, EndLine: 40, Language: "go"},
	}
	cg.symbols["renderResponse"] = []SymbolHit{
		{Name: "renderResponse", Kind: "function", FilePath: "render.go", Line: 5, EndLine: 25, Language: "go"},
	}
	cg.symbols["dbQuery"] = []SymbolHit{
		{Name: "dbQuery", Kind: "function", FilePath: "db.go", Line: 5, EndLine: 20, Language: "go"},
	}
	cg.symbols["routeSetup"] = []SymbolHit{
		{Name: "routeSetup", Kind: "function", FilePath: "router.go", Line: 10, EndLine: 50, Language: "go"},
	}

	cg.fileRefs["server.go"] = []ReferenceHit{
		{Symbol: "validateInput", Kind: "call", FilePath: "server.go", Line: 20},
		{Symbol: "fetchUser", Kind: "call", FilePath: "server.go", Line: 40},
		{Symbol: "renderResponse", Kind: "call", FilePath: "server.go", Line: 60},
	}
	cg.fileRefs["user.go"] = []ReferenceHit{
		{Symbol: "dbQuery", Kind: "call", FilePath: "user.go", Line: 15},
	}

	cg.references["handleRequest"] = []ReferenceHit{
		{Symbol: "handleRequest", Kind: "call", FilePath: "router.go", Line: 30},
	}
	cg.containing["router.go:30"] = &SymbolHit{
		Name: "routeSetup", Kind: "function", FilePath: "router.go", Line: 10, EndLine: 50, Language: "go",
	}

	return cg
}

// sinks keep the compiler from optimising away analyzer results.
var (
	benchChurnSink       *ChurnResult
	benchEntrypointsSink *EntrypointsResult
	benchCallGraphSink   *CallGraph
	benchFreshnessSink   *Freshness
)

// BenchmarkRunChurn measures the git-history churn scan over the synthetic
// repo (bounded commit walk).
func BenchmarkRunChurn(b *testing.B) {
	dir, _ := buildBenchSurveyRepo(b)

	// Sanity-check the fixture once.
	res, err := RunChurn(dir, 50, 10)
	if err != nil || res == nil || len(res.Entries) == 0 {
		b.Fatalf("fixture check failed: entries=%v err=%v", res, err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		got, err := RunChurn(dir, 50, 10)
		if err != nil {
			b.Fatalf("RunChurn: %v", err)
		}
		benchChurnSink = got
	}
}

// BenchmarkRunEntrypoints measures entrypoint analysis over a scratch tree
// with an in-memory CodeSearcher fixture.
func BenchmarkRunEntrypoints(b *testing.B) {
	dir := b.TempDir()
	benchWriteFile(b, dir, "cmd/app/main.go", benchEntrypointMainGo)
	benchWriteFile(b, dir, "cmd/cli/main.go", benchEntrypointMainGo)
	benchWriteFile(b, dir, "web/app.ts", benchEntrypointAppTS)

	cs := &mockCodeSearcher{
		symbols: []SymbolHit{
			{Name: "main", Kind: "function", FilePath: "cmd/app/main.go", Line: 5, EndLine: 15, Language: "go"},
			{Name: "main", Kind: "function", FilePath: "cmd/cli/main.go", Line: 3, EndLine: 12, Language: "go"},
			{Name: "bootstrap", Kind: "function", FilePath: "web/app.ts", Line: 3, EndLine: 14, Language: "typescript"},
		},
	}

	// Sanity-check the fixture once.
	res, err := RunEntrypoints(dir, cs)
	if err != nil || res == nil || len(res.Entries) == 0 {
		b.Fatalf("fixture check failed: entries=%v err=%v", res, err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		got, err := RunEntrypoints(dir, cs)
		if err != nil {
			b.Fatalf("RunEntrypoints: %v", err)
		}
		benchEntrypointsSink = got
	}
}

const benchEntrypointMainGo = `package main

import "fmt"

func main() {
	fmt.Println("synthetic entrypoint")
}
`

const benchEntrypointAppTS = `import { runServer } from "./server";

export function bootstrap(): void {
  runServer(8080);
}
`

// BenchmarkBuildCallGraph measures BFS call-graph construction over the
// in-memory grapher fixture.
func BenchmarkBuildCallGraph(b *testing.B) {
	cg := benchCallGraphFixture()

	// Sanity-check the fixture once.
	g, err := BuildCallGraph(cg, "handleRequest", GraphOptions{})
	if err != nil || g == nil || len(g.Nodes) < 6 || len(g.Edges) < 5 {
		b.Fatalf("fixture check failed: nodes=%d edges=%d err=%v", len(g.Nodes), len(g.Edges), err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		got, err := BuildCallGraph(cg, "handleRequest", GraphOptions{})
		if err != nil {
			b.Fatalf("BuildCallGraph: %v", err)
		}
		benchCallGraphSink = got
	}
}

// BenchmarkComputeFreshness measures computing freshness for a run commit
// that is 3 commits behind the synthetic repo's HEAD.
func BenchmarkComputeFreshness(b *testing.B) {
	dir, runCommit := buildBenchSurveyRepo(b)

	// Sanity-check the fixture once.
	f, err := ComputeFreshness(dir, runCommit)
	if err != nil || f == nil || !f.Found || f.Behind != 3 {
		b.Fatalf("fixture check failed: freshness=%v err=%v", f, err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		got, err := ComputeFreshness(dir, runCommit)
		if err != nil {
			b.Fatalf("ComputeFreshness: %v", err)
		}
		benchFreshnessSink = got
	}
}
