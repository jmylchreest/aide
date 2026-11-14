package grpcapi

import (
	"github.com/jmylchreest/aide/aide/pkg/memory"
	"reflect"
	"testing"
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
