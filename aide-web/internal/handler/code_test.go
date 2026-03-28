package handler

import (
	"path/filepath"
	"testing"
)

func TestLangFromExt(t *testing.T) {
	tests := []struct {
		ext  string
		want string
	}{
		{".go", "go"},
		{".ts", "typescript"},
		{".tsx", "typescript"},
		{".js", "javascript"},
		{".jsx", "javascript"},
		{".mjs", "javascript"},
		{".cjs", "javascript"},
		{".rs", "rust"},
		{".py", "python"},
		{".rb", "ruby"},
		{".java", "java"},
		{".c", "c"},
		{".h", "c"},
		{".cpp", "cpp"},
		{".cc", "cpp"},
		{".cxx", "cpp"},
		{".hpp", "cpp"},
		{".css", "css"},
		{".html", "html"},
		{".htm", "html"},
		{".json", "json"},
		{".yaml", "yaml"},
		{".yml", "yaml"},
		{".toml", "toml"},
		{".md", "markdown"},
		{".sh", "shell"},
		{".bash", "shell"},
		{".zsh", "shell"},
		{".sql", "sql"},
		{".proto", "protobuf"},
		{".unknown", "text"},
		{"", "text"},
		{".GO", "go"},   // case insensitive
		{".Py", "python"},
	}
	for _, tt := range tests {
		t.Run(tt.ext, func(t *testing.T) {
			got := langFromExt(tt.ext)
			if got != tt.want {
				t.Errorf("langFromExt(%q) = %q, want %q", tt.ext, got, tt.want)
			}
		})
	}
}

func TestValidatePath(t *testing.T) {
	root := "/project/root"

	tests := []struct {
		name    string
		path    string
		wantAbs string
		wantErr bool
	}{
		// Valid paths
		{
			name:    "simple file",
			path:    "main.go",
			wantAbs: filepath.Join(root, "main.go"),
		},
		{
			name:    "nested file",
			path:    "pkg/handler/code.go",
			wantAbs: filepath.Join(root, "pkg/handler/code.go"),
		},
		{
			name:    "with redundant slashes",
			path:    "pkg//handler///code.go",
			wantAbs: filepath.Join(root, "pkg/handler/code.go"),
		},
		{
			name:    "with inner dot-dot that resolves within root",
			path:    "pkg/handler/../handler/code.go",
			wantAbs: filepath.Join(root, "pkg/handler/code.go"),
		},

		// Path traversal attacks
		{
			name:    "absolute path",
			path:    "/etc/passwd",
			wantErr: true,
		},
		{
			name:    "leading dot-dot",
			path:    "../../../etc/passwd",
			wantErr: true,
		},
		{
			name:    "dot-dot only",
			path:    "..",
			wantErr: true,
		},
		{
			name:    "dot-dot with trailing slash",
			path:    "../",
			wantErr: true,
		},
		{
			name:    "encoded traversal that resolves to dot-dot prefix",
			path:    "../../etc/shadow",
			wantErr: true,
		},
		{
			name:    "sneaky dot-dot after clean",
			path:    "foo/../../..",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validatePath(root, tt.path)
			if tt.wantErr {
				if err == nil {
					t.Errorf("validatePath(%q, %q) = %q, want error", root, tt.path, got)
				}
				return
			}
			if err != nil {
				t.Errorf("validatePath(%q, %q) unexpected error: %v", root, tt.path, err)
				return
			}
			if got != tt.wantAbs {
				t.Errorf("validatePath(%q, %q) = %q, want %q", root, tt.path, got, tt.wantAbs)
			}
		})
	}
}
