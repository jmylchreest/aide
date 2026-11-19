package main

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/jmylchreest/aide/aide/pkg/memory"

	"github.com/fatih/color"
	"github.com/mattn/go-runewidth"
	"github.com/olekukonko/tablewriter/pkg/twwarp"
	"golang.org/x/term"

	"github.com/jmylchreest/aide/aide/pkg/blueprint"
	"github.com/jmylchreest/aide/aide/pkg/config"
	"github.com/jmylchreest/aide/aide/pkg/grammar"
	"github.com/jmylchreest/aide/aide/pkg/store"
)

func cmdBlueprint(dbPath string, args []string) error {
	return dispatchSubcmd("blueprint", args, printBlueprintUsage, []subcmd{
		{name: "list", handler: func(a []string) error { return blueprintList() }},
		{name: "show", handler: func(a []string) error {
			if len(a) < 1 {
				return fmt.Errorf("usage: aide blueprint show <name>")
			}
			return blueprintShow(a[0], dbPath)
		}},
		{name: "migrate", handler: func(a []string) error { return blueprintMigrate(dbPath, a) }},
		{name: "import", handler: func(a []string) error { return blueprintImport(dbPath, a) }},
	})
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
	registries = append(registries, config.Get().Blueprints.Registries...)

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
	var bundle []*blueprint.Blueprint
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

			bundle = append(bundle, bp)
		}
	}

	if err := blueprint.ValidateBundle(bundle); err != nil {
		return err
	}
	if err := validateBlueprintImport(backend, bundle, force); err != nil {
		return err
	}
	for _, bp := range bundle {
		result, err := importBlueprint(backend, bp, force, dryRun)
		if err != nil {
			return err
		}
		allResults = append(allResults, result)
	}
	printImportSummary(allResults, dryRun)
	return nil
}

func validateBlueprintImport(backend *Backend, bundle []*blueprint.Blueprint, force bool) error {
	// Check stored blueprint collisions before writing any part of the bundle.
	for _, bp := range bundle {
		for _, d := range bp.Decisions {
			existing, err := backend.GetDecision(d.Topic)
			if err != nil && err != store.ErrNotFound {
				return err
			}
			if existing != nil && strings.HasPrefix(existing.DecidedBy, "blueprint:") && !blueprintOwns(existing, bp.Name) && !blueprint.SameContent(existing, bp.DecisionRecord(d)) {
				return fmt.Errorf("topic %q conflicts with %s; use distinct topics and explicit supersedes", d.Topic, existing.DecidedBy)
			}
		}
	}
	existingDecisions, err := backend.ListDecisions()
	if err != nil {
		return err
	}
	contexts := map[string]memory.DecisionContext{}
	for topic, d := range latestDecisionsByTopic(existingDecisions) {
		contexts[topic] = d.DecisionContext
	}
	for _, bp := range bundle {
		for _, d := range bp.Decisions {
			existing, err := backend.GetDecision(d.Topic)
			if err == store.ErrNotFound || force || (err == nil && blueprintOwns(existing, bp.Name)) {
				contexts[d.Topic] = bp.EffectiveContext(d)
			}
		}
	}
	if err := memory.ValidateSupersession(contexts); err != nil {
		return err
	}
	return nil
}

