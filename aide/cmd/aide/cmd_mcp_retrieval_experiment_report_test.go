package main

// This harness is test-only. It never writes benchmark events into a user's
// token history and does not run a model or estimate provider billing.
import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/memory"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type retrievalTrialStep struct {
	Tool       string `json:"tool"`
	Text       string `json:"text"`
	Bytes      int64  `json:"bytes"`
	Failed     bool   `json:"failed"`
	DurationNS int64  `json:"duration_ns"`
}

type retrievalTrial struct {
	Scenario             string               `json:"scenario"`
	Repetition           int                  `json:"repetition"`
	ReferenceSHA256      string               `json:"reference_sha256"`
	ReferenceBytes       int64                `json:"reference_bytes"`
	ResultBytes          int64                `json:"result_bytes"`
	ByteDelta            int64                `json:"byte_delta"`
	EstimatedTokenDelta  int64                `json:"estimated_token_delta"`
	Failures             int                  `json:"failed_attempts"`
	EvidenceChecksPassed bool                 `json:"evidence_checks_passed"`
	ProviderUsage        *int64               `json:"provider_usage"`
	ModelQuality         *bool                `json:"model_quality"`
	Steps                []retrievalTrialStep `json:"steps"`
}

func newRetrievalTrial(name, source string) *retrievalTrial {
	return &retrievalTrial{Scenario: name, ReferenceSHA256: fmt.Sprintf("%x", sha256.Sum256([]byte(source))), ReferenceBytes: int64(len(source))}
}

func (r *retrievalTrial) add(tool, text string, failed bool, elapsed time.Duration) {
	n := int64(len(text))
	r.Steps = append(r.Steps, retrievalTrialStep{Tool: tool, Text: text, Bytes: n, Failed: failed, DurationNS: elapsed.Nanoseconds()})
	r.ResultBytes += n
	r.ByteDelta = r.ReferenceBytes - r.ResultBytes
	r.EstimatedTokenDelta = memory.EstimateTextTokens(r.ReferenceBytes)
	for _, step := range r.Steps {
		r.EstimatedTokenDelta -= memory.EstimateTextTokens(step.Bytes)
	}
	if failed {
		r.Failures++
	}
}

