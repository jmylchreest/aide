package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jmylchreest/aide/aide/pkg/config"
	"github.com/jmylchreest/aide/aide/pkg/contextshare"
	"github.com/jmylchreest/aide/aide/pkg/memory"
)

// cmdShareShow renders the resolved sharing policy and previews what the current
// store would publish (and, when a per-record tree is present, what it would
// import) under that policy. It is strictly read-only: it never opens an
// export, never writes a file, and computes counts directly from the resolved
// filters rather than by invoking the export/import engines.
func cmdShareShow(dbPath string, args []string) error {
	projectRoot := projectRoot(dbPath)
	sharedDir := filepath.Join(projectRoot, ".aide", "shared")

	share := config.Get().Share

	backend, err := NewBackend(dbPath)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer backend.Close()

	decisions, err := backend.Store().ListDecisions()
	if err != nil {
		return fmt.Errorf("failed to list decisions: %w", err)
	}
	// IncludeAll mirrors Export's enumeration: forget-tagged memories are listed
	// and then gated by the filter, so the preview counts match a real export.
	memories, err := backend.Store().ListMemories(memory.SearchOptions{IncludeAll: true})
	if err != nil {
		return fmt.Errorf("failed to list memories: %w", err)
	}

	view := buildShareView(share, sharedDir, decisions, memories)

	if wantJSON(args) {
		return printJSON(view)
	}
	printShareView(view)
	return nil
}

// --- View model ---

// shareView is the stable shape rendered by `aide share show`, in both human
// and --json forms. It captures the resolved policy and the preview counts for
// each record type, plus an optional import preview when a per-record tree is
// present under .aide/shared/.
type shareView struct {
	SharedDir string         `json:"shared_dir"`
	Decisions shareTypeView  `json:"decisions"`
	Memories  shareTypeView  `json:"memories"`
	Import    *importPreview `json:"import,omitempty"`
}

// shareTypeView is the policy + export preview for one record type.
type shareTypeView struct {
	ExportEnabled bool          `json:"export_enabled"`
	ImportEnabled bool          `json:"import_enabled"`
	ExportFilter  filterView    `json:"export_filter"`
	ImportFilter  filterView    `json:"import_filter"`
	Preview       exportPreview `json:"export_preview"`
}

// filterView is the resolved include/exclude lists of a filter.
type filterView struct {
	Include []string `json:"include"`
	Exclude []string `json:"exclude"`
}

// exportPreview counts what the export filter would publish for a record type.
// Total is the number of records enumerated (at the export's granularity:
// decision VERSIONS, individual memories); Published pass the filter; Excluded
// are rejected by an exclude glob; ByPattern attributes each excluded record to
// the first exclude glob it matched. For a type whose export is OFF, the counts
// are still computed and labelled "if enabled" by the renderer.
type exportPreview struct {
	Total     int            `json:"total"`
	Published int            `json:"published"`
	Excluded  int            `json:"excluded"`
	ByPattern []patternTally `json:"by_pattern,omitempty"`
}

// patternTally is the per-exclude-pattern count of excluded records.
type patternTally struct {
	Pattern string `json:"pattern"`
	Count   int    `json:"count"`
}

// importPreview counts what the import filter would accept from the per-record
// tree under .aide/shared/. It is present only when that tree exists.
type importPreview struct {
	Decisions importTypePreview `json:"decisions"`
	Memories  importTypePreview `json:"memories"`
}

// importTypePreview counts tree records the import filter accepts vs excludes.
type importTypePreview struct {
	Total     int            `json:"total"`
	Accepted  int            `json:"accepted"`
	Excluded  int            `json:"excluded"`
	ByPattern []patternTally `json:"by_pattern,omitempty"`
}

// --- Build ---

