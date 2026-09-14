package contextshare

import (
	"reflect"
	"testing"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/memory"
)

func TestDecisionCheckoutRoundTrip(t *testing.T) {
	d := &memory.Decision{CreatedAt: time.Now(), Topic: "storage", Decision: "isolate generated stores", Checkout: &memory.CheckoutProvenance{ID: "stable-checkout", Branch: "feature/stores", Commit: "abc123"}}
	got, err := ParseDecision(MarshalDecision(d))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Checkout, d.Checkout) {
		t.Fatalf("lost provenance: %+v", got.Checkout)
	}
}