// TestRetrievalExperiment checks deterministic evidence retrieval, not whether
// a model can solve a task. Set AIDE_RETRIEVAL_REPORT to retain raw JSON results.
func TestRetrievalExperiment(t *testing.T) {
	large := "package demo\nfunc Target() string {\n return \"current-λ\"\n}\n"
	for i := 0; i < 80; i++ {
		large += fmt.Sprintf("func Other%d() string {\n return %q\n}\n", i, strings.Repeat("unrelated", 20))
	}
	cases := []struct {
		name, file, source, indexed string
		outline, fallback           bool
		requests                    []CodeReadSymbolInput
		failures                    []bool
		want, reject                []string
	}{
		{name: "large-outline-symbol", file: "source.go", source: large, outline: true,
			requests: []CodeReadSymbolInput{{File: "source.go", Symbol: "Target"}}, failures: []bool{false}, want: []string{`return "current-λ"`}, reject: []string{"unrelated"}},
		{name: "small-outline-symbol", file: "source.go", source: "package demo\nfunc Target() string { return \"tiny\" }\n", outline: true,
			requests: []CodeReadSymbolInput{{File: "source.go", Symbol: "Target"}}, failures: []bool{false}, want: []string{`return "tiny"`}},
		{name: "ambiguous-then-explicit", file: "source.go", source: "package demo\ntype A struct{}\ntype B struct{}\nfunc (A) Target() string { return \"first\" }\nfunc (B) Target() string { return \"second\" }\n",
			requests: []CodeReadSymbolInput{{File: "source.go", Symbol: "Target"}, {File: "source.go", Symbol: "Target", StartLine: 5}}, failures: []bool{true, false}, want: []string{`return "second"`}, reject: []string{`return "first"`}},
		{name: "stale-index-current-source", file: "source.go", source: "package demo\n\nfunc Target() string{ return \"new\" }\n", indexed: "package demo\nfunc Target() string { return \"old\" }\n",
			requests: []CodeReadSymbolInput{{Symbol: "Target"}}, failures: []bool{false}, want: []string{`return "new"`, "source.go:3-3"}, reject: []string{`return "old"`}},
		{name: "partial-batch-full-fallback", file: "source.go", source: "package demo\nfunc Target() string { return \"present\" }\n", fallback: true,
			requests: []CodeReadSymbolInput{{File: "source.go", Symbols: []string{"Target", "Missing"}}}, failures: []bool{true}, want: []string{`return "present"`, "Missing"}},
		{name: "unsupported-full-fallback", file: "notes.txt", source: "The release marker is café-λ.\n", fallback: true,
			requests: []CodeReadSymbolInput{{File: "notes.txt", Symbol: "Target"}}, failures: []bool{true}},
	}
	var trials []*retrievalTrial
	// Fresh project and index per trial. Repetitions expose local variability;
	// they are not independent model runs and do not establish cache residency.
	for repetition := 1; repetition <= 3; repetition++ {
		for _, tc := range cases {
			t.Run(fmt.Sprintf("%s/%d", tc.name, repetition), func(t *testing.T) {
				s, cs, root := retrievalFixture(t)
				file := filepath.Join(root, tc.file)
				if tc.indexed != "" {
					indexRetrievalFile(t, s, cs, root, tc.file, tc.indexed)
				}
				if err := os.WriteFile(file, []byte(tc.source), 0o600); err != nil {
					t.Fatal(err)
				}
				r := newRetrievalTrial(tc.name, tc.source)
				r.Repetition = repetition
				trials = append(trials, r)
				capture := func(tool string, result *mcp.CallToolResult, err error, elapsed time.Duration, failed bool) string {
					t.Helper()
					if err != nil || result == nil {
						t.Fatalf("%s failed to execute: %v", tool, err)
					}
					var text strings.Builder
					for _, block := range result.Content {
						part, ok := block.(*mcp.TextContent)
						if !ok {
							t.Fatal("cannot measure opaque result as text")
						}
						text.WriteString(part.Text)
					}
					r.add(tool, text.String(), result.IsError, elapsed)
					if result.IsError != failed {
						t.Fatalf("%s unexpected error status: %s", tool, text.String())
					}
					if meta, ok := result.Meta["aide/retrieval"].(map[string]any); ok {
						refs, _ := json.Marshal(meta["references"])
						assertRetrievalReceipt(t, result, tool, meta["id"].(string), string(refs))
						var sources []sourceReference
						if err := json.Unmarshal(refs, &sources); err != nil || len(sources) != 1 || sources[0].File != tc.file || sources[0].SHA256 != r.ReferenceSHA256 || int64(sources[0].Bytes) != r.ReferenceBytes {
							t.Fatalf("receipt does not describe fixture source: %s", refs)
						}
					} else if !failed {
						t.Fatal("successful retrieval lost its source receipt")
					}
					return text.String()
				}
				if tc.outline {
					start := time.Now()
					result, _, err := s.handleCodeOutline(context.Background(), nil, CodeOutlineInput{File: tc.file})
					capture("code_outline", result, err, time.Since(start), false)
				}
				var last string
				for i, input := range tc.requests {
					start := time.Now()
					result, _, err := s.handleCodeReadSymbol(context.Background(), nil, input)
					last = capture("code_read_symbol", result, err, time.Since(start), tc.failures[i])
				}
				for _, want := range tc.want {
					if !strings.Contains(last, want) {
						t.Fatalf("missing required source evidence %q: %s", want, last)
					}
				}
				for _, reject := range tc.reject {
					if strings.Contains(last, reject) {
						t.Fatalf("returned wrong definition or unrelated body %q", reject)
					}
				}
				if tc.fallback {
					start := time.Now()
					full, err := os.ReadFile(file)
					elapsed := time.Since(start)
					if err != nil || string(full) != tc.source {
						t.Fatalf("fallback did not recover exact source: %v", err)
					}
					r.add("full_file", string(full), false, elapsed)
				}
				if tc.name == "large-outline-symbol" && r.ByteDelta <= 0 {
					t.Fatal("large-file fixture should reduce returned text")
				}
				if (tc.fallback || tc.name == "small-outline-symbol") && r.ByteDelta >= 0 {
					t.Fatal("small-file/fallback overhead must remain visible")
				}
				r.EvidenceChecksPassed = true
				t.Logf("reference=%d result=%d delta=%+d bytes (~%+d tokens), failures=%d", r.ReferenceBytes, r.ResultBytes, r.ByteDelta, r.EstimatedTokenDelta, r.Failures)
			})
		}
	}
	if output := os.Getenv("AIDE_RETRIEVAL_REPORT"); output != "" {
		report := struct {
			Schema    int               `json:"schema"`
			Kind      string            `json:"kind"`
			Estimator string            `json:"estimator"`
			Platform  string            `json:"platform"`
			At        time.Time         `json:"at"`
			Passed    bool              `json:"passed"`
			Limits    []string          `json:"limitations"`
			Trials    []*retrievalTrial `json:"trials"`
		}{1, "deterministic_retrieval_fixture", memory.TextEstimator, runtime.GOOS + "/" + runtime.GOARCH, time.Now().UTC(), !t.Failed(), []string{
			"Prescribed retrieval sequences with known source checks; not model answer quality or a task-success benchmark.",
			"Reference is one raw full-file read; not an observed alternative model workflow or an avoided-call count.",
			"Bytes measure rendered tool text including headers, errors and fallbacks. Protocol framing, prompts and host formatting are excluded.",
			"Token values use the central bytes/3 estimate. Provider token usage, cost and KV-cache effects are unmeasured.",
			"Durations cover in-process retrieval handlers or local fallback reads only; exclude startup, indexing, MCP transport, hooks and model time. No latency speedup ratio is inferred.",
			"Fresh project per repetition; process/filesystem/grammar caches are uncontrolled. Three repetitions are not statistical performance evidence.",
		}, trials}
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		// Refuse overwriting an existing report, including a symlink target.
		f, err := os.OpenFile(output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		_, writeErr := f.Write(append(data, '\n'))
		closeErr := f.Close()
		if writeErr != nil || closeErr != nil {
			t.Fatalf("write report: %v; close: %v", writeErr, closeErr)
		}
	}
}
