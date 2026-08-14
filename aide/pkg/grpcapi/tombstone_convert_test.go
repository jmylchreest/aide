package grpcapi_test

import (
	"testing"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/grpcapi"
	"github.com/jmylchreest/aide/aide/pkg/memory"
)

// The converters are exported so pkg/grpcapi/adapter can share them rather
// than keeping its own copy; round-trip is the contract that matters.
func TestTombstoneProtoRoundTrip(t *testing.T) {
	deletedAt := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	original := &memory.Tombstone{
		ID:        "01ABCDEF",
		Kind:      memory.TombstoneKindMemory,
		DeletedAt: deletedAt,
	}

	got := grpcapi.ProtoToTombstone(grpcapi.TombstoneToProto(original))
	if got.ID != original.ID {
		t.Errorf("ID: got %q, want %q", got.ID, original.ID)
	}
	if got.Kind != original.Kind {
		t.Errorf("Kind: got %q, want %q", got.Kind, original.Kind)
	}
	if !got.DeletedAt.Equal(deletedAt) {
		t.Errorf("DeletedAt: got %v, want %v", got.DeletedAt, deletedAt)
	}
}

func TestTombstoneProtoNil(t *testing.T) {
	if got := grpcapi.TombstoneToProto(nil); got != nil {
		t.Errorf("TombstoneToProto(nil) = %v, want nil", got)
	}
	if got := grpcapi.ProtoToTombstone(nil); got != nil {
		t.Errorf("ProtoToTombstone(nil) = %v, want nil", got)
	}
}

func TestProtoToTombstoneZeroTimestamp(t *testing.T) {
	// The proto carries a nil DeletedAt when the server has not stamped it.
	got := grpcapi.ProtoToTombstone(&grpcapi.Tombstone{Id: "X", Kind: "memory"})
	if !got.DeletedAt.IsZero() {
		t.Errorf("DeletedAt: got %v, want zero", got.DeletedAt)
	}
}
