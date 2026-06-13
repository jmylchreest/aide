// Package main provides the share command for exporting/importing aide data
// as git-friendly markdown files with YAML frontmatter.
//
// Exports write the file-per-record format under .aide/shared/ (see
// pkg/contextshare): write-once decision version files, one file per memory
// that passes the share filter, tombstones for deletions, and a manifest
// watermark. The legacy aggregate layout shares the same .aide/shared/
// directory without colliding (legacy decisions are files decisions/<topic>.md,
// per-record decisions are directories decisions/<topic>-<hash>/<ts>.md; legacy
// memories are multi-entry files memories/<category>.md, per-record memories
// are single-entry files memories/<ulid>.md). The legacy layout remains
// importable and is exportable behind --legacy; the first per-record export
// migrates any legacy aggregates into the store and removes them.
package main

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/config"
	"github.com/jmylchreest/aide/aide/pkg/contextshare"
	"github.com/jmylchreest/aide/aide/pkg/memory"
)

// toFilter maps a config.ShareFilter (already resolved with defaults by the
// config accessors) onto the config-agnostic contextshare.Filter the engine
// applies. Keeping the conversion in the cmd layer is what lets contextshare
// avoid importing pkg/config.
func toFilter(f config.ShareFilter) contextshare.Filter {
	return contextshare.Filter{Include: f.Include, Exclude: f.Exclude}
}

// sanitizeFilenameRe is compiled once for sanitizeFilename.
var sanitizeFilenameRe = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

// reservedShareFiles are the explainer markdowns written alongside exports.
// They must be skipped by the import parsers and preserved by removeStaleFiles.
var reservedShareFiles = map[string]bool{
	"DECISIONS.md": true,
	"MEMORIES.md":  true,
}

// isReservedShareFile reports whether name is one of the reserved explainer
// files written by the export command and must be skipped by importers.
func isReservedShareFile(name string) bool {
	return reservedShareFiles[name]
}

// cmdShare dispatches share subcommands.
func cmdShare(dbPath string, args []string) error {
	return dispatchSubcmd("share", args, printShareUsage, []subcmd{
		{name: "export", handler: func(a []string) error { return cmdShareExport(dbPath, a) }},
		{name: "import", handler: func(a []string) error { return cmdShareImport(dbPath, a) }},
	})
}

func printShareUsage() {
	fmt.Println(`aide share - Export/import aide data as git-friendly markdown

Shared files are written to .aide/shared/ and designed to be committed to git.
One file per record (decision version, memory, tombstone), so re-exports of
unchanged content are byte-identical and deletions propagate via tombstones.
Which records ship is governed by the share.{decisions,memories} policy in
.aide/config/aide.json (decisions export+import on by default, memories off).
The first per-record export migrates any legacy aggregate files in place.

Usage:
  aide share <subcommand> [arguments]

Subcommands:
  export     Export decisions and memories to .aide/shared/
  import     Import decisions and memories from .aide/shared/

Options:
  export:
    --decisions          Export decisions only
    --memories           Export memories only (forces memory export on)
    --all-memories       Export all memories (include ["*"], no exclude)
    --output=DIR         Output directory (default: .aide/shared)
    --legacy             Write the legacy .aide/shared/ aggregate layout

  import:
    --decisions          Import decisions only
    --memories           Import memories only (forces memory import on)
    --input=DIR          Input directory (default: .aide/shared)
    --dry-run            Show what would be imported without changing anything
    --force              Import even when the export manifest is missing or stale

Examples:
  aide share export                          # Export per policy (decisions by default)
  aide share export --decisions              # Decisions only
  aide share import                          # Import per policy
  aide share import --dry-run                # Preview import`)
}

// --- Export ---

