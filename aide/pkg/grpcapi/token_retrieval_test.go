package grpcapi

import (
	"reflect"
	"testing"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/memory"
	"google.golang.org/protobuf/proto"
)

func TestRetrievalAccountingWireRoundTrip(t *testing.T) {
	at := time.Now().UTC()
	original := memory.NewTokenAccounting()
	original.Retrievals = &memory.TokenRetrievals{WindowsLimited: true, UnwindowedEvents: 2, Windows: []*memory.RetrievalWindow{{
		Host: "host", SessionID: "session", ActorID: "actor", Epoch: "epoch", First: at, Last: at, Boundary: "open_or_unknown", Events: 2,
		Observed: memory.TokenQuantity{Bytes: 30, EstimatedTokens: 10, Events: 1}, Comparison: &memory.TokenChange{BeforeBytes: 15, AfterBytes: 30, DeltaBytes: -15, EstimatedTokenDelta: -5, Events: 2}, MissingPayload: 1,
		Issues: []string{"missing_payload"}, Sources: []*memory.RetrievalSource{{File: "file.ts", SHA256: "hash", Bytes: 15}}, Steps: []*memory.RetrievalStep{{ID: "1", InvocationID: "a", At: at, Tool: "Read", Status: "unverified"}, {ID: "2", InvocationID: "b", At: at, Tool: "Read", Status: "full_file", Text: &memory.TokenQuantity{Events: 1}}},
	}}}
	wire, err := proto.Marshal(TokenAccountingToProto(original))
	if err != nil {
		t.Fatal(err)
	}
	decoded := &TokenAccounting{}
	if err := proto.Unmarshal(wire, decoded); err != nil {
		t.Fatal(err)
	}
	got := TokenAccountingFromProto(decoded)
	if !reflect.DeepEqual(original.Retrievals, got.Retrievals) {
		t.Fatalf("retrieval evidence changed across wire: %+v", got.Retrievals)
	}
	if TokenAccountingFromProto(&TokenAccounting{}).Retrievals != nil {
		t.Fatal("old server acquired a fabricated empty report")
	}
}
