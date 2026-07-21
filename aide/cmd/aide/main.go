// Package main provides the CLI for aide.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jmylchreest/aide/aide/internal/version"
	"github.com/jmylchreest/aide/aide/pkg/anchor"
	"github.com/jmylchreest/aide/aide/pkg/config"
)

const (
	defaultDBName = ".aide/memory/memory.db"
	legacyDBName  = ".aide/memory/store.db"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	// --project-root <path> may appear before or after the subcommand; strip
	// it from the full arg list and stash the value in AIDE_PROJECT_ROOT so
	// downstream subcommands and findProjectRoot pick it up uniformly. The
	// flag name is deliberately distinct from `--project` (which several
	// subcommands use as a project *name* / memory-scope tag).
	rawArgs := os.Args[1:]
	if override, rest, ok := extractProjectRootFlag(rawArgs); ok {
		_ = os.Setenv("AIDE_PROJECT_ROOT", override)
		rawArgs = rest
	}
	storeSelector, rest, hasStoreFlag := extractStoreFlag(rawArgs)
	if hasStoreFlag {
		rawArgs = rest
	}
	if len(rawArgs) == 0 {
		printUsage()
		os.Exit(1)
	}
	cmd := rawArgs[0]
	args := rawArgs[1:]

	// Fast path: commands that don't need a database.
	switch cmd {
	case "help", "-h", "--help":
		printUsage()
		return
	case "version", "-v", "--version":
		cmdVersion(args)
		return
	case "upgrade":
		if err := cmdUpgrade(args); err != nil {
			fatal("%v", err)
		}
		return
	case "dashboard":
		if err := cmdDashboard(args); err != nil {
			fatal("%v", err)
		}
		return
	case "anchor":
		// Read-only resolution probe: must run before any .aide/ creation
		// below and must not require a marker to exist.
		if err := cmdAnchor(args); err != nil {
			fatal("%v", err)
		}
		return
	}

	// Determine database path from project root (walks up to .aide or .git).
	// --store re-targets the whole invocation onto another member of the
	// anchor chain (decision store-routing) — resolved here, at the single
	// dbPath seam, so every subcommand routes uniformly.
	projectRoot, hasMarker := findProjectRoot()
	if hasStoreFlag {
		target, err := resolveStoreTarget(resolveAnchor(""), storeSelector)
		if err != nil {
			fatal("%v", err)
		}
		projectRoot, hasMarker = target, true
	}
	if _, err := config.Load(projectRoot); err != nil {
		fmt.Fprintf(os.Stderr, "aide: warning: config load failed: %v\n", err)
	}
	forceInit := config.Get().ForceInit

	if !hasMarker && !forceInit {
		// No .aide/ or .git/ marker found and AIDE_FORCE_INIT is not set.
		// Skip creating .aide/ directories to avoid polluting arbitrary dirs.
		fmt.Fprintf(os.Stderr, "aide: no .aide/ or .git/ directory found (set AIDE_FORCE_INIT=1 to override)\n")
		os.Exit(1)
	}

	// Use legacy store.db if it exists, otherwise use the new memory.db name.
	dbPath := computeDBPath(projectRoot)

	// Ensure memory directory exists.
	memoryDir := filepath.Dir(dbPath)
	if err := os.MkdirAll(memoryDir, 0o700); err != nil {
		fatal("failed to create memory directory: %v", err)
	}

	// Ensure .aide/bin/aide symlink points to this binary (best-effort).
	ensureBinSymlink(projectRoot)

	if err := runCommand(cmd, dbPath, args); err != nil {
		fatal("%v", err)
	}
}

func runCommand(cmd, dbPath string, args []string) error {
	switch cmd {
	case "memory":
		return cmdMemoryDispatcher(dbPath, args)
	case "code":
		return cmdCodeDispatcher(dbPath, args)
	case "findings":
		return cmdFindingsDispatcher(dbPath, args)
	case "survey":
		return cmdSurveyDispatcher(dbPath, args)
	case "task":
		return cmdTask(dbPath, args)
	case "decision":
		return cmdDecision(dbPath, args)
	case "message":
		return cmdMessage(dbPath, args)
	case "session":
		return cmdSession(dbPath, args)
	case "state":
		return cmdState(dbPath, args)
	case "token":
		return cmdTokenDispatcher(dbPath, args)
	case "observe":
		return cmdObserveDispatcher(dbPath, args)
	case "reflect":
		return cmdReflect(dbPath, args)
	case "agent":
		return cmdAgent(dbPath, args)
	case "daemon":
		return cmdDaemon(dbPath, args)
	case "mcp":
		return cmdMCP(dbPath, args)
	case "blueprint":
		return cmdBlueprint(dbPath, args)
	case "share":
		return cmdShare(dbPath, args)
	case "sync":
		return cmdSync(dbPath, args)
	case "config":
		return cmdConfig(dbPath, args)
	case "maintenance":
		return cmdMaintenance(dbPath, args)
	case "status":
		return cmdStatus(dbPath, args)
	case "grammar":
		return cmdGrammarDispatcher(dbPath, args)
	default:
		return fmt.Errorf("unknown command: %s", cmd)
	}
}