func cmdShareExport(dbPath string, args []string) error {
	projectRoot := projectRoot(dbPath)
	legacy := hasFlag(args, "--legacy")

	decisionsOnly := hasFlag(args, "--decisions")
	memoriesOnly := hasFlag(args, "--memories")
	allMemories := hasFlag(args, "--all-memories")

	backend, err := NewBackend(dbPath)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer backend.Close()

	if !legacy {
		share := config.Get().Share

		// Start from the config policy, then apply CLI overrides. --decisions and
		// --memories scope the run to a single type; --memories and --all-memories
		// force memory export on (the latter with the broadest filter).
		exportDecisions := share.DecisionExportEnabled()
		exportMemories := share.MemoryExportEnabled()
		decisionFilter := toFilter(share.DecisionExportFilter())
		memoryFilter := toFilter(share.MemoryExportFilter())

		if decisionsOnly {
			exportDecisions = true
			exportMemories = false
		}
		if memoriesOnly {
			exportDecisions = false
			exportMemories = true
		}
		if allMemories {
			exportMemories = true
			memoryFilter = contextshare.Filter{Include: []string{"*"}, Exclude: nil}
		}

		outputDir := filepath.Join(projectRoot, ".aide", "shared")
		if o := parseFlag(args, "--output="); o != "" {
			outputDir = o
		}

		if err := migrateLegacyAggregates(backend, outputDir); err != nil {
			return fmt.Errorf("failed to migrate legacy share files: %w", err)
		}

		stats, err := contextshare.Export(backend.Store(), backend.TombstoneStore(), outputDir, contextshare.ExportOptions{
			Decisions:      exportDecisions,
			Memories:       exportMemories,
			DecisionFilter: decisionFilter,
			MemoryFilter:   memoryFilter,
		})
		if err != nil {
			return fmt.Errorf("failed to export: %w", err)
		}
		if err := writeShareExplainers(outputDir); err != nil {
			return fmt.Errorf("failed to write explainer files: %w", err)
		}
		fmt.Printf("Exported to %s\n", outputDir)
		if exportDecisions {
			fmt.Printf("  decisions:  %d\n", stats.Decisions)
		}
		if exportMemories {
			fmt.Printf("  memories:   %d\n", stats.Memories)
		}
		fmt.Printf("  tombstones: %d\n", stats.Tombstones)
		if backend.TombstoneStore() == nil {
			fmt.Fprintln(os.Stderr, "warning: daemon is running; deletions recorded in the daemon's store were not materialised as tombstones")
		}
		return nil
	}

	// Legacy aggregate export still honours the type-scoping CLI flags.
	exportDecisions := !memoriesOnly
	exportMemories := !decisionsOnly

	outputDir := filepath.Join(projectRoot, ".aide", "shared")
	if o := parseFlag(args, "--output="); o != "" {
		outputDir = o
	}

	var decisionsExported, memoriesExported int

	if exportDecisions {
		n, err := shareExportDecisions(backend, outputDir)
		if err != nil {
			return fmt.Errorf("failed to export decisions: %w", err)
		}
		decisionsExported = n
	}

	if exportMemories {
		n, err := shareExportMemories(backend, outputDir, allMemories)
		if err != nil {
			return fmt.Errorf("failed to export memories: %w", err)
		}
		memoriesExported = n
	}

	fmt.Printf("Exported to %s\n", outputDir)
	if exportDecisions {
		fmt.Printf("  decisions: %d\n", decisionsExported)
	}
	if exportMemories {
		fmt.Printf("  memories:  %d\n", memoriesExported)
	}
	return nil
}

// legacyAggregateFiles returns the flat legacy aggregate .md files under
// outputDir's decisions/ and memories/ subdirectories (skipping the
// DECISIONS.md / MEMORIES.md explainers and any per-record directories). The
// presence of any such file means a first per-record export must migrate.
//
// Decision aggregates are unambiguous — per-record decisions live in topic
// subdirectories, so any flat decisions/<name>.md is legacy. Memory files are
// ambiguous on name alone (memories/<ulid>.md vs memories/<category>.md), so a
// flat memory file is treated as a legacy aggregate only when its frontmatter
// is the aggregate "type: memories" form rather than a per-record "id:" record.
func legacyAggregateFiles(outputDir string) []string {
	var files []string
	for _, sub := range []string{"decisions", "memories"} {
		dir := filepath.Join(outputDir, sub)
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
				continue
			}
			if isReservedShareFile(entry.Name()) {
				continue
			}
			path := filepath.Join(dir, entry.Name())
			if sub == "memories" && !isLegacyMemoryAggregate(path) {
				continue
			}
			files = append(files, path)
		}
	}
	return files
}

// isLegacyMemoryAggregate reports whether a flat memories/*.md file is a legacy
// multi-entry aggregate (frontmatter "type: memories") rather than a per-record
// single-memory file (frontmatter "id:"). Unreadable files are not treated as
// legacy, so a transient read error never triggers a destructive migration.
func isLegacyMemoryAggregate(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "---" {
			continue
		}
		if line == "" {
			break // end of frontmatter region
		}
		if strings.HasPrefix(line, "id:") {
			return false
		}
		if strings.HasPrefix(line, "type:") && strings.TrimSpace(strings.TrimPrefix(line, "type:")) == "memories" {
			return true
		}
	}
	return false
}

