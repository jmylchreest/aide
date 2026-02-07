package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/jmylchreest/aide/aide/pkg/memory"
)

func cmdExport(dbPath string, args []string) error {
	if hasFlag(args, "--help") || hasFlag(args, "-h") {
		printExportUsage()
		return nil
	}

	format := "markdown"
	output := ".aide/memory/exports"
	toStdout := hasFlag(args, "--stdout")

	if f := parseFlag(args, "--format="); f != "" {
		format = f
	}
	if o := parseFlag(args, "--output="); o != "" {
		output = o
	}

	backend, err := NewBackend(dbPath)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer backend.Close()

	memories, err := backend.ListMemories("", 0) // empty category = all, 0 = no limit
	if err != nil {
		return fmt.Errorf("failed to list memories: %w", err)
	}

	// Output to stdout if --stdout flag is set
	if toStdout {
		switch format {
		case "markdown":
			exportMarkdownToWriter(memories, os.Stdout)
		case "json":
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			if err := enc.Encode(memories); err != nil {
				return fmt.Errorf("failed to encode JSON: %w", err)
			}
		default:
			return fmt.Errorf("unknown format: %s", format)
		}
		return nil
	}

	// Output to files
	if err := os.MkdirAll(output, 0o755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	switch format {
	case "markdown":
		if err := exportMarkdown(memories, output); err != nil {
			return fmt.Errorf("failed to export markdown: %w", err)
		}
	case "json":
		if err := exportJSON(memories, output); err != nil {
			return fmt.Errorf("failed to export JSON: %w", err)
		}
	default:
		return fmt.Errorf("unknown format: %s", format)
	}

	fmt.Printf("Exported to %s\n", output)
	return nil
}

func exportMarkdown(memories []*memory.Memory, output string) error {
	categories := map[memory.Category][]*memory.Memory{}
	for _, m := range memories {
		categories[m.Category] = append(categories[m.Category], m)
	}

	for cat, mems := range categories {
		filename := fmt.Sprintf("%s/%s.md", output, cat)
		f, err := os.Create(filename)
		if err != nil {
			return fmt.Errorf("failed to create file %s: %w", filename, err)
		}

		fmt.Fprintf(f, "# %s\n\n", titleCase(string(cat)))
		for _, m := range mems {
			fmt.Fprintf(f, "## %s\n\n%s\n\n", m.CreatedAt.Format("2006-01-02 15:04:05"), m.Content)
			if len(m.Tags) > 0 {
				fmt.Fprintf(f, "Tags: %s\n\n", strings.Join(m.Tags, ", "))
			}
			fmt.Fprintln(f, "---")
		}

		if err := f.Close(); err != nil {
			return fmt.Errorf("failed to close file %s: %w", filename, err)
		}
	}
	return nil
}

// exportMarkdownToWriter writes all memories as markdown to a writer (e.g., stdout).
// Format is optimized for context injection - compact but readable.
func exportMarkdownToWriter(memories []*memory.Memory, w *os.File) {
	if len(memories) == 0 {
		fmt.Fprintln(w, "No memories stored.")
		return
	}

	// Group by category
	categories := map[memory.Category][]*memory.Memory{}
	for _, m := range memories {
		categories[m.Category] = append(categories[m.Category], m)
	}

	fmt.Fprintln(w, "# Stored Memories")
	fmt.Fprintln(w)

	for cat, mems := range categories {
		fmt.Fprintf(w, "## %s\n\n", titleCase(string(cat)))
		for _, m := range mems {
			fmt.Fprintf(w, "- **[%s]** %s", m.CreatedAt.Format("2006-01-02"), m.Content)
			if len(m.Tags) > 0 {
				fmt.Fprintf(w, " _(tags: %s)_", strings.Join(m.Tags, ", "))
			}
			fmt.Fprintln(w)
		}
		fmt.Fprintln(w)
	}
}

func exportJSON(memories []*memory.Memory, output string) error {
	filename := fmt.Sprintf("%s/memories.json", output)
	f, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create JSON file: %w", err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(memories); err != nil {
		return fmt.Errorf("failed to encode JSON: %w", err)
	}
	return nil
}

func printExportUsage() {
	fmt.Println(`aide export - Export memories to files

Usage:
  aide export [options]

Options:
  --format=FORMAT    Output format: markdown, json (default: markdown)
  --output=DIR       Output directory (default: .aide/memory/exports)
  --stdout           Write to stdout instead of files

Examples:
  aide export
  aide export --format=json --output=./exports
  aide export --stdout --format=json`)
}
