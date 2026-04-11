package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/olekukonko/tablewriter"
	"github.com/olekukonko/tablewriter/renderer"
	"github.com/olekukonko/tablewriter/tw"

	"github.com/jmylchreest/aide/aide/pkg/blueprint"
	"github.com/jmylchreest/aide/aide/pkg/grammar"
	"github.com/jmylchreest/aide/aide/pkg/store"
)

func cmdBlueprint(dbPath string, args []string) error {
	if len(args) == 0 || hasFlag(args, "--help") || hasFlag(args, "-h") {
		printBlueprintUsage()
		return nil
	}

	subcmd := args[0]
	subargs := args[1:]

	switch subcmd {
	case "list":
		return blueprintList()
	case "show":
		if len(subargs) < 1 {
			return fmt.Errorf("usage: aide blueprint show <name>")
		}
		return blueprintShow(subargs[0], dbPath)
	case "import":
		return blueprintImport(dbPath, subargs)
	case "help", "-h", "--help":
		printBlueprintUsage()
		return nil
	default:
		return fmt.Errorf("unknown blueprint subcommand: %s", subcmd)
	}
}

func blueprintImport(dbPath string, args []string) error {
	if len(args) == 0 && !hasFlag(args, "--detect") {
		return fmt.Errorf("usage: aide blueprint import [--detect] [names...]")
	}

	force := hasFlag(args, "--force")
	dryRun := hasFlag(args, "--dry-run")
	detect := hasFlag(args, "--detect")
	registryFlag := parseFlag(args, "--registry=")

	var registries []string
	if registryFlag != "" {
		registries = append(registries, registryFlag)
	}

	localDir := blueprintOverrideDir(dbPath)

	var names []string
	if detect {
		names = detectBlueprints(dbPath)
		if len(names) == 0 {
			fmt.Println("No blueprints detected for this project.")
			return nil
		}
		fmt.Printf("Detected: %s\n\n", strings.Join(names, ", "))
	}

	for _, arg := range args {
		if strings.HasPrefix(arg, "--") {
			continue
		}
		names = append(names, arg)
	}

	if len(names) == 0 {
		fmt.Println("No blueprints specified. Use --detect or provide names.")
		return nil
	}

	backend, err := NewBackend(dbPath)
	if err != nil {
		return err
	}
	defer backend.Close()

	var allResults []blueprint.ImportResult
	imported := make(map[string]bool) // deduplicate across includes

	for _, name := range names {
		var chain []*blueprint.Blueprint

		switch {
		case strings.HasPrefix(name, "http://") || strings.HasPrefix(name, "https://"):
			bp, err := blueprint.LoadFromURL(name)
			if err != nil {
				return fmt.Errorf("load %s: %w", name, err)
			}
			chain = []*blueprint.Blueprint{bp}

		case strings.HasSuffix(name, ".json") || strings.Contains(name, string(os.PathSeparator)) || strings.HasPrefix(name, "."):
			bp, err := blueprint.LoadFromFile(name)
			if err != nil {
				return fmt.Errorf("load %s: %w", name, err)
			}
			chain = []*blueprint.Blueprint{bp}

		default:
			resolved, err := blueprint.ResolveWithIncludes(name, localDir, registries)
			if err != nil {
				return err
			}
			chain = resolved
		}

		for _, bp := range chain {
			if imported[bp.Name] {
				continue
			}
			imported[bp.Name] = true

			result, err := importBlueprint(backend, bp, force, dryRun)
			if err != nil {
				return err
			}
			allResults = append(allResults, result)
		}
	}

	printImportSummary(allResults, dryRun)
	return nil
}

func importBlueprint(backend *Backend, bp *blueprint.Blueprint, force, dryRun bool) (blueprint.ImportResult, error) {
	result := blueprint.ImportResult{BlueprintName: bp.Name}
	provenance := "blueprint:" + bp.Name + "@" + bp.Version
	provenancePrefix := "blueprint:" + bp.Name + "@"

	for _, d := range bp.Decisions {
		existing, err := backend.GetDecision(d.Topic)
		if err != nil && err != store.ErrNotFound {
			return result, fmt.Errorf("check existing decision %s: %w", d.Topic, err)
		}

		if err == nil && existing != nil && !force {
			// Check if this decision was imported from the same blueprint.
			// Legacy provenance: "blueprint:go" (no version).
			// Current provenance: "blueprint:go@1.0.0".
			legacyProvenance := "blueprint:" + bp.Name
			fromThisBlueprint := strings.HasPrefix(existing.DecidedBy, provenancePrefix) ||
				existing.DecidedBy == legacyProvenance

			if fromThisBlueprint {
				contentChanged := existing.Decision != d.Decision ||
					existing.Rationale != d.Rationale ||
					existing.Details != d.Details

				// Extract existing version; legacy (unversioned) is always superseded.
				existingVer := "0.0.0"
				if strings.HasPrefix(existing.DecidedBy, provenancePrefix) {
					existingVer = strings.TrimPrefix(existing.DecidedBy, provenancePrefix)
				}

				if compareVersions(bp.Version, existingVer) > 0 && contentChanged {
					// Newer blueprint version with changed content — supersede.
					if !dryRun {
						_, err = backend.SetDecision(d.Topic, d.Decision, d.Rationale, d.Details, provenance, d.References)
						if err != nil {
							return result, fmt.Errorf("update decision %s: %w", d.Topic, err)
						}
					}
					result.Updated++
					continue
				}
			}
			// User-set decision or same version/content — skip.
			result.Skipped++
			result.SkippedTopics = append(result.SkippedTopics, d.Topic)
			continue
		}

		if dryRun {
			result.Imported++
			continue
		}

		_, err = backend.SetDecision(d.Topic, d.Decision, d.Rationale, d.Details, provenance, d.References)
		if err != nil {
			return result, fmt.Errorf("set decision %s: %w", d.Topic, err)
		}
		result.Imported++
	}

	return result, nil
}

