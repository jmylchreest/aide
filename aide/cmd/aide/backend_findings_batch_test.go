package main

import (
	"context"
	"strings"
	"testing"

	"github.com/jmylchreest/aide/aide/pkg/findings"
	"github.com/jmylchreest/aide/aide/pkg/grpcapi"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type replacementClient struct {
	grpcapi.FindingsServiceClient
	err                      error
	replaced, cleared, added int
	batchSizes               []int
}

func (c *replacementClient) AddBatch(_ context.Context, req *grpcapi.FindingAddBatchRequest, _ ...grpc.CallOption) (*grpcapi.FindingReplaceAnalyzerResponse, error) {
	c.batchSizes = append(c.batchSizes, len(req.Findings))
	return &grpcapi.FindingReplaceAnalyzerResponse{}, c.err
}

func (c *replacementClient) ReplaceAnalyzer(context.Context, *grpcapi.FindingReplaceAnalyzerRequest, ...grpc.CallOption) (*grpcapi.FindingReplaceAnalyzerResponse, error) {
	c.replaced++
	return &grpcapi.FindingReplaceAnalyzerResponse{}, c.err
}

func TestFindingsReplaceLargeBatches(t *testing.T) {
	for _, code := range []codes.Code{codes.OK, codes.Internal} {
		c := &replacementClient{err: status.Error(code, "test")}
		b := &Backend{grpcClient: &grpcapi.Client{Findings: c}}
		items := make([]*findings.Finding, 260)
		for i := range items {
			items[i] = &findings.Finding{Analyzer: "security", Title: "large", Detail: strings.Repeat("x", 13000)}
		}
		err := b.grpcFindingsReplaceForAnalyzer("security", items)
		if c.replaced != 0 || c.cleared != 1 || c.added != 0 {
			t.Fatalf("unexpected fallback: %+v", c)
		}
		if code == codes.OK {
			if err != nil || len(c.batchSizes) != 3 || c.batchSizes[0] != 128 || c.batchSizes[1] != 128 || c.batchSizes[2] != 4 {
				t.Fatalf("err=%v batches=%v", err, c.batchSizes)
			}
		} else if status.Code(err) != code || len(c.batchSizes) != 1 {
			t.Fatalf("failed batch retried: %v %v", err, c.batchSizes)
		}
	}
}
func (c *replacementClient) ClearAnalyzer(context.Context, *grpcapi.FindingClearAnalyzerRequest, ...grpc.CallOption) (*grpcapi.FindingClearAnalyzerResponse, error) {
	c.cleared++
	return &grpcapi.FindingClearAnalyzerResponse{}, nil
}
func (c *replacementClient) Add(context.Context, *grpcapi.FindingAddRequest, ...grpc.CallOption) (*grpcapi.FindingAddResponse, error) {
	c.added++
	return &grpcapi.FindingAddResponse{}, nil
}

func TestFindingsReplaceFallback(t *testing.T) {
	for _, tc := range []struct {
		name                string
		code                codes.Code
		large               bool
		replace, clear, add int
		wantErr             bool
	}{
		{"batch", codes.OK, false, 1, 0, 0, false},
		{"old server", codes.Unimplemented, false, 1, 1, 1, false},
		{"failed write", codes.Internal, false, 1, 0, 0, true},
		{"large request", codes.OK, true, 0, 1, 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &replacementClient{err: status.Error(tc.code, "test")}
			b := &Backend{grpcClient: &grpcapi.Client{Findings: c}}
			f := &findings.Finding{Analyzer: "complexity", Title: "example"}
			if tc.large {
				f.Detail = strings.Repeat("x", 3<<20)
			}
			err := b.grpcFindingsReplaceForAnalyzer("complexity", []*findings.Finding{f})
			if (err != nil) != tc.wantErr || c.replaced != tc.replace || c.cleared != tc.clear || c.added != tc.add {
				t.Fatalf("err=%v replaced=%d cleared=%d added=%d", err, c.replaced, c.cleared, c.added)
			}
		})
	}
}