// migrateLegacyAggregates performs the one-shot legacy → per-record migration
// on first per-record export. When legacy aggregate files are present, it first
// imports them into the store (reusing the legacy parsers so nothing a teammate
// published is lost), then deletes the legacy aggregate files plus their stale
// DECISIONS.md / MEMORIES.md explainers, so the subsequent per-record export
// re-materialises the migrated records in the new format. Idempotent: after the
// first export the legacy files are gone and this step is skipped.
func migrateLegacyAggregates(backend *Backend, outputDir string) error {
	legacy := legacyAggregateFiles(outputDir)
	if len(legacy) == 0 {
		return nil
	}

	if _, _, err := shareImportDecisions(backend, outputDir, false); err != nil {
		return fmt.Errorf("failed to import legacy decisions: %w", err)
	}
	if _, _, err := shareImportMemories(backend, outputDir, false); err != nil {
		return fmt.Errorf("failed to import legacy memories: %w", err)
	}

	// Remove the migrated aggregate files and the now-stale explainer files; the
	// per-record export writes fresh DECISIONS.md / MEMORIES.md afterwards.
	toRemove := append([]string{}, legacy...)
	for _, sub := range []string{"decisions", "memories"} {
		for name := range reservedShareFiles {
			toRemove = append(toRemove, filepath.Join(outputDir, sub, name))
		}
	}
	for _, path := range toRemove {
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("failed to remove legacy file %s: %w", path, err)
		}
	}

	fmt.Fprintf(os.Stderr, "notice: migrated %d legacy aggregate share file(s) in %s to per-record format\n", len(legacy), outputDir)
	return nil
}