func printImportSummary(results []blueprint.ImportResult, dryRun bool) {
	totalImported := 0
	totalUpdated := 0
	totalSkipped := 0

	w := newTabWriter()
	for _, r := range results {
		totalImported += r.Imported
		totalUpdated += r.Updated
		totalSkipped += r.Skipped

		if r.Imported == 0 && r.Updated == 0 && r.Skipped == 0 {
			continue
		}
		status := fmt.Sprintf("%d new", r.Imported)
		if r.Updated > 0 {
			status += fmt.Sprintf(", %d updated", r.Updated)
		}
		if r.Skipped > 0 {
			status += fmt.Sprintf(", %d skipped", r.Skipped)
		}
		fmt.Fprintf(w, "  %s\t%s\n", r.BlueprintName, status)
	}
	w.Flush()

	fmt.Println()
	if dryRun {
		fmt.Printf("%d would be imported, %d would be updated (dry run)\n", totalImported, totalUpdated)
	} else {
		fmt.Printf("%d imported, %d updated\n", totalImported, totalUpdated)
	}
	if totalSkipped > 0 {
		fmt.Printf("%d skipped (already set)\n", totalSkipped)
	}
}

func blueprintList() error {
	blueprints, err := blueprint.ListEmbedded()
	if err != nil {
		return err
	}

	// Sort: base blueprints (no includes) first, then group by includes, then by name.
	sort.Slice(blueprints, func(i, j int) bool {
		iBase := len(blueprints[i].Includes) == 0
		jBase := len(blueprints[j].Includes) == 0
		if iBase != jBase {
			return iBase
		}
		iInc := strings.Join(blueprints[i].Includes, ",")
		jInc := strings.Join(blueprints[j].Includes, ",")
		if iInc != jInc {
			return iInc < jInc
		}
		return blueprints[i].Name < blueprints[j].Name
	})

	cfg := tablewriter.NewConfigBuilder().
		WithHeaderAutoFormat(tw.Off).
		WithRowAutoWrap(tw.WrapNormal).
		WithRowAlignment(tw.AlignLeft).
		WithHeaderAlignment(tw.AlignLeft).
		ForColumn(4).WithMaxWidth(50).Build().
		Build()

	table := tablewriter.NewTable(os.Stdout,
		tablewriter.WithConfig(cfg),
		tablewriter.WithRenderer(renderer.NewBlueprint(tw.Rendition{
			Borders: tw.Border{Left: tw.Off, Right: tw.Off, Top: tw.Off, Bottom: tw.Off},
			Settings: tw.Settings{
				Separators: tw.Separators{
					BetweenRows:    tw.Off,
					BetweenColumns: tw.On,
				},
				Lines: tw.Lines{
					ShowHeaderLine: tw.On,
					ShowTop:        tw.Off,
					ShowBottom:     tw.Off,
				},
			},
		})),
	)
	table.Header([]string{"NAME", "VERSION", "DECISIONS", "INCLUDES", "DESCRIPTION"})

	for _, bp := range blueprints {
		includes := "—"
		if len(bp.Includes) > 0 {
			includes = strings.Join(bp.Includes, ", ")
		}
		table.Append([]string{
			bp.Name,
			bp.Version,
			fmt.Sprintf("%d", len(bp.Decisions)),
			includes,
			bp.Description,
		})
	}
	table.Render()

	fmt.Printf("\n%d blueprints available\n", len(blueprints))
	return nil
}

func blueprintShow(name, dbPath string) error {
	localDir := blueprintOverrideDir(dbPath)
	bp, source, err := blueprint.Resolve(name, localDir, nil)
	if err != nil {
		return err
	}

	fmt.Printf("%s (%s) — %s\n", bp.DisplayName, source, bp.Description)
	fmt.Printf("Version: %s\n", bp.Version)
	if len(bp.Includes) > 0 {
		fmt.Printf("Includes: %s\n", strings.Join(bp.Includes, ", "))
	}
	fmt.Printf("Decisions: %d\n\n", len(bp.Decisions))

	for _, d := range bp.Decisions {
		fmt.Printf("  %s\n", d.Topic)
		fmt.Printf("    %s\n", d.Decision)
		if d.Rationale != "" {
			fmt.Printf("    Why: %s\n", truncate(d.Rationale, 120))
		}
		fmt.Println()
	}
	return nil
}

