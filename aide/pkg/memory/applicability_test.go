package memory

import "testing"

func TestApplicabilityValidation(t *testing.T) {
	for _, a := range []*Applicability{
		{Kind: "unknown"}, {Kind: "context"}, {Kind: "context", AnyOf: []ApplicabilitySelector{{}}},
		{Kind: "project", AnyOf: []ApplicabilitySelector{{Language: "go"}}},
		{Kind: "context", AnyOf: []ApplicabilitySelector{{Tools: []string{""}}}},
	} {
		if err := a.Validate(); err == nil {
			t.Errorf("accepted %#v", a)
		}
	}
	a := &Applicability{Kind: "context", AnyOf: []ApplicabilitySelector{{Language: "python", Framework: "django"}, {Tools: []string{"opentofu", "github-actions"}}}}
	if err := a.Validate(); err != nil {
		t.Fatal(err)
	}
	if got := a.Label(); got != "python + django OR opentofu + github-actions" {
		t.Fatal(got)
	}
	if got := (*Applicability)(nil).Label(); got != "Applicability unspecified" {
		t.Fatal(got)
	}
}

func TestSupersessionValidation(t *testing.T) {
	a := &Applicability{Kind: "project"}
	if err := (DecisionContext{Supersedes: []string{"x"}}).Validate("y"); err == nil {
		t.Fatal("unspecified override")
	}
	if err := (DecisionContext{Applicability: a, Supersedes: []string{"x"}}).Validate("x"); err == nil {
		t.Fatal("self override")
	}
	graph := map[string]DecisionContext{"a": {Applicability: a, Supersedes: []string{"b"}}, "b": {Applicability: a, Supersedes: []string{"a"}}}
	if err := ValidateSupersession(graph); err == nil {
		t.Fatal("cycle accepted")
	}
	delete(graph, "b")
	if err := ValidateSupersession(graph); err != nil {
		t.Fatalf("portable external target: %v", err)
	}
}
