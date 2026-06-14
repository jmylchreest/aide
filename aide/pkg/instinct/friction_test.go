package instinct

import (
	"testing"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/observe"
	"github.com/oklog/ulid/v2"
)

// failingBash builds Bash tool_call events that each carry an Error, spaced 20s
// apart. cmds[i] pairs with errs[i].
func failingBash(t *testing.T, pairs ...[2]string) []*observe.Event {
	t.Helper()
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	out := make([]*observe.Event, 0, len(pairs))
	for i, p := range pairs {
		out = append(out, &observe.Event{
			ID:        ulid.Make().String(),
			Timestamp: base.Add(time.Duration(i) * 20 * time.Second),
			Kind:      observe.KindToolCall,
			Name:      "Bash",
			SessionID: "t",
			Attrs:     map[string]string{"command": p[0]},
			Error:     p[1],
		})
	}
	return out
}

func detectFriction(events []*observe.Event) []Proposal {
	return Friction{}.Detect(events, DefaultFrictionConfig(), ParserContext{Now: time.Now()})
}

// The same command failing repeatedly is friction.
func TestFriction_RepeatedFailureFires(t *testing.T) {
	events := failingBash(t,
		[2]string{"go test ./...", "exit status 1"},
		[2]string{"go test ./...", "exit status 1"},
	)
	props := detectFriction(events)
	if len(props) != 1 {
		t.Fatalf("expected 1 proposal, got %d", len(props))
	}
	if props[0].Shape != ShapeFriction {
		t.Errorf("shape = %q, want %q", props[0].Shape, ShapeFriction)
	}
	if want := "instinct_key:friction:Bash `go test ./...`"; props[0].ProposedInstinct.Tags[3] != want {
		t.Errorf("instinct_key = %q, want %q", props[0].ProposedInstinct.Tags[3], want)
	}
}

// Successful calls carry no Error and are ignored entirely.
func TestFriction_SuccessIgnored(t *testing.T) {
	events := failingBash(t,
		[2]string{"go test ./...", ""},
		[2]string{"go test ./...", ""},
		[2]string{"go test ./...", ""},
	)
	if props := detectFriction(events); len(props) != 0 {
		t.Fatalf("expected no proposals when nothing failed, got %d", len(props))
	}
}

// Different failing commands don't cluster — a single failure each is not
// recurring friction.
func TestFriction_DistinctFailuresDoNotGroup(t *testing.T) {
	events := failingBash(t,
		[2]string{"go build ./...", "err a"},
		[2]string{"npm run lint", "err b"},
	)
	if props := detectFriction(events); len(props) != 0 {
		t.Fatalf("expected no proposals for two distinct single failures, got %d", len(props))
	}
}

// Friction is an LLM-tier detector: it must be skipped by the deterministic
// runner and run by the LLM runner.
func TestFriction_RequiresLLM(t *testing.T) {
	if !(Friction{}).Capabilities().RequiresLLM {
		t.Fatal("Friction should declare RequiresLLM=true")
	}
	events := failingBash(t,
		[2]string{"go test ./...", "boom"},
		[2]string{"go test ./...", "boom"},
	)
	runner := NewRunner(Friction{})

	if got := runner.Run("s", events, nil, nil, RunOpts{Mode: RunDeterministic}); len(got) != 0 {
		t.Errorf("deterministic mode should skip friction, got %d proposals", len(got))
	}
	if got := runner.Run("s", events, nil, nil, RunOpts{Mode: RunWithLLM}); len(got) != 1 {
		t.Errorf("LLM mode should run friction, got %d proposals", len(got))
	}
}

// File-tool failures group by tool + path, not command.
func TestFriction_FileToolSignature(t *testing.T) {
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	mk := func(i int) *observe.Event {
		return &observe.Event{
			ID:        ulid.Make().String(),
			Timestamp: base.Add(time.Duration(i) * time.Minute),
			Kind:      observe.KindToolCall,
			Name:      "Edit",
			FilePath:  "src/api/users.ts",
			SessionID: "t",
			Error:     "String to replace not found",
		}
	}
	props := detectFriction([]*observe.Event{mk(0), mk(1)})
	if len(props) != 1 {
		t.Fatalf("expected 1 proposal, got %d", len(props))
	}
	if want := "instinct_key:friction:Edit .../users.ts"; props[0].ProposedInstinct.Tags[3] != want {
		t.Errorf("instinct_key = %q, want %q", props[0].ProposedInstinct.Tags[3], want)
	}
}
