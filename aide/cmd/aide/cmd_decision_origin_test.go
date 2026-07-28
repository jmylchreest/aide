package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseOriginFlag(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    string
		wantErr bool
	}{
		{name: "absent means local only", args: []string{"--format=json"}, want: ""},
		{name: "equals form", args: []string{"--origin=parent"}, want: "parent"},
		{name: "space form", args: []string{"--origin", "peer"}, want: "peer"},
		{name: "all", args: []string{"--origin=all"}, want: "all"},
		{name: "mixed with other flags", args: []string{"--format=json", "--origin", "all"}, want: "all"},
		{name: "unknown value rejected", args: []string{"--origin=cousin"}, wantErr: true},
		{name: "empty equals form rejected", args: []string{"--origin="}, want: ""},
		{name: "missing value rejected", args: []string{"--origin"}, wantErr: true},
		// "scope" and "inherited" are deliberately not accepted: the
		// terminology-axes decision reserves one word per axis.
		{name: "scope is not an origin", args: []string{"--origin=scope"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseOriginFlag(tt.args)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseOriginFlag(%q) = %q, want error", tt.args, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseOriginFlag(%q) unexpected error: %v", tt.args, err)
			}
			if got != tt.want {
				t.Fatalf("parseOriginFlag(%q) = %q, want %q", tt.args, got, tt.want)
			}
		})
	}
}

// listDecisionsJSON runs decisionList against the child store and decodes
// the JSON output.
func listDecisionsJSON(t *testing.T, childRoot string, args ...string) []SessionDecision {
	t.Helper()
	b, err := NewBackend(computeDBPath(childRoot))
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	out := captureStdout(t, func() {
		if err := decisionList(b, append([]string{"--format=json"}, args...)); err != nil {
			t.Errorf("decisionList: %v", err)
		}
	})
	var got []SessionDecision
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("decode %q: %v", out, err)
	}
	return got
}

// Default stays local-only: adding --origin must not change what existing
// callers see.
func TestDecisionListDefaultsToLocalOnly(t *testing.T) {
	_, childRoot := cascadeFixture(t)

	got := listDecisionsJSON(t, childRoot)

	if d := decisionByTopic(got, "parent-only"); d != nil {
		t.Errorf("parent-only leaked into the default listing: %+v", d)
	}
	if d := decisionByTopic(got, "child-only"); d == nil || d.Value != "local-rule" {
		t.Errorf("child-only = %+v, want local-rule", d)
	}
	for _, d := range got {
		if d.OriginKind != "" {
			t.Errorf("%s carries origin %q in a local-only listing", d.Topic, d.OriginKind)
		}
	}
}

func TestDecisionListOriginParentCascades(t *testing.T) {
	parentRoot, childRoot := cascadeFixture(t)

	got := listDecisionsJSON(t, childRoot, "--origin=parent")

	// The ancestor-only topic appears, stamped with its provenance.
	d := decisionByTopic(got, "parent-only")
	if d == nil {
		t.Fatalf("parent-only missing from --origin=parent listing: %+v", got)
	}
	if d.Value != "estate-rule" {
		t.Errorf("parent-only = %q, want estate-rule", d.Value)
	}
	if d.OriginKind != "parent" {
		t.Errorf("parent-only OriginKind = %q, want parent", d.OriginKind)
	}
	if d.Origin != parentRoot {
		t.Errorf("parent-only Origin = %q, want %q", d.Origin, parentRoot)
	}

	// Nearer wins: the child's own decision must not be shadowed by the
	// ancestor's, and the topic must appear exactly once.
	shared := decisionByTopic(got, "shared-topic")
	if shared == nil || shared.Value != "child-override" {
		t.Fatalf("shared-topic = %+v, want child-override", shared)
	}
	if shared.OriginKind != "" {
		t.Errorf("shared-topic OriginKind = %q, want local", shared.OriginKind)
	}
	var n int
	for _, x := range got {
		if x.Topic == "shared-topic" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("shared-topic appears %d times, want 1", n)
	}
}

// The kill switch must silence the cascade even when explicitly requested,
// so a broken estate can never wedge the command.
func TestDecisionListOriginRespectsCascadeKillSwitch(t *testing.T) {
	_, childRoot := cascadeFixture(t)
	t.Setenv("AIDE_CASCADE_DISABLED", "1")

	got := listDecisionsJSON(t, childRoot, "--origin=all")

	if d := decisionByTopic(got, "parent-only"); d != nil {
		t.Errorf("cascade ran despite AIDE_CASCADE_DISABLED: %+v", d)
	}
}

func TestDecisionListRejectsUnknownOrigin(t *testing.T) {
	_, childRoot := cascadeFixture(t)
	b, err := NewBackend(computeDBPath(childRoot))
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	err = decisionList(b, []string{"--origin=elsewhere"})
	if err == nil {
		t.Fatal("decisionList accepted an unknown --origin value")
	}
	if !strings.Contains(err.Error(), "parent, peer, or all") {
		t.Errorf("error %q does not name the valid values", err)
	}
}

// The ORIGIN column appears only when a non-local row is present, so the
// default table keeps its existing shape.
func TestDecisionListOriginColumnOnlyWhenNeeded(t *testing.T) {
	_, childRoot := cascadeFixture(t)
	b, err := NewBackend(computeDBPath(childRoot))
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	local := captureStdout(t, func() {
		if err := decisionList(b, nil); err != nil {
			t.Errorf("decisionList: %v", err)
		}
	})
	if strings.Contains(local, "ORIGIN") {
		t.Errorf("local-only listing grew an ORIGIN column:\n%s", local)
	}

	estate := captureStdout(t, func() {
		if err := decisionList(b, []string{"--origin=parent"}); err != nil {
			t.Errorf("decisionList: %v", err)
		}
	})
	if !strings.Contains(estate, "ORIGIN") {
		t.Errorf("cascaded listing lacks an ORIGIN column:\n%s", estate)
	}
	if !strings.Contains(estate, "parent:") {
		t.Errorf("cascaded listing lacks a parent: label:\n%s", estate)
	}
}