// writeShareExplainers (re)writes the DECISIONS.md / MEMORIES.md human-readable
// explainers in the per-record tree. Each is written only when its subdirectory
// holds records (a per-record directory or .md file other than the explainer
// itself), and removed when the subdirectory has emptied out, so the explainer
// never lingers next to an empty folder.
func writeShareExplainers(outputDir string) error {
	type explainer struct {
		sub     string
		name    string
		content string
	}
	for _, e := range []explainer{
		{"decisions", "DECISIONS.md", decisionsReadmeContent},
		{"memories", "MEMORIES.md", memoriesReadmeContent},
	} {
		dir := filepath.Join(outputDir, e.sub)
		path := filepath.Join(dir, e.name)
		if shareDirHasRecords(dir, e.name) {
			if err := os.WriteFile(path, []byte(e.content), 0o644); err != nil {
				return err
			}
			continue
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	return nil
}

// shareDirHasRecords reports whether dir holds any record (a subdirectory or a
// .md file other than the named explainer). Used to decide whether the explainer
// is warranted.
func shareDirHasRecords(dir, explainer string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.IsDir() {
			return true
		}
		if strings.HasSuffix(entry.Name(), ".md") && entry.Name() != explainer {
			return true
		}
	}
	return false
}

// shareExportDecisions writes each decision topic as a separate markdown file.
//
// Format:
//
//	---
//	topic: auth-strategy
//	decision: JWT with refresh tokens
//	decided_by: agent-abc
//	date: 2026-01-15
//	references:
//	  - https://example.com/jwt-guide
//	---
//
//	## Rationale
//
//	Stateless auth, mobile client support needed.
//
//	## Details
//
//	<full details text>
func shareExportDecisions(b *Backend, outputDir string) (int, error) {
	decisions, err := b.ListDecisions()
	if err != nil {
		return 0, err
	}

	// Group by topic, keep latest
	latest := make(map[string]*memory.Decision)
	for _, d := range decisions {
		if existing, ok := latest[d.Topic]; !ok || d.CreatedAt.After(existing.CreatedAt) {
			latest[d.Topic] = d
		}
	}

	dir := filepath.Join(outputDir, "decisions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, fmt.Errorf("failed to create directory: %w", err)
	}

	// Build set of files we'll write (to detect stale files)
	expectedFiles := make(map[string]bool)
	count := 0
	for _, d := range latest {
		filename := sanitizeFilename(d.Topic) + ".md"
		expectedFiles[filename] = true
		fullPath := filepath.Join(dir, filename)
		if err := writeDecisionMarkdown(fullPath, d); err != nil {
			return count, fmt.Errorf("failed to write %s: %w", fullPath, err)
		}
		count++
	}

	// Write an explainer DECISIONS.md only when the folder has content; exempt
	// it from stale-file cleanup. If count is zero, fall through so
	// removeStaleFiles wipes any orphaned DECISIONS.md too.
	if count > 0 {
		expectedFiles["DECISIONS.md"] = true
		if err := writeDecisionsReadme(filepath.Join(dir, "DECISIONS.md")); err != nil {
			return count, fmt.Errorf("failed to write DECISIONS.md: %w", err)
		}
	}

	// Remove stale decision files (deleted decisions)
	if err := removeStaleFiles(dir, expectedFiles); err != nil {
		return count, fmt.Errorf("failed to clean stale decisions: %w", err)
	}

	return count, nil
}

// decisionsReadmeContent is the static explainer written to .aide/shared/decisions/DECISIONS.md.
// Covers both aide users (import) and non-aide users (folder as LLM context).
const decisionsReadmeContent = `# Team Decisions

This folder contains team architectural decisions, one markdown file per topic.
Each file has YAML frontmatter with structured fields and a markdown body.

## With aide

Decisions import automatically at session start when ` + "`AIDE_SHARE_AUTO_IMPORT=1`" + ` is
set in ` + "`.claude/settings.json`" + `. Manually:

    aide share import --decisions

Decisions are append-only per topic: committing a different decision for an existing
topic supersedes the old one, and ` + "`aide decision history <topic>`" + ` shows the thread.

## Without aide

Each ` + "`.md`" + ` file is a self-contained decision record. Point your AI assistant at this
folder as context — the frontmatter answers *what* was decided, the body answers *why*.
`

func writeDecisionsReadme(path string) error {
	return os.WriteFile(path, []byte(decisionsReadmeContent), 0o644)
}

func writeDecisionMarkdown(filename string, d *memory.Decision) error {
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	// YAML frontmatter
	fmt.Fprintln(f, "---")
	fmt.Fprintf(f, "topic: %s\n", d.Topic)
	fmt.Fprintf(f, "decision: %s\n", yamlEscape(d.Decision))
	if d.DecidedBy != "" {
		fmt.Fprintf(f, "decided_by: %s\n", d.DecidedBy)
	}
	fmt.Fprintf(f, "date: %s\n", d.CreatedAt.Format("2006-01-02"))
	if len(d.References) > 0 {
		fmt.Fprintln(f, "references:")
		for _, ref := range d.References {
			fmt.Fprintf(f, "  - %s\n", ref)
		}
	}
	fmt.Fprintln(f, "---")
	fmt.Fprintln(f)

	// Body
	fmt.Fprintf(f, "# %s\n\n", d.Topic)
	fmt.Fprintf(f, "**Decision:** %s\n\n", d.Decision)

	if d.Rationale != "" {
		fmt.Fprintln(f, "## Rationale")
		fmt.Fprintln(f)
		fmt.Fprintln(f, d.Rationale)
		fmt.Fprintln(f)
	}

	if d.Details != "" {
		fmt.Fprintln(f, "## Details")
		fmt.Fprintln(f)
		fmt.Fprintln(f, d.Details)
		fmt.Fprintln(f)
	}

	return nil
}

// shareExportMemories writes memories grouped by category as markdown files.
//
// Format per entry in the file:
//
//	### <truncated content as heading>
//
//	<!-- aide:id=01ABC123 tags=project:myapp,testing date=2026-01-15 -->
//
//	<full content>
//
//	---
func shareExportMemories(b *Backend, outputDir string, includeAll bool) (int, error) {
	var excludeOpts *memory.SearchOptions
	if includeAll {
		excludeOpts = &memory.SearchOptions{IncludeAll: true}
	}
	memories, err := b.ListMemories("", 0, excludeOpts)
	if err != nil {
		return 0, err
	}

	// Filter to shareable memories unless --all-memories
	var shareable []*memory.Memory
	if includeAll {
		shareable = memories
	} else {
		for _, m := range memories {
			if contextshare.IsShareableMemory(m) {
				shareable = append(shareable, m)
			}
		}
	}

	dir := filepath.Join(outputDir, "memories")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, fmt.Errorf("failed to create directory: %w", err)
	}

	// Group by category
	categories := map[memory.Category][]*memory.Memory{}
	for _, m := range shareable {
		categories[m.Category] = append(categories[m.Category], m)
	}

	// Build set of expected files
	expectedFiles := make(map[string]bool)
	total := 0
	for cat, mems := range categories {
		filename := string(cat) + ".md"
		expectedFiles[filename] = true
		fullPath := filepath.Join(dir, filename)
		if err := writeMemoriesMarkdown(fullPath, cat, mems); err != nil {
			return total, fmt.Errorf("failed to write %s: %w", fullPath, err)
		}
		total += len(mems)
	}

	// Write an explainer MEMORIES.md only when the folder has content; exempt
	// it from stale-file cleanup. If total is zero, fall through so
	// removeStaleFiles wipes any orphaned MEMORIES.md too.
	if total > 0 {
		expectedFiles["MEMORIES.md"] = true
		if err := writeMemoriesReadme(filepath.Join(dir, "MEMORIES.md")); err != nil {
			return total, fmt.Errorf("failed to write MEMORIES.md: %w", err)
		}
	}

	// Remove stale category files (categories with no more shareable memories)
	if err := removeStaleFiles(dir, expectedFiles); err != nil {
		return total, fmt.Errorf("failed to clean stale memories: %w", err)
	}

	return total, nil
}

// memoriesReadmeContent is the static explainer written to .aide/shared/memories/MEMORIES.md.
// Covers both aide users (import) and non-aide users (folder as LLM context).
const memoriesReadmeContent = `# Team Memories

This folder contains team project memories — patterns, gotchas, and learnings —
grouped into one markdown file per category.

## With aide

Memories import automatically at session start when ` + "`AIDE_SHARE_AUTO_IMPORT=1`" + ` is
set in ` + "`.claude/settings.json`" + `. Manually:

    aide share import --memories

Each memory is keyed by a ULID (in the ` + "`<!-- aide:id=... -->`" + ` metadata comment) so
teammate edits with a newer ` + "`updated=`" + ` timestamp land as in-place updates instead of
duplicates.

## Without aide

Each entry inside a category file is a self-contained memory with a short metadata
comment and a free-text body. Point your AI assistant at this folder as context —
the category headings group related notes.
`

