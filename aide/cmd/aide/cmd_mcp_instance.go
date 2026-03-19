package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/jmylchreest/aide/aide/internal/version"
	"github.com/jmylchreest/aide/aide/pkg/grpcapi"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// InstanceInfo is the response payload for the instance_info MCP tool.
type InstanceInfo struct {
	ProjectRoot string       `json:"project_root"`
	Cwd         string       `json:"cwd"`
	Version     version.Info `json:"version"`
	DBPath      string       `json:"db_path"`
	SocketPath  string       `json:"socket_path"`
	Mode        string       `json:"mode,omitempty"`
	PID         int          `json:"pid"`
	StartedAt   time.Time    `json:"started_at,omitempty"`
	PprofURL    string       `json:"pprof_url,omitempty"`
}

// ============================================================================
// Instance Info Tool
// ============================================================================

func (s *MCPServer) registerInstanceInfoTools() {
	mcp.AddTool(s.server, &mcp.Tool{
		Name: "instance_info",
		Description: `Get identity and configuration of this aide instance.

Returns the resolved project root, working directory, version info,
database path, gRPC socket path, operating mode, and process ID.

**Use cases:**
- Confirm which project root this instance resolved to
- Verify version and build info
- Debug multi-instance / worktree issues
- Check if the instance is running in a specific mode`,
	}, s.handleInstanceInfo)
}

func (s *MCPServer) handleInstanceInfo(_ context.Context, _ *mcp.CallToolRequest, _ emptyInput) (*mcp.CallToolResult, any, error) {
	mcpLog.Printf("tool: instance_info")

	cwd, _ := os.Getwd()
	root := projectRoot(s.dbPath)

	info := InstanceInfo{
		ProjectRoot: root,
		Cwd:         cwd,
		Version:     version.GetInfo(),
		DBPath:      s.dbPath,
		SocketPath:  grpcapi.SocketPathFromDB(s.dbPath),
		Mode:        os.Getenv("AIDE_MODE"),
		PID:         os.Getpid(),
		PprofURL:    pprofURL(),
	}

	// Derive .aide directory and check for a start-time marker if available.
	aideDir := filepath.Join(root, ".aide")
	if fi, err := os.Stat(aideDir); err == nil {
		info.StartedAt = fi.ModTime()
	}

	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return errorResult("failed to marshal instance info"), nil, nil
	}

	mcpLog.Printf("  project_root=%s", root)
	return textResult(string(data)), nil, nil
}
