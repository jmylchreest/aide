package contextshare

import (
	"github.com/jmylchreest/aide/aide/pkg/memory"
	"reflect"
	"testing"
	"time"
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