// detectBlueprints uses the pack index project markers to discover which
// languages and tools are present, then maps them to available blueprints.
func detectBlueprints(dbPath string) []string {
	projectRoot := filepath.Dir(filepath.Dir(filepath.Dir(dbPath)))

	reg := grammar.DefaultPackRegistry()
	allMarkers := reg.ProjectMarkers()

	// Collect unique pack names and labels detected via markers.
	detectedPacks := make(map[string]bool)
	detectedLabels := make(map[string]bool)

	for _, marker := range allMarkers {
		if marker.Check == grammar.MarkerCheckSibling {
			continue
		}
		if markerExistsWithDepth(projectRoot, marker) {
			if marker.Pack != "" {
				detectedPacks[marker.Pack] = true
			}
			if marker.Label != "" {
				detectedLabels[marker.Label] = true
			}
		}
	}

	// Map detected packs/labels to available blueprints.
	seen := make(map[string]bool)
	var detected []string

	for pack := range detectedPacks {
		if _, err := blueprint.LoadEmbedded(pack); err == nil && !seen[pack] {
			detected = append(detected, pack)
			seen[pack] = true
		}
	}

	for label := range detectedLabels {
		if _, err := blueprint.LoadEmbedded(label); err == nil && !seen[label] {
			detected = append(detected, label)
			seen[label] = true
		}
		// Check for language-specific variants (e.g., go-github-actions).
		for pack := range detectedPacks {
			compound := pack + "-" + label
			if _, err := blueprint.LoadEmbedded(compound); err == nil && !seen[compound] {
				detected = append(detected, compound)
				seen[compound] = true
			}
		}
	}

	return detected
}

// markerExistsWithDepth checks if a project marker exists, respecting max_depth.
func markerExistsWithDepth(root string, marker grammar.ProjectMarker) bool {
	maxDepth := marker.MaxDepth

	switch marker.Check {
	case grammar.MarkerCheckDirectory:
		path := filepath.Join(root, marker.File)
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
			return false
		}
		entries, err := os.ReadDir(path)
		return err == nil && len(entries) > 0

	case grammar.MarkerCheckFile:
		if maxDepth == 0 {
			info, err := os.Stat(filepath.Join(root, marker.File))
			return err == nil && !info.IsDir()
		}

		target := marker.File
		found := false
		depth := -1

		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || found {
				return filepath.SkipDir
			}

			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				return nil
			}

			if d.IsDir() {
				currentDepth := strings.Count(rel, string(os.PathSeparator))
				if rel == "." {
					currentDepth = 0
				}
				if maxDepth > 0 && currentDepth >= maxDepth {
					return filepath.SkipDir
				}
				// Skip common noise directories.
				base := d.Name()
				if base == "node_modules" || base == "vendor" || base == ".git" || base == ".aide" {
					return filepath.SkipDir
				}
				return nil
			}

			if filepath.Base(path) == target || rel == target {
				depth = strings.Count(rel, string(os.PathSeparator))
				if maxDepth < 0 || depth <= maxDepth {
					found = true
					return filepath.SkipAll
				}
			}
			return nil
		})

		_ = depth
		return found

	default:
		return false
	}
}

func blueprintOverrideDir(dbPath string) string {
	projectRoot := filepath.Dir(filepath.Dir(filepath.Dir(dbPath)))
	return filepath.Join(projectRoot, ".aide", "blueprints")
}

func printBlueprintUsage() {
	fmt.Println(`aide blueprint - Manage and import best-practice decision blueprints

Usage:
  aide blueprint <subcommand> [arguments]

Subcommands:
  import     Import blueprint decisions into the project
  list       List available blueprints
  show       Preview a blueprint's decisions

Import:
  aide blueprint import [flags] [blueprints...]

  Sources:
    go                           Resolve via chain (local → embedded → registries)
    ./path/to/custom.json        Load from local file
    https://example.com/bp.json  Fetch from URL

  Flags:
    --detect          Auto-detect blueprints from project markers
    --force           Overwrite existing decisions on conflict
    --dry-run         Show what would happen without writing
    --registry=URL    Add a one-off registry for this invocation

Examples:
  aide blueprint list                           # List available blueprints
  aide blueprint show go                        # Preview Go blueprint decisions
  aide blueprint import go                      # Import Go best practices
  aide blueprint import go rust                 # Import multiple
  aide blueprint import go go-github-actions    # With CI/CD patterns
  aide blueprint import --detect                # Auto-detect from project markers
  aide blueprint import --dry-run go            # See what would be imported
  aide blueprint import ./our-practices.json    # Import from local file`)
}
