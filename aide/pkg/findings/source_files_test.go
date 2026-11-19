package findings

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/jmylchreest/aide/aide/pkg/aideignore"
)

func TestTextAnalyzersFileSelection(t *testing.T) {
	root := t.TempDir()
	for name, content := range map[string]string{
		"nested/keep.go": "package demo\n// TODO: inspect\n",
		"large.go":       strings.Repeat("x", 129),
		"ignored.go":     "package ignored\n",
		"notes.unknown":  "unsupported extension",
		".git/hidden.go": "package hidden\n",
		".aideignore":    "ignored.go\n",
	} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ignore, err := aideignore.New(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, analyzer := range []string{"security", "todos"} {
		t.Run(analyzer, func(t *testing.T) {
			var progress []string
			progressCount := 0
			onProgress := func(path string, count int) {
				progress = append(progress, path)
				progressCount += count
			}
			paths := []string{root, filepath.Join(root, "missing.go")}
			var analyzed, skipped, count int
			var findings []*Finding
			if analyzer == "security" {
				ff, result, err := AnalyzeSecurity(SecurityConfig{
					Paths: paths, ProjectRoot: root, Ignore: ignore, MaxFileSize: 128, ProgressFn: onProgress,
				})
				if err != nil {
					t.Fatal(err)
				}
				findings, analyzed, skipped, count = ff, result.FilesAnalyzed, result.FilesSkipped, result.FindingsCount
			} else {
				ff, result, err := AnalyzeTodos(TodosConfig{
					Paths: paths, ProjectRoot: root, Ignore: ignore, MaxFileSize: 128, ProgressFn: onProgress,
				})
				if err != nil {
					t.Fatal(err)
				}
				findings, analyzed, skipped, count = ff, result.FilesAnalyzed, result.FilesSkipped, result.FindingsCount
			}
			if analyzed != 1 || skipped != 1 {
				t.Fatalf("analyzed=%d skipped=%d, want one of each", analyzed, skipped)
			}
			if !reflect.DeepEqual(progress, []string{filepath.Join("nested", "keep.go")}) {
				t.Errorf("progress should include only the readable eligible file, got %v", progress)
			}
			if progressCount != count || count != len(findings) {
				t.Errorf("inconsistent finding counts: progress=%d summary=%d findings=%d", progressCount, count, len(findings))
			}
			if analyzer == "todos" && count != 1 {
				t.Errorf("wanted the TODO from the selected file, got %d findings", count)
			}
		})
	}
}
