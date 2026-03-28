package handler

import "testing"

func TestTruncate(t *testing.T) {
	tests := []struct {
		name string
		s    string
		max  int
		want string
	}{
		{"short string", "hello", 10, "hello"},
		{"exact length", "hello", 5, "hello"},
		{"over limit", "hello world", 5, "hello..."},
		{"empty string", "", 5, ""},
		{"zero max", "hello", 0, "..."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncate(tt.s, tt.max)
			if got != tt.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tt.s, tt.max, got, tt.want)
			}
		})
	}
}

func TestContainsFold(t *testing.T) {
	tests := []struct {
		s, substr string
		want      bool
	}{
		{"Hello World", "hello", true},
		{"Hello World", "WORLD", true},
		{"Hello World", "LO WO", true},
		{"Hello World", "xyz", false},
		{"abc", "abcd", false}, // substr longer than s
		{"", "a", false},
		{"a", "", true}, // empty substr matches (contains() guards this)
		{"UPPERCASE", "uppercase", true},
		{"MiXeD", "mixed", true},
	}
	for _, tt := range tests {
		t.Run(tt.s+"_"+tt.substr, func(t *testing.T) {
			got := containsFold(tt.s, tt.substr)
			if got != tt.want {
				t.Errorf("containsFold(%q, %q) = %v, want %v", tt.s, tt.substr, got, tt.want)
			}
		})
	}
}

func TestContains(t *testing.T) {
	tests := []struct {
		haystack, needle string
		want             bool
	}{
		{"Hello World", "hello", true},
		{"Hello World", "", false},  // empty needle returns false
		{"", "hello", false},        // empty haystack returns false
		{"", "", false},             // both empty returns false
		{"foobar", "OBA", true},
	}
	for _, tt := range tests {
		t.Run(tt.haystack+"_"+tt.needle, func(t *testing.T) {
			got := contains(tt.haystack, tt.needle)
			if got != tt.want {
				t.Errorf("contains(%q, %q) = %v, want %v", tt.haystack, tt.needle, got, tt.want)
			}
		})
	}
}

func TestEqualFold(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		{"abc", "ABC", true},
		{"abc", "abc", true},
		{"ABC", "ABC", true},
		{"abc", "abd", false},
		{"abc", "ab", false},  // different lengths
		{"", "", true},
		{"Go", "go", true},
	}
	for _, tt := range tests {
		t.Run(tt.a+"_"+tt.b, func(t *testing.T) {
			got := equalFold(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("equalFold(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}
