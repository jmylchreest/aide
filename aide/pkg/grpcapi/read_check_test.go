package grpcapi

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/jmylchreest/aide/aide/pkg/code"
	"github.com/jmylchreest/aide/aide/pkg/memory"
	"github.com/jmylchreest/aide/aide/pkg/store"
	"google.golang.org/protobuf/proto"
)

func TestReadCheckWireOptionalAndWide(t *testing.T) {
	for _, value := range []*code.TextEstimate{nil, {Bytes: 0, EstimatedTokens: 0, Estimator: memory.TextEstimator}, {Bytes: (1 << 53) - 1, EstimatedTokens: memory.EstimateTextTokens((1 << 53) - 1), Estimator: memory.TextEstimator}} {
		wire, err := proto.Marshal(&CodeReadCheckResponse{TextEstimate: ReadCheckTextEstimateToProto(value)})
		if err != nil {
			t.Fatal(err)
		}
		var received CodeReadCheckResponse
		if err := proto.Unmarshal(wire, &received); err != nil {
			t.Fatal(err)
		}
		if got := ProtoToReadCheckTextEstimate(received.TextEstimate); !reflect.DeepEqual(got, value) {
			t.Fatalf("got=%+v want=%+v", got, value)
		}
	}
}

func TestReadCheckServiceCurrentSize(t *testing.T) {
	root := t.TempDir()
	service := newCodeServiceFixture(t, root)
	path := filepath.Join(root, "file.go")
	if err := os.WriteFile(path, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	stat, _ := os.Stat(path)
	indexed := &code.FileInfo{Path: "file.go", ModTime: stat.ModTime(), Tokens: 9999}
	if err := service.server.GetCodeStore().(*store.CodeStore).SetFileInfo(indexed); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("αβ🙂"), 0600); err != nil {
		t.Fatal(err)
	}
	response, err := service.ReadCheck(context.Background(), &CodeReadCheckRequest{FilePath: "file.go"})
	if err != nil {
		t.Fatal(err)
	}
	if response.TextEstimate == nil || response.TextEstimate.Bytes != 8 || response.EstimatedTokens != 3 || response.TextEstimate.Estimator != memory.TextEstimator {
		t.Fatalf("current size lost: %+v", response)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	response, err = service.ReadCheck(context.Background(), &CodeReadCheckRequest{FilePath: "file.go"})
	if err != nil {
		t.Fatal(err)
	}
	if response.TextEstimate != nil || response.EstimatedTokens != 0 || response.Fresh {
		t.Fatalf("unknown lost: %+v", response)
	}
}