func writeMemoriesReadme(path string) error {
	return os.WriteFile(path, []byte(memoriesReadmeContent), 0o644)
}

func writeMemoriesMarkdown(filename string, cat memory.Category, memories []*memory.Memory) error {
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	// File header with frontmatter
	fmt.Fprintln(f, "---")
	fmt.Fprintf(f, "type: memories\n")
	fmt.Fprintf(f, "category: %s\n", cat)
	fmt.Fprintf(f, "count: %d\n", len(memories))
	fmt.Fprintf(f, "exported: %s\n", time.Now().Format("2006-01-02"))
	fmt.Fprintln(f, "---")
	fmt.Fprintln(f)
	fmt.Fprintf(f, "# %s\n\n", titleCase(string(cat)))

	for _, m := range memories {
		// Heading: first line of content, truncated
		heading := firstLine(m.Content)
		if len(heading) > 80 {
			heading = heading[:77] + "..."
		}
		fmt.Fprintf(f, "### %s\n\n", heading)

		// Metadata comment (parseable but invisible in rendered markdown).
		// `updated` is emitted only when the memory has been edited after creation
		// so that import can resolve "same ID, newer content" conflicts.
		tags := ""
		if len(m.Tags) > 0 {
			tags = " tags=" + strings.Join(m.Tags, ",")
		}
		updated := ""
		if !m.UpdatedAt.IsZero() && m.UpdatedAt.After(m.CreatedAt) {
			updated = " updated=" + m.UpdatedAt.UTC().Format(time.RFC3339)
		}
		// `date` uses RFC3339 to round-trip with full precision. The parser
		// still accepts the legacy `YYYY-MM-DD` form for files exported by
		// older versions of aide.
		fmt.Fprintf(f, "<!-- aide:id=%s%s date=%s%s -->\n\n",
			m.ID, tags, m.CreatedAt.UTC().Format(time.RFC3339), updated)

		// Full content
		fmt.Fprintln(f, m.Content)
		fmt.Fprintln(f)
		fmt.Fprintln(f, "---")
		fmt.Fprintln(f)
	}

	return nil
}

// --- Import ---

// hasPerRecordLayout reports whether dir holds the file-per-record format,
// which now coexists with the legacy aggregate layout in the same .aide/shared/
// directory. A manifest is the definitive marker; a tombstones dir or decision
// topic subdirectories identify a per-record tree whose manifest was lost (the
// import stale guard then asks for a fresh export). Per-record memory files
// (memories/<ulid>.md) are intentionally not used as a marker — they share the
// flat-.md shape of legacy category files (memories/<category>.md), and every
// real per-record export writes a manifest anyway.
func hasPerRecordLayout(dir string) bool {
	if _, err := os.Stat(filepath.Join(dir, contextshare.ManifestName)); err == nil {
		return true
	}
	if _, err := os.Stat(filepath.Join(dir, "tombstones")); err == nil {
		return true
	}
	if entries, err := os.ReadDir(filepath.Join(dir, "decisions")); err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				return true
			}
		}
	}
	return false
}

// hasLegacyRecords reports whether dir holds any legacy aggregate file the
// legacy importer should read — a flat decisions/<topic>.md or a flat
// memories/<category>.md (the "type: memories" form), excluding the explainer
// files and per-record memory files. It is the same classification as
// legacyAggregateFiles, so the import path and the migration path agree on what
// "legacy" means.
func hasLegacyRecords(dir string) bool {
	return len(legacyAggregateFiles(dir)) > 0
}