func importBlueprint(backend *Backend, bp *blueprint.Blueprint, force, dryRun bool) (blueprint.ImportResult, error) {
	result := blueprint.ImportResult{BlueprintName: bp.Name}
	provenance := "blueprint:" + bp.Name + "@" + bp.Version
	provenancePrefix := "blueprint:" + bp.Name + "@"

	for _, d := range bp.Decisions {
		context := bp.EffectiveContext(d)
		existing, err := backend.GetDecision(d.Topic)
		if err != nil && err != store.ErrNotFound {
			return result, fmt.Errorf("check existing decision %s: %w", d.Topic, err)
		}

		if err == nil && existing != nil && blueprint.SameContent(existing, bp.DecisionRecord(d)) {
			result.Skipped++
			result.SkippedTopics = append(result.SkippedTopics, d.Topic)
			continue
		}
		if err == nil && existing != nil && !force {
			// Check if this decision was imported from the same blueprint.
			// Legacy provenance: "blueprint:go" (no version).
			// Current provenance: "blueprint:go@1.0.0".
			legacyProvenance := "blueprint:" + bp.Name
			fromThisBlueprint := strings.HasPrefix(existing.DecidedBy, provenancePrefix) ||
				existing.DecidedBy == legacyProvenance

			if fromThisBlueprint {
				// Precedence participates: a blueprint that only re-weights a
				// decision must still be able to upgrade an earlier import.
				contentChanged := existing.Decision != d.Decision ||
					existing.Rationale != d.Rationale ||
					existing.Details != d.Details ||
					existing.Precedence != bp.EffectivePrecedence(d) ||
					!reflect.DeepEqual(existing.DecisionContext, context) || !reflect.DeepEqual(existing.References, d.References)

				// Extract existing version; legacy (unversioned) is always superseded.
				existingVer := "0.0.0"
				if strings.HasPrefix(existing.DecidedBy, provenancePrefix) {
					existingVer = strings.TrimPrefix(existing.DecidedBy, provenancePrefix)
				}

				comparison, err := compareVersions(bp.Version, existingVer)
				if err != nil {
					return result, fmt.Errorf("compare blueprint versions: %w", err)
				}
				if comparison > 0 && contentChanged {
					// Newer blueprint version with changed content — supersede.
					if !dryRun {
						precedence := bp.EffectivePrecedence(d)
						_, err = backend.SetDecision(d.Topic, d.Decision, d.Rationale, d.Details, provenance, d.References, &precedence, &context)
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

		precedence := bp.EffectivePrecedence(d)
		_, err = backend.SetDecision(d.Topic, d.Decision, d.Rationale, d.Details, provenance, d.References, &precedence, &context)
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

	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || width <= 0 {
		width, _ = strconv.Atoi(os.Getenv("COLUMNS"))
	}
	return renderBlueprintList(color.Output, blueprints, width, !color.NoColor)
}

func renderBlueprintList(w io.Writer, blueprints []*blueprint.Blueprint, width int, bold bool) error {
	if width <= 0 {
		width = 80
	}
	// Keep prose readable even on very wide terminals.
	width = min(width, 100)
	blueprints = append([]*blueprint.Blueprint(nil), blueprints...)
	sort.Slice(blueprints, func(i, j int) bool {
		return blueprints[i].Name < blueprints[j].Name
	})

	nameStyle := color.New(color.Bold)
	nameStyle.DisableColor()
	if bold {
		nameStyle.EnableColor()
	}
	var out strings.Builder
	writeLine := func(text, indent string) {
		if len(indent) >= width {
			indent = ""
		}
		contentWidth := width - len(indent)
		lines, _ := twwarp.WrapString(text, contentWidth)
		for _, line := range lines {
			// Long identifiers must also fit when there is no word boundary.
			for _, part := range strings.Split(runewidth.Wrap(line, contentWidth), "\n") {
				fmt.Fprintln(&out, indent+part)
			}
		}
	}
	for _, bp := range blueprints {
		metadata := fmt.Sprintf("v%s | %d decisions", bp.Version, len(bp.Decisions))
		if len(bp.Decisions) == 0 && len(bp.Includes) > 0 {
			metadata = fmt.Sprintf("v%s | bundle", bp.Version)
		}
		gap := width - runewidth.StringWidth(bp.Name) - runewidth.StringWidth(metadata)
		if gap >= 2 {
			fmt.Fprintln(&out, nameStyle.Sprint(bp.Name)+strings.Repeat(" ", gap)+metadata)
		} else {
			for _, line := range strings.Split(runewidth.Wrap(bp.Name, width), "\n") {
				fmt.Fprintln(&out, nameStyle.Sprint(line))
			}
			lines, _ := twwarp.WrapString(metadata, width)
			for _, line := range lines {
				for _, part := range strings.Split(runewidth.Wrap(line, width), "\n") {
					fmt.Fprintln(&out, strings.Repeat(" ", max(0, width-runewidth.StringWidth(part)))+part)
				}
			}
		}
		writeLine(bp.Description, "  ")
		if len(bp.Includes) > 0 {
			writeLine("Includes: "+strings.Join(bp.Includes, ", "), "  ")
		}
		fmt.Fprintln(&out)
	}
	writeLine(fmt.Sprintf("%d embedded blueprints available", len(blueprints)), "")
	_, err := io.WriteString(w, out.String())
	return err
}

func blueprintShow(name, dbPath string) error {
	localDir := blueprintOverrideDir(dbPath)
	bp, source, err := blueprint.Resolve(name, localDir, config.Get().Blueprints.Registries)
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
	projectRoot := store.ProjectRootFromDB(dbPath)

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
	if marker.Validate() != nil {
		return false
	}
	maxDepth := marker.MaxDepth

	switch marker.Check {
	case grammar.MarkerCheckDirectory:
		return grammar.MarkerExistsInProject(root, marker)

	case grammar.MarkerCheckFile:
		if maxDepth == 0 {
			return grammar.MarkerExistsInProject(root, marker)
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
	projectRoot := store.ProjectRootFromDB(dbPath)
	return filepath.Join(projectRoot, ".aide", "blueprints")
}

func printBlueprintUsage() {
	fmt.Println(`aide blueprint - Manage and import best-practice decision blueprints

Usage:
  aide blueprint <subcommand> [arguments]

Subcommands:
  import     Import blueprint decisions into the project
  migrate    Annotate unchanged installed blueprint decisions [--dry-run]
  list       List embedded blueprints
  show       Preview a blueprint's decisions

Import:
  aide blueprint import [flags] [blueprints...]

  Sources:
    go                           Resolve via chain (local → embedded → registries)
    ./path/to/custom.json        Load from local file
    https://example.com/bp.json  Fetch from URL

  Flags:
    --detect          Auto-detect blueprints from project markers
    --force           Overwrite user decisions; conflicting blueprint definitions still fail
    --dry-run         Show what would happen without writing
    --registry=URL    Try this registry before configured registries

  Named imports and previews use blueprints.registries from global/project config.
  Local overrides and embedded blueprints take precedence over all registries.

Examples:
  aide blueprint list                           # List embedded blueprints
  aide blueprint show go                        # Preview Go blueprint decisions
  aide blueprint import go                      # Import Go best practices
  aide blueprint import go rust                 # Import multiple
  aide blueprint import go go-github-actions    # With CI/CD patterns
  aide blueprint import --detect                # Auto-detect from project markers
  aide blueprint import --dry-run go            # See what would be imported
  aide blueprint import ./our-practices.json    # Import from local file`)
}

func blueprintOwns(d *memory.Decision, name string) bool {
	return d.DecidedBy == "blueprint:"+name || strings.HasPrefix(d.DecidedBy, "blueprint:"+name+"@")
}

// blueprintMigrate annotates only unchanged, recognisably blueprint-owned
// decisions. It neither installs missing decisions nor changes their wording.
func blueprintMigrate(dbPath string, args []string) error {
	b, err := NewBackend(dbPath)
	if err != nil {
		return err
	}
	defer b.Close()
	bps, err := blueprint.ListEmbedded()
	if err != nil {
		return err
	}
	dry := hasFlag(args, "--dry-run")
	changed, skipped := 0, 0
	for _, bp := range bps {
		for _, d := range bp.Decisions {
			previous, err := b.GetDecision(d.Topic)
			if err == store.ErrNotFound {
				continue
			}
			if err != nil {
				return err
			}
			if !blueprintOwns(previous, bp.Name) {
				continue
			}
			desired := bp.DecisionRecord(d)
			// Existing explicit metadata belongs to its author; leave it intact.
			if previous.Applicability != nil || len(previous.Supersedes) > 0 {
				continue
			}
			oldContext := desired.DecisionContext
			desired.DecisionContext = previous.DecisionContext
			if !blueprint.SameContent(previous, desired) {
				fmt.Printf("Skipped modified decision: %s\n", d.Topic)
				skipped++
				continue
			}
			if !dry {
				if _, err := b.SetDecision(previous.Topic, previous.Decision, previous.Rationale, previous.Details, previous.DecidedBy, previous.References, &previous.Precedence, &oldContext); err != nil {
					return err
				}
			}
			fmt.Printf("%s: [%s]\n", d.Topic, oldContext.Applicability.Label())
			changed++
		}
	}
	fmt.Printf("Applicability migration (dry-run=%t): %d annotated, %d modified decisions skipped\n", dry, changed, skipped)
	return nil
}
