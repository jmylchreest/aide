package store

import (
	"testing"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/memory"
)

// fakeGetter serves a fixed current revision per topic.
type fakeGetter struct {
	decisions map[string]*memory.Decision
}

func (f fakeGetter) GetDecision(topic string) (*memory.Decision, error) {
	d, ok := f.decisions[topic]
	if !ok {
		return nil, ErrNotFound
	}
	return d, nil
}

func TestResolvePrecedence(t *testing.T) {
	guardrail := &memory.Decision{
		Topic:      "existing-codebase-precedence",
		Precedence: memory.PrecedenceOverride,
		CreatedAt:  time.Now(),
	}
	ordinary := &memory.Decision{Topic: "python-testing", CreatedAt: time.Now()}
	g := fakeGetter{decisions: map[string]*memory.Decision{
		guardrail.Topic: guardrail,
		ordinary.Topic:  ordinary,
	}}

	explicitZero := 0
	explicitHigh := 200

	tests := []struct {
		name      string
		topic     string
		requested *int
		want      int
	}{
		{
			// The demotion hazard: rewording a guardrail from the CLI or the web
			// UI must not silently drop it out of the overriding block.
			name:      "omitted carries the current weight forward",
			topic:     guardrail.Topic,
			requested: nil,
			want:      memory.PrecedenceOverride,
		},
		{
			name:      "explicit zero demotes deliberately",
			topic:     guardrail.Topic,
			requested: &explicitZero,
			want:      0,
		},
		{
			name:      "explicit value wins over the current weight",
			topic:     ordinary.Topic,
			requested: &explicitHigh,
			want:      200,
		},
		{
			name:      "omitted on an ordinary decision stays default",
			topic:     ordinary.Topic,
			requested: nil,
			want:      memory.PrecedenceDefault,
		},
		{
			name:      "unknown topic defaults",
			topic:     "never-decided",
			requested: nil,
			want:      memory.PrecedenceDefault,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ResolvePrecedence(g, tt.topic, tt.requested); got != tt.want {
				t.Errorf("ResolvePrecedence() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestDecisionOverrides(t *testing.T) {
	tests := []struct {
		precedence int
		want       bool
	}{
		{precedence: 0, want: false},
		{precedence: 80, want: false},  // ordered above default, claims nothing
		{precedence: 99, want: false},  // just below the threshold
		{precedence: 100, want: true},  // the threshold itself
		{precedence: 200, want: true},  // above an existing guardrail
		{precedence: -10, want: false}, // deprioritised
	}
	for _, tt := range tests {
		d := &memory.Decision{Precedence: tt.precedence}
		if got := d.Overrides(); got != tt.want {
			t.Errorf("Precedence %d: Overrides() = %v, want %v", tt.precedence, got, tt.want)
		}
	}
}
