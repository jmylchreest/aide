package memory

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// Applicability is authored context, not an inferred filter or precedence rank.
// Missing applicability means unspecified. Selectors are alternatives; fields
// within a selector are conjunctive, including every listed tool.
type Applicability struct {
	Kind  string                  `json:"kind"`
	AnyOf []ApplicabilitySelector `json:"any_of,omitempty"`
}

type ApplicabilitySelector struct {
	Language  string   `json:"language,omitempty"`
	Framework string   `json:"framework,omitempty"`
	Tools     []string `json:"tools,omitempty"`
	Component string   `json:"component,omitempty"`
	Concern   string   `json:"concern,omitempty"`
}

// DecisionContext is replaceable as a unit. A nil update preserves it; an
// explicit empty context clears it without conflating unspecified with project-wide.
type DecisionContext struct {
	Applicability *Applicability `json:"applicability,omitempty"`
	Supersedes    []string       `json:"supersedes,omitempty"`
}

func (a *Applicability) Validate() error {
	if a == nil {
		return nil
	}
	switch a.Kind {
	case "project":
		if len(a.AnyOf) != 0 {
			return fmt.Errorf("project applicability cannot contain selectors")
		}
	case "context":
		if len(a.AnyOf) == 0 {
			return fmt.Errorf("context applicability requires any_of selectors")
		}
		for _, s := range a.AnyOf {
			if s.Language == "" && s.Framework == "" && len(s.Tools) == 0 && s.Component == "" && s.Concern == "" {
				return fmt.Errorf("empty applicability selector")
			}
			for _, v := range append([]string{s.Language, s.Framework, s.Component, s.Concern}, s.Tools...) {
				if strings.ContainsAny(v, "\r\n[]") || (v != "" && strings.TrimSpace(v) != v) {
					return fmt.Errorf("invalid applicability label %q", v)
				}
			}
			for _, tool := range s.Tools {
				if tool == "" {
					return fmt.Errorf("empty applicability tool")
				}
			}
		}
	default:
		return fmt.Errorf("unknown applicability kind %q", a.Kind)
	}
	return nil
}

func (c DecisionContext) Validate(topic string) error {
	if err := c.Applicability.Validate(); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, t := range c.Supersedes {
		if t == "" || strings.TrimSpace(t) != t || strings.ContainsAny(t, "\r\n") || t == topic || seen[t] {
			return fmt.Errorf("invalid supersedes topic %q", t)
		}
		seen[t] = true
	}
	if len(c.Supersedes) > 0 && c.Applicability == nil {
		return fmt.Errorf("supersession requires explicit applicability")
	}
	return nil
}

func (a *Applicability) Label() string {
	if a == nil {
		return "Applicability unspecified"
	}
	if a.Kind == "project" {
		return "Project-wide"
	}
	var alternatives []string
	for _, s := range a.AnyOf {
		var parts []string
		for _, v := range []string{s.Language, s.Framework} {
			if v != "" {
				parts = append(parts, v)
			}
		}
		parts = append(parts, s.Tools...)
		for _, v := range []string{s.Component, s.Concern} {
			if v != "" {
				parts = append(parts, v)
			}
		}
		alternatives = append(alternatives, strings.Join(parts, " + "))
	}
	return strings.Join(alternatives, " OR ")
}

// ValidateSupersession rejects cycles among the supplied decisions. External
// targets can be absent when a portable decision is imported independently.
func ValidateSupersession(decisions map[string]DecisionContext) error {
	visiting, done := map[string]bool{}, map[string]bool{}
	var visit func(string) error
	visit = func(topic string) error {
		if visiting[topic] {
			return fmt.Errorf("supersession cycle at %s", topic)
		}
		if done[topic] {
			return nil
		}
		visiting[topic] = true
		for _, target := range decisions[topic].Supersedes {
			if err := visit(target); err != nil {
				return err
			}
		}
		delete(visiting, topic)
		done[topic] = true
		return nil
	}
	for topic := range decisions {
		if err := visit(topic); err != nil {
			return err
		}
	}
	return nil
}

// Reject misspelled selector fields rather than silently broadening a rule.
func (a *Applicability) UnmarshalJSON(data []byte) error {
	type plain Applicability
	var parsed plain
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&parsed); err != nil {
		return err
	}
	*a = Applicability(parsed)
	return a.Validate()
}