// buildShareView resolves the policy and computes the export (and, when a
// per-record tree is present, import) previews. It performs no I/O beyond
// reading the existing .aide/shared/ tree for the import preview.
func buildShareView(share config.ShareConfig, sharedDir string, decisions []*memory.Decision, memories []*memory.Memory) shareView {
	decExport := toFilter(share.DecisionExportFilter())
	memExport := toFilter(share.MemoryExportFilter())

	view := shareView{
		SharedDir: sharedDir,
		Decisions: shareTypeView{
			ExportEnabled: share.DecisionExportEnabled(),
			ImportEnabled: share.DecisionImportEnabled(),
			ExportFilter:  toFilterView(share.DecisionExportFilter()),
			ImportFilter:  toFilterView(share.DecisionImportFilter()),
			Preview:       previewDecisions(decisions, decExport),
		},
		Memories: shareTypeView{
			ExportEnabled: share.MemoryExportEnabled(),
			ImportEnabled: share.MemoryImportEnabled(),
			ExportFilter:  toFilterView(share.MemoryExportFilter()),
			ImportFilter:  toFilterView(share.MemoryImportFilter()),
			Preview:       previewMemories(memories, memExport),
		},
	}

	// Import preview only for the per-record layout — the format `show`
	// describes. It is omitted when the tree is absent or only holds legacy
	// aggregates (the legacy importer has no token filter to preview).
	if hasPerRecordLayout(sharedDir) {
		imp := buildImportPreview(sharedDir,
			toFilter(share.DecisionImportFilter()),
			toFilter(share.MemoryImportFilter()))
		view.Import = &imp
	}

	return view
}

// previewDecisions counts how many decision VERSIONS the export filter would
// publish. This matches exportDecisions, which writes one write-once file per
// version (not per topic), so the count equals what a real export would write
// modulo tombstone shadowing (a deletion concern, not a policy one).
func previewDecisions(decisions []*memory.Decision, f contextshare.Filter) exportPreview {
	p := exportPreview{Total: len(decisions)}
	tally := map[string]int{}
	for _, d := range decisions {
		tokens := contextshare.DecisionTokens(d)
		if f.Match(tokens) {
			p.Published++
			continue
		}
		p.Excluded++
		if pat, ok := f.FirstExcludeMatch(tokens); ok {
			tally[pat]++
		}
	}
	p.ByPattern = sortedTally(tally)
	return p
}

// previewMemories counts how many memories the export filter would publish,
// enumerating every memory (forget-tagged included) exactly as exportMemories
// does before applying the token filter.
func previewMemories(memories []*memory.Memory, f contextshare.Filter) exportPreview {
	p := exportPreview{Total: len(memories)}
	tally := map[string]int{}
	for _, m := range memories {
		tokens := contextshare.MemoryTokens(m)
		if f.Match(tokens) {
			p.Published++
			continue
		}
		p.Excluded++
		if pat, ok := f.FirstExcludeMatch(tokens); ok {
			tally[pat]++
		}
	}
	p.ByPattern = sortedTally(tally)
	return p
}

// buildImportPreview walks the per-record tree under sharedDir and counts how
// many decision versions and memories the import filters would accept. It reads
// decisions/<topic>/<version>.md and memories/<ulid>.md using the exported
// contextshare parsers; unreadable or unparseable files are skipped, matching
// the importer's tolerance for foreign files.
func buildImportPreview(sharedDir string, decFilter, memFilter contextshare.Filter) importPreview {
	return importPreview{
		Decisions: previewImportDecisions(sharedDir, decFilter),
		Memories:  previewImportMemories(sharedDir, memFilter),
	}
}

func previewImportDecisions(sharedDir string, f contextshare.Filter) importTypePreview {
	p := importTypePreview{}
	tally := map[string]int{}
	topics, err := os.ReadDir(filepath.Join(sharedDir, "decisions"))
	if err != nil {
		return p
	}
	for _, topicDir := range topics {
		if !topicDir.IsDir() {
			continue
		}
		versions, err := os.ReadDir(filepath.Join(sharedDir, "decisions", topicDir.Name()))
		if err != nil {
			continue
		}
		for _, v := range versions {
			if v.IsDir() || !strings.HasSuffix(v.Name(), ".md") {
				continue
			}
			data, err := os.ReadFile(filepath.Join(sharedDir, "decisions", topicDir.Name(), v.Name()))
			if err != nil {
				continue
			}
			d, err := contextshare.ParseDecision(data)
			if err != nil {
				continue
			}
			p.Total++
			tokens := contextshare.DecisionTokens(d)
			if f.Match(tokens) {
				p.Accepted++
				continue
			}
			p.Excluded++
			if pat, ok := f.FirstExcludeMatch(tokens); ok {
				tally[pat]++
			}
		}
	}
	p.ByPattern = sortedTally(tally)
	return p
}

