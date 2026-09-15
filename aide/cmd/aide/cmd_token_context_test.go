package main

import (
	"strings"
	"testing"

	"github.com/jmylchreest/aide/aide/pkg/observe"
)

func TestPreparedContextCLIShowsMeasuredSourceBoundaryInDetails(t *testing.T) {
	dbPath, _ := newShareProject(t)
	withBackend(t, dbPath, func(b *Backend) {
		if err := b.Store().AddObserveEvent(&observe.Event{Kind: observe.KindInjection, Name: "skill", Category: "inject", SessionID: "s", Attrs: map[string]string{"accounting_version": "1", "observation_stage": "aide_context", "payload_bytes": "6"}}); err != nil {
			t.Fatal(err)
		}
	})
	out := captureStdout(t, func() {
		if err := cmdTokenStats(dbPath, []string{"--details", "--session=s"}); err != nil {
			t.Fatal(err)
		}
	})
	for _, want := range []string{"prepared_aide_context: 6 bytes; ~2 tokens; 1 observations", "not full prompt usage or confirmed delivery"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q: %s", want, out)
		}
	}
}
