// Package main provides the CLI for aide.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jmylchreest/aide/aide/internal/version"
)

const defaultDBName = ".aide/memory/store.db"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	// Fast path: commands that don't need a database.
	switch cmd {
	case "help", "-h", "--help":
		printUsage()
		return
	case "version", "-v", "--version":
		if err := cmdVersion(args); err != nil {
			fatal("%v", err)
		}
		return
	case "upgrade":
		if err := cmdUpgrade(args); err != nil {
			fatal("%v", err)
		}
		return
	}

	// Determine database path.
	dbPath := os.Getenv("AIDE_MEMORY_DB")
	if dbPath == "" {
		projectRoot := findProjectRoot()
		dbPath = filepath.Join(projectRoot, defaultDBName)
	}

	// Ensure memory directory exists.
	memoryDir := filepath.Dir(dbPath)
	if err := os.MkdirAll(memoryDir, 0o755); err != nil {
		fatal("failed to create memory directory: %v", err)
	}

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
	case "daemon":
		return cmdDaemon(dbPath, args)
	case "mcp":
		return cmdMCP(dbPath, args)
	default:
		return fmt.Errorf("unknown command: %s", cmd)
	}
}

func cmdVersion(args []string) error {
	for _, arg := range args {
		if arg == "--json" {
			fmt.Println(version.JSON())
			return nil
		}
	}
	fmt.Println(version.String())
	return nil
}

func printUsage() {
	fmt.Printf(`aide %s - AI Development Environment - Unified system for AI agent orchestration

Usage:
  aide <command> [arguments]

Commands:
  session    Session lifecycle (init - single-call startup)
  memory     Manage memories (add, delete, search, select, list, export, clear)
  code       Index and search code symbols (index, search, symbols, clear)
  task       Manage swarm tasks (create, claim, complete, list)
  decision   Manage decisions (set, get, list, history) - append-only
  message    Inter-agent messaging (send, list, ack, clear, prune)
  state      Manage session/agent state (set, get, delete, list, clear)
  daemon     Start gRPC daemon (Unix socket for IPC)
  mcp        Start MCP server (for Claude Code plugin integration)
  upgrade    Check for updates and upgrade to latest version
  version    Show version information

Environment:
  AIDE_MEMORY_DB          Database path (default: .aide/memory/store.db)
  AIDE_CODE_WATCH=1       Enable file watching for code index updates
  AIDE_CODE_WATCH_PATHS   Comma-separated paths to watch (default: cwd)
  AIDE_CODE_WATCH_DELAY   Debounce delay for watcher (default: 30s)
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
  aide state set mode ralph                    # Global state
  aide state set mode eco --agent=worker-1    # Per-agent state
  aide state clear --agent=worker-1           # Clear agent state
`, version.Short())
}

// findProjectRoot walks up directories looking for .aide or .git markers.
// This avoids spawning a git subprocess on every invocation.
// For git worktrees, .git is a file pointing to the main repo; we follow it
// to find the actual repository root so all worktrees share the same store.
func findProjectRoot() string {
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}

	dir := cwd
	for {
		if _, err := os.Stat(filepath.Join(dir, ".aide")); err == nil {
			return dir
		}

		gitPath := filepath.Join(dir, ".git")
		info, err := os.Stat(gitPath)
		if err == nil {
			if info.IsDir() {
				// Normal git repo
				return dir
			}
			// Worktree: .git is a file containing "gitdir: <path>"
			// Follow it to the main repo root.
			if root := resolveWorktreeRoot(gitPath); root != "" {
				return root
			}
			return dir
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return cwd
		}
		dir = parent
	}
}

// resolveWorktreeRoot reads a .git file (worktree marker) and resolves the
// main repository root. The file contains "gitdir: /path/to/repo/.git/worktrees/<name>".
func resolveWorktreeRoot(gitFilePath string) string {
	data, err := os.ReadFile(gitFilePath)
	if err != nil {
		return ""
	}
	line := strings.TrimSpace(string(data))
	if !strings.HasPrefix(line, "gitdir:") {
		return ""
	}
	gitdir := strings.TrimSpace(strings.TrimPrefix(line, "gitdir:"))

	// Make absolute if relative
	if !filepath.IsAbs(gitdir) {
		gitdir = filepath.Join(filepath.Dir(gitFilePath), gitdir)
	}

	// Walk up from .git/worktrees/<name> to find the .git directory
	// then return its parent as the repo root
	candidate := gitdir
	for {
		parent := filepath.Dir(candidate)
		if parent == candidate {
			break
		}
		if filepath.Base(candidate) == ".git" {
			return parent
		}
		candidate = parent
	}
	return ""
}