func cmdShareImport(dbPath string, args []string) error {
	projectRoot := projectRoot(dbPath)

	inputDir := parseFlag(args, "--input=")
	if inputDir == "" {
		inputDir = filepath.Join(projectRoot, ".aide", "shared")
	}

	decisionsOnly := hasFlag(args, "--decisions")
	memoriesOnly := hasFlag(args, "--memories")
	dryRun := hasFlag(args, "--dry-run")

	share := config.Get().Share

	// Start from the config policy, then apply CLI overrides. --decisions and
	// --memories scope the run to a single type; --memories also forces memory
	// import on (a teammate can pull memories on demand without flipping config).
	importDecisions := share.DecisionImportEnabled()
	importMemories := share.MemoryImportEnabled()
	decisionFilter := toFilter(share.DecisionImportFilter())
	memoryFilter := toFilter(share.MemoryImportFilter())

	if decisionsOnly {
		importDecisions = true
		importMemories = false
	}
	if memoriesOnly {
		importDecisions = false
		importMemories = true
	}

	backend, err := NewBackend(dbPath)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer backend.Close()

	// The per-record and legacy layouts now share .aide/shared/. Run the
	// per-record importer when that layout is present (or when --input points at
	// a tree we should treat as per-record), then run the legacy importer when
	// flat aggregate files remain. The two parsers are orthogonal: per-record
	// parsing skips flat legacy files and legacy parsing skips per-record dirs.
	perRecord := hasPerRecordLayout(inputDir)
	legacy := hasLegacyRecords(inputDir)
	// An explicit --input with neither layout still hits the per-record importer
	// so the stale guard surfaces its clear error rather than silently doing
	// nothing.
	if !perRecord && !legacy {
		perRecord = true
	}

	action := "Imported from"
	if dryRun {
		action = "Dry run from"
	}
	fmt.Printf("%s %s\n", action, inputDir)

	if perRecord {
		stats, err := contextshare.Import(backend.Store(), backend.TombstoneStore(), inputDir, contextshare.ImportOptions{
			Force:          hasFlag(args, "--force"),
			DryRun:         dryRun,
			Decisions:      importDecisions,
			Memories:       importMemories,
			DecisionFilter: decisionFilter,
			MemoryFilter:   memoryFilter,
		})
		if err != nil {
			return fmt.Errorf("failed to import: %w", err)
		}
		if importDecisions {
			fmt.Printf("  decisions:  %d imported, %d skipped (already exist)\n", stats.DecisionsImported, stats.DecisionsSkipped)
		}
		if importMemories {
			fmt.Printf("  memories:   %d imported, %d skipped (already exist)\n", stats.MemoriesImported, stats.MemoriesSkipped)
		}
		fmt.Printf("  tombstones: %d recorded, %d local records deleted, %d ignored\n",
			stats.TombstonesRecorded, stats.RecordsDeleted, stats.TombstonesIgnored)
		if backend.TombstoneStore() == nil {
			fmt.Fprintln(os.Stderr, "warning: daemon is running; incoming deletions were applied, but tombstones could not be recorded locally with their original timestamps")
		}
	}

	if !legacy {
		return nil
	}

	var decisionsImported, memoriesImported, decisionsSkipped, memoriesSkipped int

	if importDecisions {
		imported, skipped, err := shareImportDecisions(backend, inputDir, dryRun)
		if err != nil {
			return fmt.Errorf("failed to import decisions: %w", err)
		}
		decisionsImported = imported
		decisionsSkipped = skipped
	}

	if importMemories {
		imported, skipped, err := shareImportMemories(backend, inputDir, dryRun)
		if err != nil {
			return fmt.Errorf("failed to import memories: %w", err)
		}
		memoriesImported = imported
		memoriesSkipped = skipped
	}

	if perRecord {
		fmt.Println("  legacy aggregate files:")
	}
	if importDecisions {
		fmt.Printf("  decisions: %d imported, %d skipped (already exist)\n", decisionsImported, decisionsSkipped)
	}
	if importMemories {
		fmt.Printf("  memories:  %d imported, %d skipped (already exist)\n", memoriesImported, memoriesSkipped)
	}
	return nil
}

// shareImportDecisions reads decision markdown files from .aide/shared/decisions/
// and imports them into the bolt store. Skips decisions where the topic already
// exists with the same decision text (idempotent).
func shareImportDecisions(b *Backend, inputDir string, dryRun bool) (imported, skipped int, err error) {
	dir := filepath.Join(inputDir, "decisions")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return 0, 0, nil
		}
		return 0, 0, err
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		if isReservedShareFile(entry.Name()) {
			continue
		}

		d, err := parseDecisionMarkdown(filepath.Join(dir, entry.Name()))
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: skipping %s: %v\n", entry.Name(), err)
			continue
		}

		// Check if decision already exists with same content
		existing, getErr := b.GetDecision(d.Topic)
		if getErr == nil && existing.Decision == d.Decision {
			skipped++
			if dryRun {
				fmt.Printf("  skip decision: %s (unchanged)\n", d.Topic)
			}
			continue
		}

		if dryRun {
			fmt.Printf("  import decision: %s = %s\n", d.Topic, d.Decision)
			imported++
			continue
		}

		_, err = b.SetDecision(d.Topic, d.Decision, d.Rationale, d.Details, d.DecidedBy, d.References)
		if err != nil {
			return imported, skipped, fmt.Errorf("failed to import decision %s: %w", d.Topic, err)
		}
		imported++
	}

	return imported, skipped, nil
}

