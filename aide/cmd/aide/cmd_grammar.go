package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jmylchreest/aide/aide/pkg/aideignore"
	"github.com/jmylchreest/aide/aide/pkg/code"
	"github.com/jmylchreest/aide/aide/pkg/grammar"
)

// cmdGrammarDispatcher routes grammar subcommands.
func cmdGrammarDispatcher(dbPath string, args []string) error {
	if len(args) < 1 {
		printGrammarUsage()
		return nil
	}

	subcmd := args[0]
	subargs := args[1:]

	switch subcmd {
	case "list", "ls":
		return cmdGrammarList(dbPath, subargs)
	case "install":
		return cmdGrammarInstall(dbPath, subargs)
	case "remove", "rm":
		return cmdGrammarRemove(dbPath, subargs)
	case "scan":
		return cmdGrammarScan(dbPath, subargs)
	case "help", "-h", "--help":
		printGrammarUsage()
		return nil
	default:
		return fmt.Errorf("unknown grammar subcommand: %s", subcmd)
	}
}

func printGrammarUsage() {
	fmt.Println(`aide grammar - Manage tree-sitter language grammars

Usage:
  aide grammar <subcommand> [arguments]

Subcommands:
  list       List available, installed, and built-in grammars
  install    Download and install a dynamic grammar
  remove     Remove a downloaded grammar from the local cache
  scan       Scan the project to detect languages and suggest grammars to install

Options:
  list:
    --installed      Show only installed grammars (builtin + dynamic)
    --available      Show only grammars available for download
    --json           Output as JSON

  install [language...]:
    --all            Install all available dynamic grammars
    --no-lock        Skip updating the lock file after install
    Downloads grammar shared libraries to .aide/grammars/
    When invoked with no arguments and no --all, installs from the lock file
    (` + grammar.LockFileName + `) if one exists.

  remove <language> [language...]:
    --all            Remove all downloaded dynamic grammars
    --no-lock        Skip updating the lock file after removal

  scan [path]:
    --json           Output as JSON
    Scans the project (or given path) for source files, detects languages,
    and reports which grammars are needed but not yet installed.

Lock file:
  Install and remove commands automatically maintain a lock file
  (` + grammar.LockFileName + `) at the project root. This file records
  the exact version and checksum of each installed dynamic grammar.
  Commit it to version control so team members can reproduce the same
  grammar set with 'aide grammar install' (no arguments).

Examples:
  aide grammar list                    # Show all grammars
  aide grammar list --installed        # Show only installed
  aide grammar scan                    # Scan project for needed grammars
  aide grammar scan ./src              # Scan specific directory
  aide grammar install ruby kotlin     # Install specific grammars
  aide grammar install --all           # Install all available
  aide grammar install                 # Install from lock file
  aide grammar install --no-lock       # Install without updating lock file
  aide grammar remove ruby             # Remove a grammar
  aide grammar remove --all            # Remove all dynamic grammars`)
}

// cmdGrammarList shows grammar status.
func cmdGrammarList(dbPath string, args []string) error {
	loader := newGrammarLoaderNoAuto(dbPath, nil)
	jsonOutput := hasFlag(args, "--json")
	onlyInstalled := hasFlag(args, "--installed")
	onlyAvailable := hasFlag(args, "--available")

	installed := loader.Installed()
	available := loader.Available()

	if jsonOutput {
		return grammarListJSON(installed, available, onlyInstalled, onlyAvailable)
	}

	// Build a unified view.
	type entry struct {
		name    string
		status  string
		version string
	}

	seen := make(map[string]bool)
	var entries []entry

	for _, info := range installed {
		seen[info.Name] = true
		if onlyAvailable {
			continue
		}
		status := "builtin"
		if !info.BuiltIn {
			status = "installed"
		}
		entries = append(entries, entry{
			name:    info.Name,
			status:  status,
			version: info.Version,
		})
	}

	if !onlyInstalled {
		// Add available-but-not-installed grammars.
		sort.Strings(available)
		for _, name := range available {
			if seen[name] {
				continue
			}
			entries = append(entries, entry{
				name:   name,
				status: "available",
			})
		}
	}

	if len(entries) == 0 {
		fmt.Println("No grammars found.")
		return nil
	}

	// Sort: builtin, installed, available.
	statusOrder := map[string]int{"builtin": 0, "installed": 1, "available": 2}
	sort.Slice(entries, func(i, j int) bool {
		oi, oj := statusOrder[entries[i].status], statusOrder[entries[j].status]
		if oi != oj {
			return oi < oj
		}
		return entries[i].name < entries[j].name
	})

	// Find max name width for alignment.
	maxName := 0
	for _, e := range entries {
		if len(e.name) > maxName {
			maxName = len(e.name)
		}
	}

	fmt.Printf("%-*s  %-10s  %s\n", maxName, "GRAMMAR", "STATUS", "VERSION")
	for _, e := range entries {
		ver := e.version
		if ver == "" {
			ver = "-"
		}
		fmt.Printf("%-*s  %-10s  %s\n", maxName, e.name, e.status, ver)
	}

	return nil
}

