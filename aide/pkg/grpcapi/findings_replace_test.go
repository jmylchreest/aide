package grpcapi

import (
	"context"
	"fmt"
	"testing"

	"github.com/jmylchreest/aide/aide/pkg/findings"
	"github.com/jmylchreest/aide/aide/pkg/store"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestFindingsReplaceAnalyzer(t *testing.T) {
	fs, err := store.NewFindingsStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer fs.Close()
	srv := &Server{}
	srv.SetFindingsStore(fs)
	svc := &findingsServiceImpl{server: srv}
	for _, analyzer := range []string{"complexity", "secrets"} {
		if err := fs.AddFinding(&findings.Finding{Analyzer: analyzer, Title: "old"}); err != nil {
			t.Fatal(err)
		}
	}
	req := &FindingReplaceAnalyzerRequest{Analyzer: "complexity"}
	for i := 0; i < 260; i++ {
		req.Findings = append(req.Findings, &FindingAddRequest{Analyzer: "complexity", Title: fmt.Sprintf("replacement %d", i), FilePath: "a.go", Metadata: map[string]string{"key": "value"}})
	}
	if _, err := svc.ReplaceAnalyzer(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	rows, err := fs.ListFindings(findings.SearchOptions{Analyzer: "complexity", Limit: -1})
	if err != nil || len(rows) != 260 {
		t.Fatalf("rows=%d err=%v", len(rows), err)
	}
	for _, f := range rows {
		if f.Title == "old" || f.Metadata["key"] != "value" {
			t.Fatal("lost fields or stale finding")
		}
	}
	hits, err := fs.SearchFindings("replacement", findings.SearchOptions{Limit: 300})
	if err != nil || len(hits) != 260 {
		t.Fatalf("search=%d err=%v", len(hits), err)
	}
	// Validation and cancellation must leave the old results untouched.
	if _, err := svc.ReplaceAnalyzer(context.Background(), &FindingReplaceAnalyzerRequest{Analyzer: "complexity", Findings: []*FindingAddRequest{{Analyzer: "secrets"}}}); status.Code(err) != codes.InvalidArgument {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := svc.ReplaceAnalyzer(ctx, &FindingReplaceAnalyzerRequest{Analyzer: "complexity"}); status.Code(err) != codes.Canceled {
		t.Fatal(err)
	}
	rows, err = fs.ListFindings(findings.SearchOptions{Analyzer: "complexity", Limit: -1})
	if err != nil || len(rows) != 260 {
		t.Fatal("failed replacement mutated results")
	}
	if _, err := svc.ReplaceAnalyzer(context.Background(), &FindingReplaceAnalyzerRequest{Analyzer: "complexity"}); err != nil {
		t.Fatal(err)
	}
	rows, err = fs.ListFindings(findings.SearchOptions{Limit: -1})
	if err != nil || len(rows) != 1 || rows[0].Analyzer != "secrets" {
		t.Fatalf("unrelated analyzer lost: %v %v", rows, err)
	}
	// Large-result appends must not remove earlier chunks or other analyzers.
	for i := 0; i < 3; i++ {
		if _, err := svc.AddBatch(context.Background(), &FindingAddBatchRequest{Findings: req.Findings[:100]}); err != nil {
			t.Fatal(err)
		}
	}
	rows, err = fs.ListFindings(findings.SearchOptions{Limit: -1})
	if err != nil || len(rows) != 301 {
		t.Fatalf("batch lost findings: %d %v", len(rows), err)
	}
	if _, err := svc.AddBatch(context.Background(), &FindingAddBatchRequest{Findings: req.Findings}); status.Code(err) != codes.InvalidArgument {
		t.Fatal(err)
	}
}
