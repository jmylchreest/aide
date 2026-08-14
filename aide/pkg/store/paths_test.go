package store

import "testing"

func TestProjectRootFromDB(t *testing.T) {
	// dbPath is <root>/.aide/memory/memory.db — three Dir() calls to reach <root>.
	tests := []struct {
		name   string
		dbPath string
		want   string
	}{
		{"standard layout", "/home/user/myproject/.aide/memory/memory.db", "/home/user/myproject"},
		{"nested project", "/a/b/c/project/.aide/memory/memory.db", "/a/b/c/project"},
		{"root-level project", "/project/.aide/memory/memory.db", "/project"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ProjectRootFromDB(tt.dbPath); got != tt.want {
				t.Errorf("ProjectRootFromDB(%q) = %q, want %q", tt.dbPath, got, tt.want)
			}
		})
	}
}