func grammarListJSON(installed []grammar.GrammarInfo, available []string, onlyInstalled, onlyAvailable bool) error {
	fmt.Print("[")
	first := true

	printEntry := func(name, status, version string) {
		if !first {
			fmt.Print(",")
		}
		first = false
		ver := version
		if ver == "" {
			ver = ""
		}
		fmt.Printf(`{"name":%q,"status":%q,"version":%q}`, name, status, ver)
	}

	seen := make(map[string]bool)
	for _, info := range installed {
		seen[info.Name] = true
	}

	if !onlyAvailable {
		for _, info := range installed {
			status := "builtin"
			if !info.BuiltIn {
				status = "installed"
			}
			printEntry(info.Name, status, info.Version)
		}
	}

	if !onlyInstalled {
		sort.Strings(available)
		for _, name := range available {
			if seen[name] {
				continue
			}
			printEntry(name, "available", "")
		}
	}

	fmt.Println("]")
	return nil
}

// cmdGrammarInstall downloads grammar shared libraries.
func cmdGrammarInstall(dbPath string, args []string) error {
	root := projectRoot(dbPath)
	loader := newGrammarLoaderNoAuto(dbPath, nil)
	ctx := context.Background()

	installAll := hasFlag(args, "--all")
	noLock := hasFlag(args, "--no-lock")

	var names []string
	if installAll {
		names = loader.Available()
		// Filter to only dynamic (not already builtin).
		var dynamic []string
		installed := make(map[string]bool)
		for _, info := range loader.Installed() {
			if info.BuiltIn {
				installed[info.Name] = true
			}
		}
		for _, name := range names {
			if !installed[name] {
				dynamic = append(dynamic, name)
			}
		}
		names = dynamic
	} else {
		for _, arg := range args {
			if strings.HasPrefix(arg, "--") {
				continue
			}
			names = append(names, grammar.NormaliseLang(arg))
		}
	}

	// No explicit names and no --all: try installing from lock file.
	if len(names) == 0 && !installAll {
		lf, err := grammar.ReadLockFile(root)
		if err != nil {
			return fmt.Errorf("reading lock file: %w", err)
		}
		if lf != nil && len(lf.Grammars) > 0 {
			fmt.Printf("Installing grammars from %s...\n", grammar.LockFileName)
			installed, err := loader.InstallFromLock(ctx, lf)
			for _, name := range installed {
				fmt.Printf("  %s... done\n", name)
			}
			if err != nil {
				return err
			}
			if len(installed) == 0 {
				fmt.Println("All locked grammars already installed.")
			}
			return nil
		}
		fmt.Println("No grammars to install. Specify language names, use --all, or create a lock file.")
		return nil
	}

	sort.Strings(names)
	var errors []string
	for _, name := range names {
		fmt.Printf("Installing %s... ", name)
		if err := loader.Install(ctx, name); err != nil {
			fmt.Printf("FAILED: %v\n", err)
			errors = append(errors, name)
		} else {
			fmt.Println("done")
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("failed to install: %s", strings.Join(errors, ", "))
	}

	// Update the lock file unless --no-lock was specified.
	if !noLock {
		lf := loader.GenerateLockFile()
		if len(lf.Grammars) > 0 {
			if err := grammar.WriteLockFile(root, lf); err != nil {
				fmt.Fprintf(os.Stderr, "warning: failed to update %s: %v\n", grammar.LockFileName, err)
			}
		}
	}

	return nil
}

// cmdGrammarRemove deletes downloaded grammar shared libraries.
func cmdGrammarRemove(dbPath string, args []string) error {
	root := projectRoot(dbPath)
	loader := newGrammarLoaderNoAuto(dbPath, nil)

	removeAll := hasFlag(args, "--all")
	noLock := hasFlag(args, "--no-lock")

	var names []string
	if removeAll {
		for _, info := range loader.Installed() {
			if !info.BuiltIn {
				names = append(names, info.Name)
			}
		}
	} else {
		for _, arg := range args {
			if strings.HasPrefix(arg, "--") {
				continue
			}
			names = append(names, grammar.NormaliseLang(arg))
		}
	}

	if len(names) == 0 {
		fmt.Println("No grammars to remove. Specify language names or use --all.")
		return nil
	}

	sort.Strings(names)
	var errors []string
	for _, name := range names {
		fmt.Printf("Removing %s... ", name)
		if err := loader.Remove(name); err != nil {
			fmt.Printf("FAILED: %v\n", err)
			errors = append(errors, name)
		} else {
			fmt.Println("done")
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("failed to remove: %s", strings.Join(errors, ", "))
	}

	// Update the lock file unless --no-lock was specified.
	if !noLock {
		lf := loader.GenerateLockFile()
		if len(lf.Grammars) > 0 {
			if err := grammar.WriteLockFile(root, lf); err != nil {
				fmt.Fprintf(os.Stderr, "warning: failed to update %s: %v\n", grammar.LockFileName, err)
			}
		} else {
			// All dynamic grammars removed — delete the lock file.
			lockPath := filepath.Join(root, grammar.LockFileName)
			if err := os.Remove(lockPath); err != nil && !os.IsNotExist(err) {
				fmt.Fprintf(os.Stderr, "warning: failed to remove %s: %v\n", grammar.LockFileName, err)
			}
		}
	}

	return nil
}

// cmdGrammarScan scans the project for languages and reports grammar status.
func cmdGrammarScan(dbPath string, args []string) error {
	root := projectRoot(dbPath)
	jsonOutput := hasFlag(args, "--json")

	// Allow overriding the scan path.
	for _, arg := range args {
		if !strings.HasPrefix(arg, "--") {
			root = arg
			break
		}
	}

	scanLog := log.New(os.Stderr, "[grammar] ", 0)
	loader := newGrammarLoaderNoAuto(dbPath, scanLog)
	ignore, err := aideignore.New(root)
	if err != nil {
		ignore = aideignore.NewFromDefaults()
	}

	statuses, err := grammar.ScanDetail(root, loader, code.DetectLanguage, ignore)
	if err != nil {
		return fmt.Errorf("scan failed: %w", err)
	}

	if len(statuses) == 0 {
		fmt.Println("No recognised source files found.")
		return nil
	}

	if jsonOutput {
		return grammarScanJSON(statuses)
	}

	// Find max name width.
	maxName := 0
	for _, s := range statuses {
		if len(s.Name) > maxName {
			maxName = len(s.Name)
		}
	}

	fmt.Printf("%-*s  %6s  %-10s  %s\n", maxName, "LANGUAGE", "FILES", "STATUS", "ACTION")
	for _, s := range statuses {
		action := "-"
		switch s.Status {
		case "available":
			action = "aide grammar install " + s.Name
		case "unavailable":
			action = "(no grammar available)"
		}
		fmt.Printf("%-*s  %6d  %-10s  %s\n", maxName, s.Name, s.Files, s.Status, action)
	}

	// Summary line.
	var needCount int
	for _, s := range statuses {
		if s.CanInstall {
			needCount++
		}
	}
	if needCount > 0 {
		fmt.Printf("\n%d language(s) can be installed. Run: aide grammar install --all\n", needCount)
	}

	return nil
}

func grammarScanJSON(statuses []grammar.LanguageStatus) error {
	fmt.Print("[")
	for i, s := range statuses {
		if i > 0 {
			fmt.Print(",")
		}
		fmt.Printf(`{"name":%q,"files":%d,"status":%q,"can_install":%t}`,
			s.Name, s.Files, s.Status, s.CanInstall)
	}
	fmt.Println("]")
	return nil
}
