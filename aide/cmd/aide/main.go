// Package main provides the CLI for aide.
package main

import (
	"fmt"
	"os"
	"os/exec"
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

	// Find the project root (git root or cwd).
	projectRoot := findProjectRoot()

	// Determine database path.
	dbPath := getEnvOrDefault("AIDE_MEMORY_DB", "")
	if dbPath == "" {
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
	case "state":
		return cmdState(dbPath, args)
	case "daemon":
		return cmdDaemon(dbPath, args)
	case "mcp":
		return cmdMCP(dbPath, args)
	case "upgrade":
		return cmdUpgrade(args)
	case "usage":
		return cmdUsage(args)
	case "help", "-h", "--help":
		printUsage()
		return nil
	case "version", "-v", "--version":
		return cmdVersion(args)
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
  memory     Manage memories (add, delete, search, select, list, export, clear)
  code       Index and search code symbols (index, search, symbols, clear)
  task       Manage swarm tasks (create, claim, complete, list)
  decision   Manage decisions (set, get, list, history) - append-only
  message    Inter-agent messaging (send, list, ack, clear, prune)
  state      Manage session/agent state (set, get, delete, list, clear)
  usage      Show Claude Code usage statistics (tokens, messages, sessions)
  daemon     Start gRPC daemon (Unix socket for IPC)
  mcp        Start MCP server (for Claude Code plugin integration)
  upgrade    Check for updates and upgrade to latest version
  version    Show version information

Environment:
  AIDE_MEMORY_DB          Database path (default: .aide/memory/store.db)
  AIDE_CODE_WATCH=1       Enable file watching for code index updates
  AIDE_CODE_WATCH_PATHS   Comma-separated paths to watch (default: cwd)
  AIDE_CODE_WATCH_DELAY   Debounce delay for watcher (default: 30s)
  AIDE_CODE_STORE_DISABLE=1  Disable code store (faster startup)
  AIDE_CODE_STORE_LAZY=1  Lazy-load code store after MCP ready (faster startup)
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

// findProjectRoot finds the git root directory, or falls back to cwd.
func findProjectRoot() string {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err == nil {
		return strings.TrimSpace(string(output))
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}

	dir := cwd
	for {
		if _, err := os.Stat(filepath.Join(dir, ".aide")); err == nil {
			return dir
		}
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return cwd
		}
		dir = parent
	}
}
