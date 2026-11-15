package contextshare

import (
	"reflect"
	"testing"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/memory"
)

func TestDecisionContextRoundTrip(t *testing.T) {
	d := &memory.Decision{Topic: "django-settings", Decision: "Use Django settings", CreatedAt: time.Now().UTC(), DecisionContext: memory.DecisionContext{Applicability: &memory.Applicability{Kind: "context", AnyOf: []memory.ApplicabilitySelector{{Language: "python", Framework: "django"}}}, Supersedes: []string{"python-settings"}}}
	got, err := ParseDecision(MarshalDecision(d))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(d, got) {
		t.Fatalf("round trip: %#v != %#v", d, got)
	}
}

func TestDecisionTextAndPathContextRoundTrip(t *testing.T) {
	text, blank := "Only customer web;\nexclude admin.", ""
	for _, a := range []*memory.Applicability{
		nil, {Text: &blank}, {Text: &text},
		{Kind: "context", AnyOf: []memory.ApplicabilitySelector{{Paths: []string{"apps/customer-web/**"}}}},
	} {
		d := &memory.Decision{Topic: "rendering", Decision: "Use SSR", CreatedAt: time.Now().UTC(), DecisionContext: memory.DecisionContext{Applicability: a}}
		got, err := ParseDecision(MarshalDecision(d))
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(d, got) {
			t.Fatalf("sharing lost metadata: %#v != %#v", d, got)
		}
	}
}