func cmdVersion(args []string) {
	for _, arg := range args {
		if arg == "--json" {
			fmt.Println(version.JSON())
			return
		}
	}
	fmt.Println(version.String())
}

func printUsage() {
	fmt.Printf(`aide %s - AI Development Environment - Unified system for AI agent orchestration

Usage:
  aide [--project-root PATH] [--store parent|top|PATH] <command> [arguments]

Global flags:
  --project-root PATH  Run against an arbitrary project root (any store)
  --store SELECTOR     Run against another member of THIS project's anchor
                       chain: parent (nearest container), top (outermost
                       ancestor), or an explicit chain-member path. The
                       target must already have a .aide store.

Commands:
  anchor     Show resolved project root, provenance, and anchor chain (read-only; --json)
  session    Session lifecycle (init - single-call startup; end - teardown + metrics)
  memory     Manage memories (add, delete, search, select, list, export, clear)
  code       Index and search code symbols (index, search, symbols, clear)
  findings   Query and manage static analysis findings (search, list, stats, clear)
  survey     Query and manage codebase survey data (search, list, stats, clear)
  task       Manage swarm tasks (create, claim, complete, list)
  decision   Manage decisions (set, get, list, history) - append-only
  message    Inter-agent messaging (send, list, ack, clear, prune)
  state      Manage session/agent state (set, get, delete, list, clear)
  blueprint  Manage and import best-practice decision blueprints
  share      Export/import decisions & memories as git-friendly markdown
  sync       Fetch subscribed peer context (decisions only, read-only layer)
  config     Inspect and edit aide configuration (show, get, set, unset, path)
  maintenance Compact bolt stores to reclaim disk (compact)
  daemon     Start gRPC daemon (Unix socket for IPC)
  mcp        Start MCP server (for Claude Code plugin integration)
  grammar    Manage tree-sitter language grammars (list, install, remove, scan)
  status     Show aide internal status (watcher, stores, analysers)
  dashboard  Manage aide-web dashboard (run, download, upgrade)
  upgrade    Check for updates and upgrade to latest version
  version    Show version information

Environment:
  AIDE_FORCE_INIT=1       Force initialisation even without .git/ or .aide/
  AIDE_CASCADE_DISABLED=1 Disable ancestor cascade and peer subscription layer in session context
  AIDE_GRAMMAR_URL        URL template for grammar downloads (placeholders: {version}, {asset}, {name}, {os}, {arch})
  AIDE_GRAMMAR_AUTO_DOWNLOAD  Set to 0 or false to disable automatic grammar downloads
  AIDE_CODE_WATCH=1       Enable file watching for code index updates
  AIDE_CODE_WATCH_PATHS   Comma-separated paths to watch (default: cwd)
  AIDE_CODE_WATCH_DELAY   Debounce delay for watcher (default: 30s)
  AIDE_INDEX_NON_VCS=1    Allow watcher/indexing in non-VCS dirs (default: refuse)
  AIDE_INDEX_WORKERS=N    Parallel parser workers for code indexing (default: NumCPU, max 32)
  AIDE_CODE_STORE_DISABLE=1  Disable code store entirely
  AIDE_CODE_STORE_SYNC=1  Force synchronous code store init (default: lazy)
  AIDE_PPROF_ENABLE=1     Enable pprof profiling server
  AIDE_PPROF_ADDR         pprof server address (default: localhost:6060)

Examples:
  # Memories
  aide memory add --category=learning "Found auth middleware at src/auth.ts"
  aide memory search "auth"                           # Fuzzy/prefix/ngram search
  aide memory select "middleware"                     # Exact substring match
  aide memory delete 1234567890

  # Decisions (append-only, latest wins)
  aide decision set auth-strategy "JWT" --rationale="Stateless"
  aide decision set auth-strategy "Session" --rationale="Changed mind"
  aide decision get auth-strategy              # Returns "Session"
  aide decision history auth-strategy          # Shows both decisions

  # Messages (with TTL, auto-prune on list)
  aide message send "Task done" --from=worker-1 --to=coordinator
  aide message list --agent=coordinator
  aide message ack 1 --agent=coordinator
  aide message prune                           # Remove expired messages

  # Tasks
  aide task create "Implement user model"
  aide task claim task-abc123 --agent=executor-1

  # State
  aide state set mode autopilot                # Global state
  aide state set mode eco --agent=worker-1    # Per-agent state
  aide state clear --agent=worker-1           # Clear agent state
`, version.Short())
}

// extractProjectRootFlag pulls a top-level --project-root=<path> or
// --project-root <path> flag out of args, returning the value, the args
// without the flag, and a found-flag indicator. Subcommands handle their own
// arguments, so we only strip this one before dispatch to keep the override
// invisible to them.
//
// This is intentionally distinct from `--project=<name>`, which several
// subcommands (`session init`, `memory sessions`, …) accept as a project
// *name* used as a memory-scope tag. Treating those as filesystem paths would
// — and historically did — silently break memory injection.
func extractProjectRootFlag(args []string) (string, []string, bool) {
	for i, a := range args {
		if a == "--project-root" {
			if i+1 >= len(args) {
				return "", args, false
			}
			return args[i+1], append(append([]string{}, args[:i]...), args[i+2:]...), true
		}
		if strings.HasPrefix(a, "--project-root=") {
			return a[len("--project-root="):], append(append([]string{}, args[:i]...), args[i+1:]...), true
		}
	}
	return "", args, false
}