// parseDecisionMarkdown reads a decision markdown file and extracts the decision.
func parseDecisionMarkdown(filename string) (*memory.Decision, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	d := &memory.Decision{}
	scanner := bufio.NewScanner(f)

	// Parse YAML frontmatter
	inFrontmatter := false
	inBody := false
	var bodySection string
	var rationaleLines []string
	var detailsLines []string

	for scanner.Scan() {
		line := scanner.Text()

		if line == "---" {
			if !inFrontmatter && !inBody {
				inFrontmatter = true
				continue
			}
			if inFrontmatter {
				inFrontmatter = false
				inBody = true
				continue
			}
		}

		if inFrontmatter {
			parseFrontmatterLine(line, d)
			continue
		}

		if inBody {
			// Track which section we're in
			if strings.HasPrefix(line, "## Rationale") {
				bodySection = "rationale"
				continue
			}
			if strings.HasPrefix(line, "## Details") {
				bodySection = "details"
				continue
			}
			if strings.HasPrefix(line, "# ") {
				// Top-level heading — skip (it's the topic name)
				continue
			}
			if strings.HasPrefix(line, "**Decision:**") {
				// Skip — already in frontmatter
				continue
			}

			switch bodySection {
			case "rationale":
				rationaleLines = append(rationaleLines, line)
			case "details":
				detailsLines = append(detailsLines, line)
			}
		}
	}

	if d.Topic == "" {
		return nil, fmt.Errorf("missing topic in frontmatter")
	}
	if d.Decision == "" {
		return nil, fmt.Errorf("missing decision in frontmatter")
	}

	// Set body sections, trimming whitespace
	if len(rationaleLines) > 0 {
		d.Rationale = strings.TrimSpace(strings.Join(rationaleLines, "\n"))
	}
	if len(detailsLines) > 0 {
		d.Details = strings.TrimSpace(strings.Join(detailsLines, "\n"))
	}

	return d, scanner.Err()
}

// parseFrontmatterLine parses a single YAML frontmatter line into a Decision.
func parseFrontmatterLine(line string, d *memory.Decision) {
	switch {
	case strings.HasPrefix(line, "topic:"):
		d.Topic = strings.TrimSpace(strings.TrimPrefix(line, "topic:"))
	case strings.HasPrefix(line, "decision:"):
		d.Decision = yamlUnescape(strings.TrimSpace(strings.TrimPrefix(line, "decision:")))
	case strings.HasPrefix(line, "decided_by:"):
		d.DecidedBy = strings.TrimSpace(strings.TrimPrefix(line, "decided_by:"))
	case strings.HasPrefix(line, "date:"):
		if dateStr, ok := strings.CutPrefix(line, "date:"); ok {
			if t, err := time.Parse("2006-01-02", strings.TrimSpace(dateStr)); err == nil {
				d.CreatedAt = t
			}
		}
	case strings.HasPrefix(line, "  - "):
		// Reference list item
		d.References = append(d.References, strings.TrimPrefix(line, "  - "))
	}
}

// shareImportMemories reads memory markdown files from .aide/shared/memories/
// and imports them into the bolt store. Skips memories that already exist (by ID).
func shareImportMemories(b *Backend, inputDir string, dryRun bool) (imported, skipped int, err error) {
	dir := filepath.Join(inputDir, "memories")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return 0, 0, nil
		}
		return 0, 0, err
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		if isReservedShareFile(entry.Name()) {
			continue
		}

		memories, err := parseMemoriesMarkdown(filepath.Join(dir, entry.Name()))
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: skipping %s: %v\n", entry.Name(), err)
			continue
		}

		for _, m := range memories {
			// Existing memory with same ID: resolve via UpdatedAt (newer wins).
			// If incoming has no `updated` field (or is not newer), skip — keeping
			// memory sync additive-by-default and only letting explicit edits propagate.
			if m.ID != "" {
				existing, getErr := b.GetMemory(m.ID)
				if getErr == nil && existing != nil {
					if m.UpdatedAt.IsZero() || !m.UpdatedAt.After(existing.UpdatedAt) {
						skipped++
						if dryRun {
							fmt.Printf("  skip memory: %s (exists)\n", truncate(m.Content, 50))
						}
						continue
					}
					if dryRun {
						fmt.Printf("  update memory: %s (incoming newer)\n", truncate(m.Content, 50))
						imported++
						continue
					}
					existing.Content = m.Content
					existing.Category = m.Category
					existing.Tags = m.Tags
					existing.UpdatedAt = m.UpdatedAt
					if err := b.Store().UpdateMemory(existing); err != nil {
						return imported, skipped, fmt.Errorf("failed to update memory %s: %w", m.ID, err)
					}
					imported++
					continue
				}
			}

			if dryRun {
				fmt.Printf("  import memory [%s]: %s\n", m.Category, truncate(m.Content, 50))
				imported++
				continue
			}

			// Use Store().AddMemory so the incoming ULID, CreatedAt and UpdatedAt
			// are preserved — same-ULID-is-same-memory is the invariant that makes
			// future updates land on the existing record on every teammate's clone.
			newMem := &memory.Memory{
				ID:        m.ID,
				Content:   m.Content,
				Category:  m.Category,
				Tags:      m.Tags,
				CreatedAt: m.CreatedAt,
				UpdatedAt: m.UpdatedAt,
			}
			if err := b.Store().AddMemory(newMem); err != nil {
				return imported, skipped, fmt.Errorf("failed to import memory: %w", err)
			}
			imported++
		}
	}

	return imported, skipped, nil
}

