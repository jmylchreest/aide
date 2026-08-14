package main

import (
	"testing"

	"github.com/jmylchreest/aide/aide/pkg/observe"
)

func mutation(file string) *observe.Event {
	return &observe.Event{Kind: observe.KindToolCall, Name: "Edit", FilePath: file}
}

func prompt(text string) *observe.Event {
	return &observe.Event{
		Kind:  observe.KindHook,
		Name:  "user_prompt",
		Attrs: map[string]string{"text": text},
	}
}

func TestNearestMutation(t *testing.T) {
	// index:      0         1        2         3        4
	events := []*observe.Event{
		mutation("before.go"),
		prompt("noise"),
		prompt("target"),
		prompt("noise"),
		mutation("after.go"),
	}

	if got := nearestMutation(events, 2, -1); got == nil || got.FilePath != "before.go" {
		t.Errorf("backwards scan: got %v, want before.go", got)
	}
	if got := nearestMutation(events, 2, 1); got == nil || got.FilePath != "after.go" {
		t.Errorf("forwards scan: got %v, want after.go", got)
	}
}

func TestNearestMutationRespectsWindow(t *testing.T) {
	// A mutation exactly at the window edge counts; one step beyond does not.
	atEdge := make([]*observe.Event, 0, mutationWindow+1)
	atEdge = append(atEdge, mutation("edge.go"))
	for i := 0; i < mutationWindow; i++ {
		atEdge = append(atEdge, prompt("filler"))
	}
	if got := nearestMutation(atEdge, mutationWindow, -1); got == nil {
		t.Errorf("mutation %d back should be inside the window", mutationWindow)
	}

	beyond := append([]*observe.Event{mutation("far.go")}, atEdge[1:]...)
	beyond = append(beyond, prompt("filler"))
	if got := nearestMutation(beyond, mutationWindow+1, -1); got != nil {
		t.Errorf("mutation %d back should be outside the window, got %v", mutationWindow+1, got)
	}
}

func TestNearestMutationBounds(t *testing.T) {
	events := []*observe.Event{prompt("only")}
	if got := nearestMutation(events, 0, -1); got != nil {
		t.Errorf("scanning before the first event should be nil, got %v", got)
	}
	if got := nearestMutation(events, 0, 1); got != nil {
		t.Errorf("scanning past the last event should be nil, got %v", got)
	}
}

func TestNearestMutationIgnoresNonMutations(t *testing.T) {
	events := []*observe.Event{
		{Kind: observe.KindToolCall, Name: "Read", FilePath: "read.go"},
		prompt("target"),
	}
	if got := nearestMutation(events, 1, -1); got != nil {
		t.Errorf("Read is not a mutation, got %v", got)
	}
}