// findProjectRoot walks up directories looking for .aide or .git markers.
// For git worktrees, .git is a file pointing to the main repo; we follow it
// to find the actual repository root so all worktrees share the same store.
// Submodules also use a .git file but are distinct repositories: a checkout
// inside a submodule anchors the submodule's own .aide/ store, not the
// superproject's — where you start (cwd) decides which project you're in.
//
// Thin wrapper over resolveAnchor (cmd_anchor.go), which is the single
// resolution authority and carries the full semantics documentation. Do
// not add resolution logic here or anywhere else.
func findProjectRoot() (string, bool) {
	a := resolveAnchor("")
	return a.Root, a.HasMarker
}

// computeDBPath returns the store path under projectRoot, preferring the
// legacy store.db name when it already exists.
func computeDBPath(projectRoot string) string {
	dbPath := filepath.Join(projectRoot, defaultDBName)
	legacyPath := filepath.Join(projectRoot, legacyDBName)
	if _, err := os.Stat(legacyPath); err == nil {
		dbPath = legacyPath
	}
	return dbPath
}

var vcsMarkerNames = anchor.VCSMarkerNames

// vcsMarker reports whether dir contains a VCS marker, and for .git files
// (worktrees) returns the resolved main-repo root. Empty resolved string
// for normal .git directories means "use dir itself".
func vcsMarker(dir string) (vcsDir string, resolved string, ok bool) {
	gitPath := filepath.Join(dir, ".git")
	if info, err := os.Stat(gitPath); err == nil {
		if info.IsDir() {
			return dir, "", true
		}
		// Worktree: .git is a file containing "gitdir: <path>"
		if root := resolveWorktreeRoot(gitPath); root != "" {
			return dir, root, true
		}
		return dir, "", true
	}
	for _, marker := range vcsMarkerNames[1:] {
		if _, err := os.Stat(filepath.Join(dir, marker)); err == nil {
			return dir, "", true
		}
	}
	return "", "", false
}

// isVCSRoot reports whether dir is the root of a version-controlled
// repository. Used to gate proactive indexing/watching: an arbitrary
// directory backed only by .aide/ (e.g. $HOME) shouldn't have the file
// watcher walk and re-index everything beneath it.
func isVCSRoot(dir string) bool {
	for _, marker := range vcsMarkerNames {
		if _, err := os.Stat(filepath.Join(dir, marker)); err == nil {
			return true
		}
	}
	return false
}

func isSubmoduleGitdir(gitdir string) bool {
	return anchor.IsSubmoduleGitdir(gitdir)
}

func gitfileInfo(gitFilePath string) (shape string, ownerRoot string) {
	return anchor.GitfileInfo(gitFilePath)
}

// resolveWorktreeRoot reads a .git file (worktree marker) and resolves the
// main repository root. The file contains "gitdir: /path/to/repo/.git/worktrees/<name>".
//
// Submodules also use a .git file, with gitdir pointing into the
// superproject's modules tree (plain or worktree-hosted — see
// isSubmoduleGitdir). A submodule is a distinct repository, so it anchors
// its own .aide/ store rather than sharing the superproject's: returning
// "" makes vcsMarker fall back to the submodule directory itself. (A
// worktree OF a submodule also lands here and anchors at the worktree
// directory — known limitation.)
func resolveWorktreeRoot(gitFilePath string) string {
	shape, ownerRoot := gitfileInfo(gitFilePath)
	if shape != "worktree" {
		return ""
	}
	return ownerRoot
}

// ensureBinSymlink creates or updates .aide/bin/aide as a symlink to the
// currently running binary. This gives users a convenient, stable path to
// invoke aide from within the project. On platforms that don't support
// symlinks (or if anything fails), it silently does nothing.
func ensureBinSymlink(projectRoot string) {
	self, err := os.Executable()
	if err != nil {
		return
	}
	self, err = filepath.EvalSymlinks(self)
	if err != nil {
		return
	}

	binDir := filepath.Join(projectRoot, ".aide", "bin")
	link := filepath.Join(binDir, "aide")

	// Check if symlink already points to the right target.
	if _, err := os.Readlink(link); err == nil {
		resolved, err := filepath.EvalSymlinks(link)
		if err == nil && resolved == self {
			return // already correct
		}
		// Symlink exists but points elsewhere — remove it.
		os.Remove(link)
	}

	// Ensure .aide/bin/ directory exists.
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return
	}

	// Remove any non-symlink file that might be in the way.
	if info, err := os.Lstat(link); err == nil && info.Mode()&os.ModeSymlink == 0 {
		os.Remove(link)
	}

	// Create the symlink. Silently ignore errors (e.g. Windows without
	// developer mode, or read-only filesystems).
	_ = os.Symlink(self, link)
}
