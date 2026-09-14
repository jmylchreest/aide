// Package grpcapi provides the gRPC client for aide.
package grpcapi

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/store"
	"google.golang.org/grpc/metadata"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Client wraps gRPC service clients for aide.
type Client struct {
	conn      *grpc.ClientConn
	Memory    MemoryServiceClient
	State     StateServiceClient
	Decision  DecisionServiceClient
	Message   MessageServiceClient
	Task      TaskServiceClient
	Code      CodeServiceClient
	Findings  FindingsServiceClient
	Survey    SurveyServiceClient
	Tombstone TombstoneServiceClient
	Token     TokenServiceClient
	Observe   ObserveServiceClient
	Instinct  InstinctServiceClient
	Swarm     SwarmServiceClient
	Health    HealthServiceClient
	Status    StatusServiceClient
}

// SocketExistsForDB checks if the gRPC socket is available for the given database path.
func SocketExistsForDB(dbPath string) bool {
	socketPath := SocketPathFromDB(dbPath)
	_, err := os.Stat(socketPath)
	return err == nil
}

// NewClientForDB creates a new gRPC client connected to the Unix socket derived from the database path.
func NewClientForDB(dbPath string) (*Client, error) {
	return NewClientForCheckout(dbPath, store.CheckoutRoot(dbPath))
}

var ErrCheckoutRoutingUnavailable = errors.New("daemon does not support checkout routing; restart it with the current aide binary")

// NewClientForCheckout selects analysis stores without changing shared memory routing.
func NewClientForCheckout(dbPath, root string) (*Client, error) {
	return newClientWithSocket(SocketPathFromDB(dbPath), root)
}

// NewClientWithSocket creates a new gRPC client connected to a specific socket.
func NewClientWithSocket(socketPath string) (*Client, error) {
	return newClientWithSocket(socketPath, "")
}

func newClientWithSocket(socketPath, root string) (*Client, error) {
	// Check if socket exists
	if _, err := os.Stat(socketPath); err != nil {
		return nil, fmt.Errorf("socket not found: %s", socketPath)
	}

	// Connect to Unix socket using grpc.NewClient (lazy connection).
	conn, err := grpc.NewClient(
		"unix://"+socketPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoke grpc.UnaryInvoker, opts ...grpc.CallOption) error {
			if root != "" {
				ctx = metadata.AppendToOutgoingContext(ctx, checkoutRootKey, root)
			}
			return invoke(ctx, method, req, reply, cc, opts...)
		}),
		grpc.WithStreamInterceptor(func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
			if root != "" {
				ctx = metadata.AppendToOutgoingContext(ctx, checkoutRootKey, root)
			}
			return streamer(ctx, desc, cc, method, opts...)
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}

	c := &Client{
		conn:      conn,
		Memory:    NewMemoryServiceClient(conn),
		State:     NewStateServiceClient(conn),
		Decision:  NewDecisionServiceClient(conn),
		Message:   NewMessageServiceClient(conn),
		Task:      NewTaskServiceClient(conn),
		Code:      NewCodeServiceClient(conn),
		Findings:  NewFindingsServiceClient(conn),
		Survey:    NewSurveyServiceClient(conn),
		Tombstone: NewTombstoneServiceClient(conn),
		Token:     NewTokenServiceClient(conn),
		Observe:   NewObserveServiceClient(conn),
		Instinct:  NewInstinctServiceClient(conn),
		Swarm:     NewSwarmServiceClient(conn),
		Health:    NewHealthServiceClient(conn),
		Status:    NewStatusServiceClient(conn),
	}

	// Verify connectivity with a health-check RPC.
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	if err := c.Ping(ctx); err != nil {
		_ = conn.Close()
		if isConnectDenied(err) {
			return nil, fmt.Errorf("failed to connect to %s: %w", socketPath, ErrSandboxDenied)
		}
		return nil, fmt.Errorf("failed to connect to socket: %w", err)
	}

	if root != "" {
		var headers metadata.MD
		if _, err := c.Health.Check(ctx, &HealthCheckRequest{}, grpc.Header(&headers)); err != nil {
			conn.Close()
			return nil, err
		}
		if len(headers.Get("aide-checkout-routing")) == 0 {
			conn.Close()
			return nil, ErrCheckoutRoutingUnavailable
		}
	}
	return c, nil
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
