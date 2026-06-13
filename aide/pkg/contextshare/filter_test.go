package contextshare

import (
	"testing"

	"github.com/jmylchreest/aide/aide/pkg/memory"
)

func TestGlobMatch(t *testing.T) {
	tests := []struct {
		pattern, s string
		want       bool
	}{
		{"*", "anything", true},
		{"*", "", true},
		{"scope:global", "scope:global", true},
		{"scope:global", "scope:local", false},
		{"team:*", "team:backend", true},
		{"team:*", "team:", true},
		{"team:*", "scope:global", false},
		{"session:*", "session:abc123", true},
		{"topic:auth-*", "topic:auth-strategy", true},
		{"topic:auth-*", "topic:auth", false},
		{"topic:auth-*", "topic:db-schema", false},
		// `*` must span the colons in a multi-segment token (no path.Match
		// special-casing of "/" or ":").
		{"decided_by:blueprint:*", "decided_by:blueprint:go@1.0", true},
		{"decided_by:blueprint:*", "decided_by:human", false},
		{"*global*", "scope:global", true},
		{"*:global", "scope:global", true},
		{"a*c", "abc", true},
		{"a*c", "ac", true},
		{"a*c", "abdc", true},
		{"a*c", "ab", false},
		// Case-sensitive.
		{"Scope:Global", "scope:global", false},
		// Literal match, no wildcard.
		{"x", "x", true},
		{"x", "xy", false},
		{"", "", true},
		{"", "x", false},
	}
	for _, tt := range tests {
		if got := globMatch(tt.pattern, tt.s); got != tt.want {
			t.Errorf("globMatch(%q, %q) = %v, want %v", tt.pattern, tt.s, got, tt.want)
		}
	}
}

func TestFilterMatch(t *testing.T) {
	tests := []struct {
		name   string
		filter Filter
		tokens []string
		want   bool
	}{
		{
			name:   "empty include matches all",
			filter: Filter{},
			tokens: []string{"category:gotcha"},
			want:   true,
		},
		{
			name:   "star include matches all",
			filter: Filter{Include: []string{"*"}},
			tokens: []string{"scope:global"},
			want:   true,
		},
		{
			name:   "include hit on custom user tag",
			filter: Filter{Include: []string{"team:*"}},
			tokens: []string{"team:backend", "category:pattern"},
			want:   true,
		},
		{
			name:   "include miss",
			filter: Filter{Include: []string{"team:*"}},
			tokens: []string{"scope:global", "category:pattern"},
			want:   false,
		},
		{
			name:   "exact category include",
			filter: Filter{Include: []string{"category:gotcha"}},
			tokens: []string{"project:foo", "category:gotcha"},
			want:   true,
		},
		{
			name:   "exclude rejects despite include all",
			filter: Filter{Exclude: []string{"scope:global"}},
			tokens: []string{"scope:global", "category:learning"},
			want:   false,
		},
		{
			name:   "exclude wins over explicit include",
			filter: Filter{Include: []string{"category:*"}, Exclude: []string{"scope:global"}},
			tokens: []string{"scope:global", "category:learning"},
			want:   false,
		},
		{
			name:   "session exclude rejects ephemeral memory",
			filter: Filter{Exclude: []string{"scope:global", "session:*"}},
			tokens: []string{"session:abc", "category:learning"},
			want:   false,
		},
		{
			name:   "project memory passes default memory exclude",
			filter: Filter{Exclude: []string{"scope:global", "session:*"}},
			tokens: []string{"project:foo", "category:learning"},
			want:   true,
		},
		{
			name:   "blueprint decision excluded by decided_by glob",
			filter: Filter{Exclude: []string{"decided_by:blueprint:*"}},
			tokens: []string{"topic:go-context", "decided_by:blueprint:go@1.0"},
			want:   false,
		},
		{
			name:   "human decision passes blueprint exclude",
			filter: Filter{Exclude: []string{"decided_by:blueprint:*"}},
			tokens: []string{"topic:auth", "decided_by:human"},
			want:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.filter.Match(tt.tokens); got != tt.want {
				t.Errorf("Match(%v) = %v, want %v", tt.tokens, got, tt.want)
			}
		})
	}
}

func TestFirstExcludeMatch(t *testing.T) {
	tests := []struct {
		name    string
		filter  Filter
		tokens  []string
		want    string
		wantHit bool
	}{
		{
			name:    "no exclude list, no hit",
			filter:  Filter{},
			tokens:  []string{"scope:global"},
			want:    "",
			wantHit: false,
		},
		{
			name:    "single exclude matches",
			filter:  Filter{Exclude: []string{"scope:global"}},
			tokens:  []string{"scope:global", "category:learning"},
			want:    "scope:global",
			wantHit: true,
		},
		{
			name:    "first matching pattern wins over later one",
			filter:  Filter{Exclude: []string{"scope:global", "session:*"}},
			tokens:  []string{"session:abc", "scope:global"},
			want:    "scope:global",
			wantHit: true,
		},
		{
			name:    "second pattern matches when first does not",
			filter:  Filter{Exclude: []string{"scope:global", "session:*"}},
			tokens:  []string{"project:foo", "session:abc"},
			want:    "session:*",
			wantHit: true,
		},
		{
			name:    "no exclude pattern matches",
			filter:  Filter{Exclude: []string{"scope:global", "session:*"}},
			tokens:  []string{"project:foo", "category:pattern"},
			want:    "",
			wantHit: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, hit := tt.filter.FirstExcludeMatch(tt.tokens)
			if got != tt.want || hit != tt.wantHit {
				t.Errorf("FirstExcludeMatch(%v) = (%q, %v), want (%q, %v)", tt.tokens, got, hit, tt.want, tt.wantHit)
			}
		})
	}
}

func TestMemoryTokens(t *testing.T) {
	m := &memory.Memory{
		Category: memory.Category("gotcha"),
		Tags:     []string{"team:backend", "scope:global", "source:user"},
	}
	got := MemoryTokens(m)
	want := []string{"team:backend", "scope:global", "source:user", "category:gotcha"}
	if len(got) != len(want) {
		t.Fatalf("MemoryTokens len = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("MemoryTokens[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestDecisionTokens(t *testing.T) {
	d := &memory.Decision{Topic: "auth-strategy", DecidedBy: "blueprint:go@1.0"}
	got := DecisionTokens(d)
	want := []string{"topic:auth-strategy", "decided_by:blueprint:go@1.0"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("DecisionTokens[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