func previewImportMemories(sharedDir string, f contextshare.Filter) importTypePreview {
	p := importTypePreview{}
	tally := map[string]int{}
	entries, err := os.ReadDir(filepath.Join(sharedDir, "memories"))
	if err != nil {
		return p
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		if isReservedShareFile(entry.Name()) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(sharedDir, "memories", entry.Name()))
		if err != nil {
			continue
		}
		m, err := contextshare.ParseMemory(data)
		if err != nil {
			// Legacy aggregate files (type: memories) are not per-record memories
			// and ParseMemory rejects them; they have no token filter to preview.
			continue
		}
		p.Total++
		tokens := contextshare.MemoryTokens(m)
		if f.Match(tokens) {
			p.Accepted++
			continue
		}
		p.Excluded++
		if pat, ok := f.FirstExcludeMatch(tokens); ok {
			tally[pat]++
		}
	}
	p.ByPattern = sortedTally(tally)
	return p
}

// sortedTally flattens a pattern→count map into a stable slice ordered by
// descending count, then pattern, so output and JSON are deterministic.
func sortedTally(m map[string]int) []patternTally {
	if len(m) == 0 {
		return nil
	}
	out := make([]patternTally, 0, len(m))
	for pat, n := range m {
		out = append(out, patternTally{Pattern: pat, Count: n})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Pattern < out[j].Pattern
	})
	return out
}

func toFilterView(f config.ShareFilter) filterView {
	return filterView{Include: f.Include, Exclude: f.Exclude}
}

// --- Render (human) ---

// printShareView renders the human-readable policy + preview summary.
func printShareView(v shareView) {
	fmt.Printf("Sharing policy — %s\n\n", v.SharedDir)
	printType(v.Decisions, "decisions", "decision version", "decision versions")
	fmt.Println()
	printType(v.Memories, "memories", "memory", "memories")

	if v.Import != nil {
		fmt.Println()
		fmt.Println("import preview — what .aide/shared/ would bring in")
		printImportType(v.Import.Decisions, "  decisions", "decision version", "decision versions")
		printImportType(v.Import.Memories, "  memories ", "memory", "memories")
	}
}

// printType renders one record type's policy block and export preview.
func printType(t shareTypeView, label, singular, plural string) {
	fmt.Printf("%-11s export %-3s   import %s\n", label, onOff(t.ExportEnabled), onOff(t.ImportEnabled))
	fmt.Printf("  export  include %s   exclude %s\n", fmtList(t.ExportFilter.Include), fmtList(t.ExportFilter.Exclude))
	fmt.Printf("  import  include %s   exclude %s\n", fmtList(t.ImportFilter.Include), fmtList(t.ImportFilter.Exclude))

	p := t.Preview
	if t.ExportEnabled {
		fmt.Printf("  preview: %d %s would be published (%d excluded by policy)\n",
			p.Published, pluralize(p.Published, singular, plural), p.Excluded)
		printPatternTally(p.ByPattern, "    ")
		return
	}

	// Export is OFF: still show what it WOULD publish, clearly labelled.
	fmt.Printf("  preview: export disabled — set share.%s.export=true to publish\n", label)
	fmt.Printf("           if enabled: %d of %d %s would be published (%d excluded)\n",
		p.Published, p.Total, plural, p.Excluded)
	printPatternTally(p.ByPattern, "             ")
}

func printImportType(t importTypePreview, label, singular, plural string) {
	fmt.Printf("%s: %d of %d %s would be imported (%d excluded by policy)\n",
		label, t.Accepted, t.Total, pluralize(t.Accepted, singular, plural), t.Excluded)
	printPatternTally(t.ByPattern, "    ")
}

// printPatternTally prints the per-exclude-pattern breakdown, one line each.
func printPatternTally(tally []patternTally, indent string) {
	for _, pt := range tally {
		fmt.Printf("%s%d excluded by %s\n", indent, pt.Count, pt.Pattern)
	}
}

func onOff(b bool) string {
	if b {
		return "ON"
	}
	return "OFF"
}

// fmtList renders an include/exclude glob list as [a, b] or (none).
func fmtList(globs []string) string {
	if len(globs) == 0 {
		return "(none)"
	}
	return "[" + strings.Join(globs, ", ") + "]"
}

func pluralize(n int, singular, plural string) string {
	if n == 1 {
		return singular
	}
	return plural
}
