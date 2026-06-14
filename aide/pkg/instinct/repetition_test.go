package instinct

import (
	"testing"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/observe"
	"github.com/oklog/ulid/v2"
)

// bashEvents builds a sequence of Bash tool_call events, one per command,
// spaced 10s apart starting at a fixed base time.
func bashEvents(cmds ...string) []*observe.Event {
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	out := make([]*observe.Event, 0, len(cmds))
	for i, c := range cmds {
		out = append(out, &observe.Event{
			ID:        ulid.Make().String(),
			Timestamp: base.Add(time.Duration(i) * 10 * time.Second),
			Kind:      observe.KindToolCall,
			Name:      "Bash",
			SessionID: "t",
			Attrs:     map[string]string{"command": c},
		})
	}
	return out
}

func detectRepetition(events []*observe.Event) []Proposal {
	return Repetition{}.Detect(events, DefaultRepetitionConfig(), ParserContext{Now: time.Now()})
}

func TestNormaliseBash(t *testing.T) {
	cases := map[string]string{
		"":                        "",
		"  ":                      "",
		"sed -n '1,20p' a.go":     "sed -n '1,20p' a.go",
		"/usr/bin/sed -n 5p a.go": "sed -n 5p a.go",
		"FOO=bar BAZ=qux git log": "git log",
		"cat x | jq '.a'":         "cat x | jq '.a'",
		"cat   README.md":         "cat README.md", // whitespace collapsed
		"grep foo a 2>/dev/null":  "grep foo a",    // attached redirect dropped
		"grep foo a > out.txt":    "grep foo a",    // standalone redirect + target dropped
		"git status -s":           "git status -s",
		"--all":                   "--all", // degenerate, but stable
	}
	for in, want := range cases {
		if got := normaliseBash(in); got != want {
			t.Errorf("normaliseBash(%q) = %q, want %q", in, got, want)
		}
	}
}

// Five DIFFERENT sed invocations must NOT be reported as repetition — they are
// distinct lookups, not a repeated command. This is the core precision fix.
func TestRepetition_DistinctArgsDoNotGroup(t *testing.T) {
	events := bashEvents(
		"sed -n '1,20p' a.go",
		"sed -n '30,40p' b.go",
		"jq '.x' one.json",
		"jq '.y' two.json",
		"grep foo a",
	)
	if props := detectRepetition(events); len(props) != 0 {
		t.Fatalf("expected no proposals for 5 distinct commands, got %d: %+v", len(props), props)
	}
}

// The SAME command repeated past MinCount still fires.
func TestRepetition_IdenticalCommandFires(t *testing.T) {
	events := bashEvents(
		"cat README.md", "cat README.md", "cat README.md", "cat README.md",
	)
	props := detectRepetition(events)
	if len(props) != 1 {
		t.Fatalf("expected 1 proposal, got %d", len(props))
	}
	if props[0].Shape != ShapeRepetition {
		t.Errorf("shape = %q, want %q", props[0].Shape, ShapeRepetition)
	}
	if got := props[0].ProposedInstinct.Tags; got[len(got)-1] != "instinct_key:cat README.md" {
		t.Errorf("instinct_key tag = %v, want instinct_key:cat README.md", got)
	}
}

// A pipeline keeps every stage in its signature, so identical pipelines group
// but a bare first-stage command does not get lumped with them.
func TestRepetition_PipelineAware(t *testing.T) {
	events := bashEvents(
		"cat log | jq '.t'", "cat log | jq '.t'", "cat log | jq '.t'", "cat log | jq '.t'",
		"cat other", // different command — must not join the pipeline group
	)
	props := detectRepetition(events)
	if len(props) != 1 {
		t.Fatalf("expected 1 proposal (the repeated pipeline), got %d", len(props))
	}
	if want := "instinct_key:cat log | jq '.t'"; props[0].ProposedInstinct.Tags[3] != want {
		t.Errorf("instinct_key = %q, want %q", props[0].ProposedInstinct.Tags[3], want)
	}
}

// Ignored commands are matched as a prefix: `git status` and its flag variants
// are skipped, but `git log` is not.
func TestRepetition_IgnorePrefix(t *testing.T) {
	events := bashEvents(
		"git status", "git status -s", "git status", "git status --short",
		"git log", "git log", "git log", "git log",
	)
	props := detectRepetition(events)
	if len(props) != 1 {
		t.Fatalf("expected only the git-log group to fire, got %d proposals", len(props))
	}
	if want := "instinct_key:git log"; props[0].ProposedInstinct.Tags[3] != want {
		t.Errorf("instinct_key = %q, want %q", props[0].ProposedInstinct.Tags[3], want)
	}
}
