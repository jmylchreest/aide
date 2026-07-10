package main

import (
	"context"
	"encoding/json"
	"os"
	"time"

	"github.com/jmylchreest/aide/aide/internal/version"
	"github.com/jmylchreest/aide/aide/pkg/config"
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
	// Authority: "daemon" = this process owns the stores and serves the
	// socket; "client" = attached to another process's daemon over gRPC.
	// Normally one session means one daemon-mode process — they diverge
	// when several sessions share a project, or a stale daemon survives an
	// upgrade (the Daemon block makes version skew visible).
	Authority string      `json:"authority"`
	PID       int         `json:"pid"`
	PPID      int         `json:"ppid"`
	StartedAt time.Time   `json:"started_at"`
	Daemon    *DaemonInfo `json:"daemon,omitempty"` // set in client mode
	PprofURL  string      `json:"pprof_url,omitempty"`
}

// DaemonInfo identifies the daemon a client-mode instance is attached to.
type DaemonInfo struct {
	Reachable   bool      `json:"reachable"`
	Version     string    `json:"version,omitempty"`
	Commit      string    `json:"commit,omitempty"`
	BuildDate   string    `json:"build_date,omitempty"`
	PID         int64     `json:"pid,omitempty"`
	StartedAt   time.Time `json:"started_at,omitempty"`
	VersionSkew bool      `json:"version_skew,omitempty"` // daemon version differs from this process
	Error       string    `json:"error,omitempty"`
}

// ============================================================================
// Instance Info Tool
// ============================================================================

func (s *MCPServer) registerInstanceInfoTools() {
	mcp.AddTool(s.server, &mcp.Tool{
		Name: "instance_info",
		Description: `Get identity and configuration of this aide instance.

Returns the resolved project root, working directory, version info,
database path, gRPC socket path, operating mode, process IDs, and
authority: "daemon" means this process owns the stores; "client" means it
is attached to another process's daemon over gRPC (common when several
sessions share a project). In client mode a daemon block reports the
daemon's version and identity — version_skew=true flags a stale daemon
left over from before an upgrade.

**Use cases:**
- Confirm which project root this instance resolved to
- Verify version and build info, and detect daemon version skew
- Debug multi-instance / worktree issues
- Check if the instance is running in a specific mode`,
	}, s.handleInstanceInfo)
}

func (s *MCPServer) handleInstanceInfo(ctx context.Context, _ *mcp.CallToolRequest, _ emptyInput) (*mcp.CallToolResult, any, error) {
	mcpLog.Printf("tool: instance_info")

	cwd, _ := os.Getwd()
	root := projectRoot(s.dbPath)

	authority := "daemon"
	if s.grpcClient != nil {
		authority = "client"
	}

	info := InstanceInfo{
		ProjectRoot: root,
		Cwd:         cwd,
		Version:     version.GetInfo(),
		DBPath:      s.dbPath,
		SocketPath:  grpcapi.SocketPathFromDB(s.dbPath),
		Mode:        config.Get().Mode,
		Authority:   authority,
		PID:         os.Getpid(),
		PPID:        os.Getppid(),
		StartedAt:   version.StartTime,
		PprofURL:    pprofURL(),
	}

	if s.grpcClient != nil {
		info.Daemon = daemonInfo(ctx, s.grpcClient)
	}

	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return errorResult("failed to marshal instance info"), nil, nil
	}

	mcpLog.Printf("  project_root=%s authority=%s", root, authority)
	return textResult(string(data)), nil, nil
}

// daemonInfo queries the attached daemon's identity over gRPC. Failure is
// reported, not swallowed — an unreachable daemon is exactly what this
// block exists to surface.
func daemonInfo(ctx context.Context, client *grpcapi.Client) *DaemonInfo {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	resp, err := client.Health.Check(ctx, &grpcapi.HealthCheckRequest{})
	if err != nil {
		return &DaemonInfo{Reachable: false, Error: err.Error()}
	}
	d := &DaemonInfo{
		Reachable:   true,
		Version:     resp.Version,
		Commit:      resp.Commit,
		BuildDate:   resp.BuildDate,
		PID:         resp.Pid,
		VersionSkew: resp.Version != version.GetInfo().Version,
	}
	if resp.StartedAtUnix > 0 {
		d.StartedAt = time.Unix(resp.StartedAtUnix, 0)
	}
	return d
}
