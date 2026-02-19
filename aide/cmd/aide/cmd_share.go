// Package main provides the share command for exporting/importing aide data
// as git-friendly markdown files with YAML frontmatter.
//
// Shared files live in .aide/shared/ and are designed to be committed to git.
// Each file is self-contained and usable as LLM context without aide.
//
// Layout:
//
//	.aide/shared/
//	  decisions/
//	    <topic>.md          # One file per decision topic
//	  memories/
//	    <category>.md       # One file per category (learning, pattern, gotcha, etc.)
package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/memory"
)

// cmdShare dispatches share subcommands.
func cmdShare(dbPath string, args []string) error {
	if len(args) < 1 || hasFlag(args, "--help") || hasFlag(args, "-h") {
		printShareUsage()
		return nil
	}

	subcmd := args[0]
	subargs := args[1:]

	switch subcmd {
	case "export":
		return cmdShareExport(dbPath, subargs)
	case "import":
		return cmdShareImport(dbPath, subargs)
	case "help", "-h", "--help":
		printShareUsage()
		return nil
	default:
		return fmt.Errorf("unknown share subcommand: %s", subcmd)
	}
}

func printShareUsage() {
	fmt.Println(`aide share - Export/import aide data as git-friendly markdown

Shared files are written to .aide/shared/ and designed to be committed to git.
They use YAML frontmatter + markdown body, so they work as LLM context without aide.

Usage:
  aide share <subcommand> [arguments]

Subcommands:
  export     Export decisions and memories to .aide/shared/
  import     Import decisions and memories from .aide/shared/

Options:
  export:
    --decisions          Export decisions only
    --memories           Export memories only (project-scoped by default)
    --all-memories       Export all memories (including session-specific)
    --output=DIR         Output directory (default: .aide/shared)

  import:
    --decisions          Import decisions only
    --memories           Import memories only
    --input=DIR          Input directory (default: .aide/shared)
    --dry-run            Show what would be imported without changing anything

Examples:
  aide share export                          # Export everything
  aide share export --decisions              # Decisions only
  aide share import                          # Import everything
  aide share import --dry-run                # Preview import`)
}

// --- Export ---

// projectRootFromDB derives the project root from the database path.
// dbPath is always <projectRoot>/.aide/memory/store.db
func projectRootFromDB(dbPath string) string {
	// .aide/memory/store.db -> .aide/memory -> .aide -> <projectRoot>
	return filepath.Dir(filepath.Dir(filepath.Dir(dbPath)))
}

func cmdShareExport(dbPath string, args []string) error {
	projectRoot := projectRootFromDB(dbPath)
	outputDir := filepath.Join(projectRoot, ".aide", "shared")

	if o := parseFlag(args, "--output="); o != "" {
		outputDir = o
	}

	decisionsOnly := hasFlag(args, "--decisions")
	memoriesOnly := hasFlag(args, "--memories")
	allMemories := hasFlag(args, "--all-memories")

	// Default: export both
	exportDecisions := !memoriesOnly
	exportMemories := !decisionsOnly

	backend, err := NewBackend(dbPath)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer backend.Close()

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

	// Remove stale decision files (deleted decisions)
	if err := removeStaleFiles(dir, expectedFiles); err != nil {
		return count, fmt.Errorf("failed to clean stale decisions: %w", err)
	}

	return count, nil
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
			if isShareableMemory(m) {
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

	// Remove stale category files (categories with no more shareable memories)
	if err := removeStaleFiles(dir, expectedFiles); err != nil {
		return total, fmt.Errorf("failed to clean stale memories: %w", err)
	}

	return total, nil
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

		// Metadata comment (parseable but invisible in rendered markdown)
		tags := ""
		if len(m.Tags) > 0 {
			tags = " tags=" + strings.Join(m.Tags, ",")
		}
		fmt.Fprintf(f, "<!-- aide:id=%s%s date=%s -->\n\n",
			m.ID, tags, m.CreatedAt.Format("2006-01-02"))

		// Full content
		fmt.Fprintln(f, m.Content)
		fmt.Fprintln(f)
		fmt.Fprintln(f, "---")
		fmt.Fprintln(f)
	}

	return nil
}

