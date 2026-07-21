package survey

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/aideignore"
	"github.com/jmylchreest/aide/aide/pkg/anchor"
)

// subprojectMaxDepth bounds the child-scope walk; nested project roots
// deeper than this are their own children's concern.
const subprojectMaxDepth = 6

// discoverSubprojects walks below rootDir for child project scopes — nested
// VCS roots and submodule checkouts — and emits one KindSubproject entry
// per child: the downward half of the estate map (the anchor chain is the
// upward half). Entries record identity and topology only, never liveness
// (whether a child's daemon runs is a read-time question). Classification
// reuses pkg/anchor — no new resolver logic here.
//
// The walk does not descend into discovered children (their internals
// belong to their own surveys) and honours .aideignore pruning — a
// submodule under an ignored dir (vendor/, node_modules/) is a dependency
// checkout, not an estate member.
func discoverSubprojects(rootDir string) []*Entry {
	ignore, _ := aideignore.New(rootDir)
	if ignore == nil {
		ignore = aideignore.NewFromDefaults()
	}

	var entries []*Entry
	_ = filepath.WalkDir(rootDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		if path == rootDir {
			return nil
		}
		rel, relErr := filepath.Rel(rootDir, path)
		if relErr != nil {
			return nil
		}
		if strings.Count(rel, string(filepath.Separator)) >= subprojectMaxDepth {
			return filepath.SkipDir
		}
		base := filepath.Base(path)
		if strings.HasPrefix(base, ".") || ignore.ShouldIgnoreDir(rel) {
			return filepath.SkipDir
		}

		marker, shape := anchor.ClassifyDir(path)
		if marker != ".git" {
			return nil
		}

		evidence := "nested-vcs-root"
		if shape == anchor.ShapeSubmodule {
			evidence = "submodule-gitdir"
		}
		name, identSource := anchor.ProjectIdentity(path)
		entries = append(entries, &Entry{
			Analyzer: AnalyzerTopology,
			Kind:     KindSubproject,
			Name:     name,
			FilePath: rel,
			Title:    fmt.Sprintf("Subproject %s at %s", name, rel),
			Detail: fmt.Sprintf(
				"Independent project scope (%s): sessions and stores anchored here are separate from this project's.",
				evidence),
			Metadata: map[string]string{
				"identity":        name,
				"identity_source": identSource,
				"gitdir_shape":    shape,
				"evidence":        evidence,
				"has_aide_store":  fmt.Sprintf("%t", anchor.HasAideStore(path)),
			},
			CreatedAt: time.Now(),
		})
		// A child scope's internals belong to its own survey.
		return filepath.SkipDir
	})
	return entries
}
