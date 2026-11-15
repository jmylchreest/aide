package memory

import (
	"encoding/json"
	"reflect"
	"testing"
)

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
	if got := (*Applicability)(nil).Label(); got != "" {
		t.Fatal(got)
	}
}

func TestApplicabilityJSONForms(t *testing.T) {
	for _, tc := range []struct {
		input, label string
	}{
		{`{}`, ""},
		{`{"applicability":null}`, ""},
		{`{"applicability":""}`, ""},
		{`{"applicability":" \n "}`, ""},
		{`{"applicability":"Only customer web;\nexclude admin."}`, "Only customer web; exclude admin."},
		{`{"applicability":{"kind":"project"}}`, "Project-wide"},
		{`{"applicability":{"kind":"context","any_of":[{"language":"go","paths":["services/billing/**","services/payments/**"]},{"paths":["apps/admin/**"]}]}}`, "go + paths: (services/billing/** OR services/payments/**) OR paths: apps/admin/**"},
	} {
		t.Run(tc.input, func(t *testing.T) {
			var c DecisionContext
			if err := json.Unmarshal([]byte(tc.input), &c); err != nil {
				t.Fatal(err)
			}
			if err := c.Validate("example"); err != nil {
				t.Fatal(err)
			}
			if got := c.Applicability.Label(); got != tc.label {
				t.Fatalf("label = %q, want %q", got, tc.label)
			}
			wantPrefix := ""
			if tc.label != "" {
				wantPrefix = "[" + tc.label + "] "
			}
			if got := c.Applicability.Prefix(); got != wantPrefix {
				t.Fatalf("prefix = %q, want %q", got, wantPrefix)
			}
			data, err := json.Marshal(c)
			if err != nil {
				t.Fatal(err)
			}
			var restored DecisionContext
			if err := json.Unmarshal(data, &restored); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(c, restored) {
				t.Fatalf("JSON round trip lost metadata: %s", data)
			}
		})
	}
}

func TestApplicabilityRejectsInvalidJSON(t *testing.T) {
	for _, input := range []string{
		`true`, `42`, `[]`, `{}`, `{"kind":"text","text":"Only admin"}`,
		`{"kind":"context","any_of":[{"path":["apps/admin/**"]}]}`,
	} {
		var a Applicability
		if err := json.Unmarshal([]byte(input), &a); err == nil {
			t.Errorf("accepted %s", input)
		}
	}
}

func TestApplicabilityPaths(t *testing.T) {
	for _, path := range []string{"", " ", "/apps/**", "../apps/**", "apps/../../**", `C:\apps\**`, `apps\admin\**`, "apps/**\n"} {
		a := &Applicability{Kind: "context", AnyOf: []ApplicabilitySelector{{Paths: []string{path}}}}
		if err := a.Validate(); err == nil {
			t.Errorf("accepted invalid path %q", path)
		}
	}
	// Character classes and spaces are useful guidance, not a runtime glob dialect.
	a := &Applicability{Kind: "context", AnyOf: []ApplicabilitySelector{{Paths: []string{"apps/[ab]*/**", "apps/customer web/**"}}}}
	if err := a.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestSupersessionValidation(t *testing.T) {
	a := &Applicability{Kind: "project"}
	if err := (DecisionContext{Supersedes: []string{"x"}}).Validate("y"); err == nil {
		t.Fatal("unspecified override")
	}
	blank := " \n "
	if err := (DecisionContext{Applicability: &Applicability{Text: &blank}, Supersedes: []string{"x"}}).Validate("y"); err == nil {
		t.Fatal("blank override accepted")
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
