// Package grpcapi provides the gRPC client for aide.
package grpcapi

import (
	"context"
	"fmt"
	"net"
	"os"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Client wraps gRPC service clients for aide.
type Client struct {
	conn     *grpc.ClientConn
	Memory   MemoryServiceClient
	State    StateServiceClient
	Decision DecisionServiceClient
	Message  MessageServiceClient
	Task     TaskServiceClient
	Code     CodeServiceClient
	Findings FindingsServiceClient
	Health   HealthServiceClient
}

// SocketExistsForDB checks if the gRPC socket is available for the given database path.
func SocketExistsForDB(dbPath string) bool {
	socketPath := SocketPathFromDB(dbPath)
	_, err := os.Stat(socketPath)
	return err == nil
}

// NewClientForDB creates a new gRPC client connected to the Unix socket derived from the database path.
func NewClientForDB(dbPath string) (*Client, error) {
	return NewClientWithSocket(SocketPathFromDB(dbPath))
}

// NewClientWithSocket creates a new gRPC client connected to a specific socket.
func NewClientWithSocket(socketPath string) (*Client, error) {
	// Check if socket exists
	if _, err := os.Stat(socketPath); err != nil {
		return nil, fmt.Errorf("socket not found: %s", socketPath)
	}

	// Connect to Unix socket
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, "unix://"+socketPath, //nolint:staticcheck // TODO: migrate to grpc.NewClient
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(), //nolint:staticcheck // required for connection-on-dial semantics
		grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
			return net.Dial("unix", socketPath)
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to socket: %w", err)
	}

	return &Client{
		conn:     conn,
		Memory:   NewMemoryServiceClient(conn),
		State:    NewStateServiceClient(conn),
		Decision: NewDecisionServiceClient(conn),
		Message:  NewMessageServiceClient(conn),
		Task:     NewTaskServiceClient(conn),
		Code:     NewCodeServiceClient(conn),
		Findings: NewFindingsServiceClient(conn),
		Health:   NewHealthServiceClient(conn),
	}, nil
}

// Close closes the gRPC connection.
func (c *Client) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// Ping checks if the server is healthy.
func (c *Client) Ping(ctx context.Context) error {
	resp, err := c.Health.Check(ctx, &HealthCheckRequest{})
	if err != nil {
		return err
	}
	if !resp.Healthy {
		return fmt.Errorf("server unhealthy")
	}
	return nil
}
