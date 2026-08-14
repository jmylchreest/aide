package memory

import "testing"

func TestHasTag(t *testing.T) {
	tags := []string{"scope:global", "source:user", "auth"}

	for _, want := range tags {
		if !HasTag(tags, want) {
			t.Errorf("HasTag(%q) = false, want true", want)
		}
	}
	if HasTag(tags, "scope") {
		t.Error("HasTag matched a prefix; it must compare whole tags")
	}
	if HasTag(nil, "anything") {
		t.Error("HasTag on nil tags = true, want false")
	}
}

func TestHasAnyTag(t *testing.T) {
	tags := []string{"project:myapp", "source:discovered"}

	tests := []struct {
		name string
		want []string
		ok   bool
	}{
		{"single match", []string{"project:myapp"}, true},
		{"one of several", []string{"absent", "source:discovered"}, true},
		{"no match", []string{"scope:global", "partial"}, false},
		{"empty wanted", nil, false},
		{"prefix is not a match", []string{"project"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HasAnyTag(tags, tt.want); got != tt.ok {
				t.Errorf("HasAnyTag(%v, %v) = %v, want %v", tags, tt.want, got, tt.ok)
			}
		})
	}

	if HasAnyTag(nil, []string{"x"}) {
		t.Error("HasAnyTag on nil tags = true, want false")
	}
}
