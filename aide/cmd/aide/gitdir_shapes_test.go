package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// gitdirShapeCase mirrors testdata/gitdir-shapes.json — the golden table
// shared with src/test/gitdir-shapes.test.ts so the Go resolver and the TS
// mirror cannot drift on gitdir classification.
type gitdirShapeCase struct {
	Name         string   `json:"name"`
	GitdirDirs   []string `json:"gitdirDirs"`
	GitdirTarget *string  `json:"gitdirTarget"`
	CwdSuffix    string   `json:"cwdSuffix"`
	ExpectAnchor string   `json:"expectAnchor"`
	ExpectShape  string   `json:"expectShape"`
}

func loadGitdirShapeCases(t *testing.T) []gitdirShapeCase {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "gitdir-shapes.json"))
	if err != nil {
		t.Fatalf("golden table: %v", err)
	}
	var table struct {
		Cases []gitdirShapeCase `json:"cases"`
	}
	if err := json.Unmarshal(data, &table); err != nil {
		t.Fatalf("golden table: %v", err)
	}
	if len(table.Cases) == 0 {
		t.Fatal("golden table is empty")
	}
	return table.Cases
}

func TestGitdirShapesGolden(t *testing.T) {
	t.Setenv("AIDE_PROJECT_ROOT", "")
	os.Unsetenv("AIDE_PROJECT_ROOT")

	for _, tc := range loadGitdirShapeCases(t) {
		t.Run(tc.Name, func(t *testing.T) {
			tmp, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			super := filepath.Join(tmp, "super")
			for _, d := range tc.GitdirDirs {
				if err := os.MkdirAll(filepath.Join(super, filepath.FromSlash(d)), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			checkout := filepath.Join(tmp, "checkout")
			cwd := checkout
			if tc.CwdSuffix != "" {
				cwd = filepath.Join(checkout, filepath.FromSlash(tc.CwdSuffix))
			}
			if err := os.MkdirAll(cwd, 0o755); err != nil {
				t.Fatal(err)
			}

			gitPath := filepath.Join(checkout, ".git")
			switch {
			case tc.GitdirTarget == nil:
				if err := os.MkdirAll(gitPath, 0o755); err != nil {
					t.Fatal(err)
				}
			case *tc.GitdirTarget == "GARBAGE":
				if err := os.WriteFile(gitPath, []byte("not a gitdir pointer\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			default:
				content := "gitdir: " + filepath.Join(super, filepath.FromSlash(*tc.GitdirTarget)) + "\n"
				if err := os.WriteFile(gitPath, []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
			}

			a := resolveAnchor(cwd)

			wantRoot := checkout
			if tc.ExpectAnchor == "super" {
				wantRoot = super
			}
			if a.Root != wantRoot {
				t.Errorf("anchor = %q, want %s (%q)", a.Root, tc.ExpectAnchor, wantRoot)
			}
			if a.Provenance.GitdirShape != tc.ExpectShape {
				t.Errorf("gitdirShape = %q, want %q", a.Provenance.GitdirShape, tc.ExpectShape)
			}
		})
	}
}