// isShareableMemory determines if a memory is worth sharing via git.
// Memories with scope:global, project:*, or certain categories are shareable.
// Session-specific memories (session:*) without project scope are excluded.
func isShareableMemory(m *memory.Memory) bool {
	// Always share gotchas, patterns, and decisions
	switch m.Category {
	case "gotcha", "pattern", "decision":
		return true
	}

	// Check tags for sharing signals
	for _, tag := range m.Tags {
		if tag == "scope:global" {
			return true
		}
		if strings.HasPrefix(tag, "project:") {
			return true
		}
	}

	return false
}

// --- Import ---

func cmdShareImport(dbPath string, args []string) error {
	projectRoot := projectRootFromDB(dbPath)
	inputDir := filepath.Join(projectRoot, ".aide", "shared")

	if i := parseFlag(args, "--input="); i != "" {
		inputDir = i
	}

	decisionsOnly := hasFlag(args, "--decisions")
	memoriesOnly := hasFlag(args, "--memories")
	dryRun := hasFlag(args, "--dry-run")

	importDecisions := !memoriesOnly
	importMemories := !decisionsOnly

	backend, err := NewBackend(dbPath)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer backend.Close()

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

	action := "Imported from"
	if dryRun {
		action = "Dry run from"
	}
	fmt.Printf("%s %s\n", action, inputDir)
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
		if os.IsNotExist(err) {
			return 0, 0, nil
		}
		return 0, 0, err
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
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
	if strings.HasPrefix(line, "topic:") {
		d.Topic = strings.TrimSpace(strings.TrimPrefix(line, "topic:"))
	} else if strings.HasPrefix(line, "decision:") {
		d.Decision = yamlUnescape(strings.TrimSpace(strings.TrimPrefix(line, "decision:")))
	} else if strings.HasPrefix(line, "decided_by:") {
		d.DecidedBy = strings.TrimSpace(strings.TrimPrefix(line, "decided_by:"))
	} else if strings.HasPrefix(line, "date:") {
		dateStr := strings.TrimSpace(strings.TrimPrefix(line, "date:"))
		if t, err := time.Parse("2006-01-02", dateStr); err == nil {
			d.CreatedAt = t
		}
	} else if strings.HasPrefix(line, "  - ") {
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
		if os.IsNotExist(err) {
			return 0, 0, nil
		}
		return 0, 0, err
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		memories, err := parseMemoriesMarkdown(filepath.Join(dir, entry.Name()))
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: skipping %s: %v\n", entry.Name(), err)
			continue
		}

		for _, m := range memories {
			// Check if memory already exists by ID
			if m.ID != "" {
				existing, getErr := b.GetMemory(m.ID)
				if getErr == nil && existing != nil {
					skipped++
					if dryRun {
						fmt.Printf("  skip memory: %s (exists)\n", truncate(m.Content, 50))
					}
					continue
				}
			}

			if dryRun {
				fmt.Printf("  import memory [%s]: %s\n", m.Category, truncate(m.Content, 50))
				imported++
				continue
			}

			_, err = b.AddMemory(m.Content, string(m.Category), m.Tags)
			if err != nil {
				return imported, skipped, fmt.Errorf("failed to import memory: %w", err)
			}
			imported++
		}
	}

	return imported, skipped, nil
}

// aideCommentRegex parses the <!-- aide:... --> metadata comments.
var aideCommentRegex = regexp.MustCompile(`<!--\s*aide:id=(\S+)(?:\s+tags=(\S+))?(?:\s+date=(\S+))?\s*-->`)

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
					if t, err := time.Parse("2006-01-02", matches[3]); err == nil {
						current.CreatedAt = t
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
		if os.IsNotExist(err) {
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
	// Replace non-alphanumeric chars (except hyphen/underscore) with hyphens
	re := regexp.MustCompile(`[^a-zA-Z0-9_-]+`)
	safe := re.ReplaceAllString(s, "-")
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
