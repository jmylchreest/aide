package findings

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/fsnotify/fsnotify"
	"github.com/jmylchreest/aide/aide/pkg/aideignore"
)

func BenchmarkRunnerChangedFiles(b *testing.B) {
	root := b.TempDir()
	files := make(map[string]fsnotify.Op)
	for i := 0; i < 8; i++ {
		path := filepath.Join(root, fmt.Sprintf("file%d.go", i))
		if err := os.WriteFile(path, []byte("package example\n// TODO: document this\nfunc example() {}\n"), 0o600); err != nil {
			b.Fatal(err)
		}
		files[path] = fsnotify.Write
	}
	output := runnerLog.Writer()
	runnerLog.SetOutput(io.Discard)
	defer runnerLog.SetOutput(output)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		r := NewRunner(&mockReplaceFindingsStore{}, AnalyzerConfig{
			ProjectRoot: root, Paths: []string{root}, Ignore: aideignore.NewEmpty(),
		}, nil)
		r.OnChanges(files)
		r.WaitAll()
		r.Stop()
	}
}
