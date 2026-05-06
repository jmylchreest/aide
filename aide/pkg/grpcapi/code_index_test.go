package grpcapi

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/jmylchreest/aide/aide/pkg/code"
	"github.com/jmylchreest/aide/aide/pkg/grammar"
	"github.com/jmylchreest/aide/aide/pkg/store"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// fakeIndexStream captures CodeIndexEvent payloads sent by the server and
// reports the supplied context — enough to drive codeServiceImpl.Index without
// a real gRPC connection.
type fakeIndexStream struct {
	grpc.ServerStream
	ctx    context.Context
	events []*CodeIndexEvent
	// onSend, when non-nil, is invoked after every Send. Useful to cancel the
	// context partway through a walk.
	onSend func(*CodeIndexEvent)
}

func (f *fakeIndexStream) Context() context.Context     { return f.ctx }
func (f *fakeIndexStream) SetHeader(metadata.MD) error  { return nil }
func (f *fakeIndexStream) SendHeader(metadata.MD) error { return nil }
func (f *fakeIndexStream) SetTrailer(metadata.MD)       {}
func (f *fakeIndexStream) SendMsg(m any) error          { return f.Send(m.(*CodeIndexEvent)) }
func (f *fakeIndexStream) RecvMsg(m any) error          { return nil }
func (f *fakeIndexStream) Send(ev *CodeIndexEvent) error {
	f.events = append(f.events, ev)
	if f.onSend != nil {
		f.onSend(ev)
	}
	return nil
}

// newCodeServiceFixture wires a codeServiceImpl with a real CodeStore in tmpDir
// and a real grammar loader (Go is built in, so .go files index for real).
func newCodeServiceFixture(t *testing.T, projDir string) *codeServiceImpl {
	t.Helper()
	aideDir := filepath.Join(projDir, ".aide", "memory")
	if err := os.MkdirAll(aideDir, 0o755); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(aideDir, "memory.db")
	indexPath, searchPath := filepath.Join(aideDir, "code", "index.db"), filepath.Join(aideDir, "code", "search.bleve")
	cs, err := store.NewCodeStore(indexPath, searchPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })

	loader := grammar.NewCompositeLoader()
	srv := &Server{dbPath: dbPath, grammarLoader: loader}
	srv.SetCodeStore(cs)
	return &codeServiceImpl{server: srv, parser: code.NewParser(loader)}
}

func writeGoFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestCodeIndex_StreamingProgress verifies that one progress event is emitted
// per indexed file and a single summary event closes the stream, with totals
// matching the per-file events.
func TestCodeIndex_StreamingProgress(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	writeGoFile(t, filepath.Join(tmp, "a.go"), "package a\nfunc A() {}\n")
	writeGoFile(t, filepath.Join(tmp, "sub", "b.go"), "package b\nfunc B() {}\n")

	svc := newCodeServiceFixture(t, tmp)
	stream := &fakeIndexStream{ctx: context.Background()}

	if err := svc.Index(&CodeIndexRequest{Paths: []string{tmp}, Force: true}, stream); err != nil {
		t.Fatalf("Index: %v", err)
	}

	var progressEvents int
	var summary *CodeIndexResponse
	for _, ev := range stream.events {
		switch e := ev.Event.(type) {
		case *CodeIndexEvent_Progress:
			progressEvents++
			if e.Progress.Path == "" {
				t.Errorf("progress event missing path: %+v", e.Progress)
			}
		case *CodeIndexEvent_Summary:
			summary = e.Summary
		}
	}
	if progressEvents != 2 {
		t.Errorf("expected 2 progress events, got %d", progressEvents)
	}
	if summary == nil {
		t.Fatal("expected summary event")
	}
	if got, want := summary.FilesIndexed, int32(2); got != want {
		t.Errorf("FilesIndexed: got %d want %d", got, want)
	}
	if summary.SymbolsIndexed < 2 {
		t.Errorf("SymbolsIndexed: got %d, want at least 2", summary.SymbolsIndexed)
	}
}

// TestCodeIndex_ContextCancellationStopsWalk verifies that cancelling the
// stream context after the first event halts the walk before all files are
// processed. Without ctx.Err() in the WalkFunc, this test would see N events
// before the cancel takes effect — so it exercises the cooperative cancel.
func TestCodeIndex_ContextCancellationStopsWalk(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	for i := 0; i < 50; i++ {
		writeGoFile(t, filepath.Join(tmp, "f"+string(rune('a'+i%26))+".go"),
			"package x\nfunc F() {}\n")
	}

	svc := newCodeServiceFixture(t, tmp)
	ctx, cancel := context.WithCancel(context.Background())
	stream := &fakeIndexStream{ctx: ctx}
	stream.onSend = func(*CodeIndexEvent) { cancel() }

	err := svc.Index(&CodeIndexRequest{Paths: []string{tmp}, Force: true}, stream)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if got := len(stream.events); got >= 50 {
		t.Errorf("walk did not honour cancellation; processed %d/50 files", got)
	}
}

// TestCodeIndex_SkipsUnchangedFiles verifies that an incremental (force=false)
// re-index emits skip progress events rather than re-parsing every file.
func TestCodeIndex_SkipsUnchangedFiles(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	writeGoFile(t, filepath.Join(tmp, "a.go"), "package a\nfunc A() {}\n")

	svc := newCodeServiceFixture(t, tmp)
	stream1 := &fakeIndexStream{ctx: context.Background()}
	if err := svc.Index(&CodeIndexRequest{Paths: []string{tmp}, Force: true}, stream1); err != nil {
		t.Fatalf("first Index: %v", err)
	}

	stream2 := &fakeIndexStream{ctx: context.Background()}
	if err := svc.Index(&CodeIndexRequest{Paths: []string{tmp}, Force: false}, stream2); err != nil {
		t.Fatalf("second Index: %v", err)
	}

	var skipped, indexed int
	var summary *CodeIndexResponse
	for _, ev := range stream2.events {
		switch e := ev.Event.(type) {
		case *CodeIndexEvent_Progress:
			if e.Progress.Skipped {
				skipped++
			} else {
				indexed++
			}
		case *CodeIndexEvent_Summary:
			summary = e.Summary
		}
	}
	if skipped != 1 || indexed != 0 {
		t.Errorf("expected 1 skipped 0 indexed on incremental, got %d skipped %d indexed", skipped, indexed)
	}
	if summary == nil || summary.FilesSkipped != 1 || summary.FilesIndexed != 0 {
		t.Errorf("summary mismatch: %+v", summary)
	}
}

// TestCodeIndex_RejectsPathsOutsideProject ensures path-validation errors
// surface from the streaming RPC just as they did from the unary version.
func TestCodeIndex_RejectsPathsOutsideProject(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	svc := newCodeServiceFixture(t, tmp)
	stream := &fakeIndexStream{ctx: context.Background()}

	other := t.TempDir()
	err := svc.Index(&CodeIndexRequest{Paths: []string{other}}, stream)
	if err == nil {
		t.Fatal("expected error for path outside project")
	}
}