// aideCommentRegex parses the <!-- aide:... --> metadata comments.
// `updated` is optional and only present when a memory was edited after creation;
// it is used by share import to resolve same-ID conflicts (newer wins).
var aideCommentRegex = regexp.MustCompile(`<!--\s*aide:id=(\S+)(?:\s+tags=(\S+))?(?:\s+date=(\S+))?(?:\s+updated=(\S+))?\s*-->`)

// parseMemoriesMarkdown reads a memories markdown file and extracts individual memories.
func parseMemoriesMarkdown(filename string) ([]*memory.Memory, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(data), "\n")
	var memories []*memory.Memory
	var current *memory.Memory
	var contentLines []string
	inFrontmatter := false
	frontmatterSeen := false
	category := memory.CategoryLearning // default

	for _, line := range lines {
		// Skip file-level frontmatter
		if line == "---" && !frontmatterSeen {
			inFrontmatter = !inFrontmatter
			if !inFrontmatter {
				frontmatterSeen = true
			}
			continue
		}
		if inFrontmatter {
			if strings.HasPrefix(line, "category:") {
				category = memory.Category(strings.TrimSpace(strings.TrimPrefix(line, "category:")))
			}
			continue
		}

		// Entry separator: --- between entries
		if line == "---" {
			// Flush current memory
			if current != nil {
				current.Content = strings.TrimSpace(strings.Join(contentLines, "\n"))
				if current.Content != "" {
					memories = append(memories, current)
				}
				current = nil
				contentLines = nil
			}
			continue
		}

		// New entry heading
		if strings.HasPrefix(line, "### ") {
			// Flush previous if any
			if current != nil {
				current.Content = strings.TrimSpace(strings.Join(contentLines, "\n"))
				if current.Content != "" {
					memories = append(memories, current)
				}
			}
			current = &memory.Memory{Category: category}
			contentLines = nil
			continue
		}

		// Metadata comment
		if matches := aideCommentRegex.FindStringSubmatch(line); matches != nil {
			if current != nil {
				current.ID = matches[1]
				if matches[2] != "" {
					current.Tags = strings.Split(matches[2], ",")
				}
				if matches[3] != "" {
					// RFC3339 is the modern format; fall back to day-precision
					// `YYYY-MM-DD` for files written by older aide versions.
					if t, err := time.Parse(time.RFC3339, matches[3]); err == nil {
						current.CreatedAt = t
					} else if t, err := time.Parse("2006-01-02", matches[3]); err == nil {
						current.CreatedAt = t
					}
				}
				if matches[4] != "" {
					if t, err := time.Parse(time.RFC3339, matches[4]); err == nil {
						current.UpdatedAt = t
					}
				}
			}
			continue
		}

		// Regular content line
		if current != nil {
			contentLines = append(contentLines, line)
		}
	}

	// Flush final entry
	if current != nil {
		current.Content = strings.TrimSpace(strings.Join(contentLines, "\n"))
		if current.Content != "" {
			memories = append(memories, current)
		}
	}

	return memories, nil
}

// --- Helpers ---

// removeStaleFiles removes .md files from dir that are not in the expectedFiles set.
// This ensures deleted decisions/memories don't linger as stale shared files.
func removeStaleFiles(dir string, expectedFiles map[string]bool) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		if !expectedFiles[entry.Name()] {
			if err := os.Remove(filepath.Join(dir, entry.Name())); err != nil {
				return fmt.Errorf("failed to remove stale file %s: %w", entry.Name(), err)
			}
		}
	}
	return nil
}

// sanitizeFilename converts a topic string to a safe filename.
func sanitizeFilename(s string) string {
	safe := sanitizeFilenameRe.ReplaceAllString(s, "-")
	safe = strings.Trim(safe, "-")
	if safe == "" {
		safe = "unnamed"
	}
	return safe
}

// yamlEscape wraps a string in quotes if it contains YAML-special characters.
func yamlEscape(s string) string {
	if strings.ContainsAny(s, ":#{}[]|>&*!%@`'\"\\,\n") {
		escaped := strings.ReplaceAll(s, `"`, `\"`)
		return `"` + escaped + `"`
	}
	return s
}

// yamlUnescape removes surrounding quotes from a YAML string value.
func yamlUnescape(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		s = s[1 : len(s)-1]
		s = strings.ReplaceAll(s, `\"`, `"`)
	}
	return s
}

// firstLine returns the first line of a string.
func firstLine(s string) string {
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		return s[:idx]
	}
	return s
}
