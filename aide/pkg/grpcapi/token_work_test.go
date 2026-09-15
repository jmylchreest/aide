package grpcapi

import (
	"reflect"
	"testing"

	"github.com/jmylchreest/aide/aide/pkg/memory"
	"google.golang.org/protobuf/proto"
)

func TestTokenWorkRoundTripAndOldServer(t *testing.T) {
	if TokenAccountingFromProto(&TokenAccounting{}).Work != nil {
		t.Fatal("older server work must remain unavailable")
	}
	for _, work := range []*memory.TokenWork{
		nil,
		{Version: 1, ByTool: map[string]*memory.TokenWorkCounters{}},
		{Version: 1, TokenWorkCounters: memory.TokenWorkCounters{Calls: 2, Returned: 1, UnknownOutcomes: 1, MeasuredDurations: 1, MissingDurations: 1, ReturnedText: memory.TokenQuantity{Bytes: 0, Events: 1}, MissingPayload: 1}, ByTool: map[string]*memory.TokenWorkCounters{"code_search": {Calls: 2, Returned: 1, UnknownOutcomes: 1, MeasuredDurations: 1, MissingDurations: 1, ReturnedText: memory.TokenQuantity{Bytes: 0, Events: 1}, MissingPayload: 1}}},
	} {
		a := memory.NewTokenAccounting()
		a.Work = work
		wire, err := proto.Marshal(TokenAccountingToProto(a))
		if err != nil {
			t.Fatal(err)
		}
		p := &TokenAccounting{}
		if err := proto.Unmarshal(wire, p); err != nil {
			t.Fatal(err)
		}
		got := TokenAccountingFromProto(p)
		if !reflect.DeepEqual(work, got.Work) {
			t.Fatalf("work changed across wire: %#v / %#v", work, got.Work)
		}
	}
}

func TestTokenWorkMissingMessagesRemainUnavailable(t *testing.T) {
	for name, work := range map[string]*TokenWork{
		"missing_totals":     {Version: 1},
		"missing_total_text": {Version: 1, Totals: &TokenWorkCounters{}},
		"nil_tool_row":       {Version: 1, Totals: &TokenWorkCounters{ReturnedText: &TokenQuantity{}}, ByTool: map[string]*TokenWorkCounters{"code_outline": nil}},
		"missing_tool_text":  {Version: 1, Totals: &TokenWorkCounters{ReturnedText: &TokenQuantity{}}, ByTool: map[string]*TokenWorkCounters{"code_outline": {}}},
	} {
		t.Run(name, func(t *testing.T) {
			check := func(p *TokenAccounting) {
				t.Helper()
				if got := TokenAccountingFromProto(p).Work; got != nil {
					t.Fatalf("missing measurements became known work: %+v", got)
				}
			}
			p := &TokenAccounting{Version: 1, Work: work}
			check(p)
			wire, err := proto.Marshal(p)
			if err != nil {
				t.Fatal(err)
			}
			decoded := &TokenAccounting{}
			if err := proto.Unmarshal(wire, decoded); err != nil {
				t.Fatal(err)
			}
			check(decoded)
		})
	}
}
