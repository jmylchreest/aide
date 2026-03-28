package adapter

import (
	"testing"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/grpcapi"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestProtoToSurveyEntry_FullEntry(t *testing.T) {
	ts := time.Date(2025, 6, 15, 10, 30, 0, 0, time.UTC)
	pe := &grpcapi.SurveyEntry{
		Id:        "test-id-1",
		Analyzer:  "topology",
		Kind:      "module",
		Name:      "auth-module",
		FilePath:  "pkg/auth",
		Title:     "Authentication module",
		Detail:    "Handles user auth",
		Metadata:  map[string]string{"lang": "go", "framework": "gin"},
		CreatedAt: timestamppb.New(ts),
	}

	entry := ProtoToSurveyEntry(pe)
	if entry == nil {
		t.Fatal("expected non-nil entry")
	}

	if entry.ID != "test-id-1" {
		t.Errorf("ID = %q, want %q", entry.ID, "test-id-1")
	}
	if entry.Analyzer != "topology" {
		t.Errorf("Analyzer = %q, want %q", entry.Analyzer, "topology")
	}
	if entry.Kind != "module" {
		t.Errorf("Kind = %q, want %q", entry.Kind, "module")
	}
	if entry.Name != "auth-module" {
		t.Errorf("Name = %q, want %q", entry.Name, "auth-module")
	}
	if entry.FilePath != "pkg/auth" {
		t.Errorf("FilePath = %q, want %q", entry.FilePath, "pkg/auth")
	}
	if entry.Title != "Authentication module" {
		t.Errorf("Title = %q, want %q", entry.Title, "Authentication module")
	}
	if entry.Detail != "Handles user auth" {
		t.Errorf("Detail = %q, want %q", entry.Detail, "Handles user auth")
	}
	if len(entry.Metadata) != 2 {
		t.Errorf("Metadata len = %d, want 2", len(entry.Metadata))
	}
	if entry.Metadata["lang"] != "go" {
		t.Errorf("Metadata[lang] = %q, want %q", entry.Metadata["lang"], "go")
	}
	if entry.Metadata["framework"] != "gin" {
		t.Errorf("Metadata[framework] = %q, want %q", entry.Metadata["framework"], "gin")
	}
	if !entry.CreatedAt.Equal(ts) {
		t.Errorf("CreatedAt = %v, want %v", entry.CreatedAt, ts)
	}
}

func TestProtoToSurveyEntry_NilInput(t *testing.T) {
	entry := ProtoToSurveyEntry(nil)
	if entry != nil {
		t.Errorf("expected nil, got %+v", entry)
	}
}

func TestProtoToSurveyEntry_NilTimestamp(t *testing.T) {
	pe := &grpcapi.SurveyEntry{
		Id:        "test-id-2",
		Analyzer:  "churn",
		Kind:      "hot_file",
		Name:      "main.go",
		CreatedAt: nil,
	}

	entry := ProtoToSurveyEntry(pe)
	if entry == nil {
		t.Fatal("expected non-nil entry")
	}
	if !entry.CreatedAt.IsZero() {
		t.Errorf("CreatedAt = %v, want zero time", entry.CreatedAt)
	}
}

func TestProtoToSurveyEntry_EmptyMetadata(t *testing.T) {
	pe := &grpcapi.SurveyEntry{
		Id:       "test-id-3",
		Analyzer: "entrypoints",
		Kind:     "entrypoint",
		Metadata: nil,
	}

	entry := ProtoToSurveyEntry(pe)
	if entry == nil {
		t.Fatal("expected non-nil entry")
	}
	if entry.Metadata != nil {
		t.Errorf("Metadata = %v, want nil", entry.Metadata)
	}
}

func TestProtoToSurveyEntry_EmptyStrings(t *testing.T) {
	pe := &grpcapi.SurveyEntry{
		Id:       "",
		Analyzer: "",
		Kind:     "",
		Name:     "",
		FilePath: "",
		Title:    "",
		Detail:   "",
	}

	entry := ProtoToSurveyEntry(pe)
	if entry == nil {
		t.Fatal("expected non-nil entry")
	}
	if entry.ID != "" {
		t.Errorf("ID = %q, want empty", entry.ID)
	}
	if entry.Analyzer != "" {
		t.Errorf("Analyzer = %q, want empty", entry.Analyzer)
	}
}
