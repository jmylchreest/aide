package grpcapi

import (
	"reflect"
	"testing"

	"github.com/jmylchreest/aide/aide/pkg/memory"
	"google.golang.org/protobuf/proto"
)

func TestDecisionContextWireRoundTrip(t *testing.T) {
	c := memory.DecisionContext{Applicability: &memory.Applicability{Kind: "context", AnyOf: []memory.ApplicabilitySelector{{Language: "go", Tools: []string{"github-actions"}, Component: "CLI", Concern: "deployment"}}}, Supersedes: []string{"base-deploy"}}
	got := DecisionContextFromProto(DecisionContextToProto(c))
	if !reflect.DeepEqual(c, got) {
		t.Fatalf("metadata lost: %#v", got)
	}
	if got := DecisionContextFromProto(nil); got.Applicability != nil {
		t.Fatal("legacy metadata became project-wide")
	}
}

func TestDecisionContextTextAndPathsWireRoundTrip(t *testing.T) {
	text, blank := "Only customer web;\nexclude admin.", ""
	for _, a := range []*memory.Applicability{
		nil,
		{Text: &blank},
		{Text: &text},
		{Kind: "context", AnyOf: []memory.ApplicabilitySelector{{Language: "go", Paths: []string{"services/billing/**", "services/payments/**"}}}},
	} {
		c := memory.DecisionContext{Applicability: a}
		data, err := proto.Marshal(DecisionContextToProto(c))
		if err != nil {
			t.Fatal(err)
		}
		var wire DecisionContext
		if err := proto.Unmarshal(data, &wire); err != nil {
			t.Fatal(err)
		}
		if got := DecisionContextFromProto(&wire); !reflect.DeepEqual(c, got) {
			t.Fatalf("metadata lost on wire: %#v != %#v", c, got)
		}
	}
}
