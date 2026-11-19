package findings

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/jmylchreest/aide/aide/pkg/aideignore"
	"github.com/jmylchreest/aide/aide/pkg/code"
)

// walkSourceFiles shares the standalone text analyzers' file eligibility policy.
// Unreadable entries are ignored; only oversized source files count as skipped.
func walkSourceFiles(paths []string, ignore *aideignore.Matcher, maxFileSize int64, visit func(string, []byte)) (analyzed, skipped int, err error) {
	for _, root := range paths {
		absRoot, err := filepath.Abs(root)
		if err != nil {
			return analyzed, skipped, fmt.Errorf("abs path %s: %w", root, err)
		}
		shouldSkip := ignore.WalkFunc(absRoot)
		err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if skip, skipDir := shouldSkip(path, info); skip {
				if skipDir {
					return filepath.SkipDir
				}
				return nil
			}
			if info.IsDir() || !code.SupportedFile(path) {
				return nil
			}
			if info.Size() > maxFileSize {
				skipped++
				return nil
			}
			content, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			visit(path, content)
			analyzed++
			return nil
		})
		if err != nil {
			return analyzed, skipped, fmt.Errorf("walk %s: %w", root, err)
		}
	}
	return analyzed, skipped, nil
}
