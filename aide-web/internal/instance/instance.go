package instance

import (
	"sync"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/grpcapi"
	"github.com/jmylchreest/aide/aide/pkg/grpcapi/adapter"
	"github.com/jmylchreest/aide/aide/pkg/store"
)

// Status represents the connection state of an instance.
type Status string

const (
	StatusConnected    Status = "connected"
	StatusDisconnected Status = "disconnected"
	StatusConnecting   Status = "connecting"
)

// Instance represents a single aide daemon connection.
type Instance struct {
	mu          sync.RWMutex
	projectRoot string
	projectName string
	socketPath  string
	dbPath      string
	version     string
	status      Status
	client      *grpcapi.Client
	store       store.Store
	findings    store.FindingsStore
	survey      store.SurveyStore
	lastSeen    time.Time
}

// NewInstance creates a new disconnected Instance.
func NewInstance(projectRoot, projectName, socketPath, dbPath, version string) *Instance {
	return &Instance{
		projectRoot: projectRoot,
		projectName: projectName,
		socketPath:  socketPath,
		dbPath:      dbPath,
		version:     version,
		status:      StatusDisconnected,
	}
}

func (i *Instance) ProjectRoot() string {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.projectRoot
}

func (i *Instance) ProjectName() string {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.projectName
}

func (i *Instance) SocketPath() string {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.socketPath
}

func (i *Instance) DBPath() string {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.dbPath
}

func (i *Instance) Version() string {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.version
}

// Status returns the current connection status.
func (i *Instance) Status() Status {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.status
}

// LastSeen returns when the instance was last known alive.
func (i *Instance) LastSeen() time.Time {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.lastSeen
}

// Client returns the gRPC client, or nil if disconnected.
func (i *Instance) Client() *grpcapi.Client {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.client
}

// Store returns the store adapter, or nil if disconnected.
func (i *Instance) Store() store.Store {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.store
}

// FindingsStore returns the findings adapter, or nil if disconnected.
func (i *Instance) FindingsStore() store.FindingsStore {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.findings
}

// SurveyStore returns the survey adapter, or nil if disconnected.
func (i *Instance) SurveyStore() store.SurveyStore {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.survey
}

// UpdateMeta updates the mutable metadata fields under the lock.
func (i *Instance) UpdateMeta(socketPath, dbPath, version string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.socketPath = socketPath
	i.dbPath = dbPath
	i.version = version
}

// Connect attempts to establish a gRPC connection to the daemon.
// Returns false if already connected or connecting (caller should not retry).
func (i *Instance) Connect() error {
	i.mu.Lock()
	if i.status == StatusConnected || i.status == StatusConnecting {
		i.mu.Unlock()
		return nil
	}
	i.status = StatusConnecting
	socketPath := i.socketPath
	i.mu.Unlock()

	// Dial outside the lock to avoid holding it during network I/O
	client, err := grpcapi.NewClientWithSocket(socketPath)
	if err != nil {
		i.mu.Lock()
		i.status = StatusDisconnected
		i.mu.Unlock()
		return err
	}

	i.mu.Lock()
	defer i.mu.Unlock()
	i.client = client
	i.store = adapter.NewStoreAdapter(client)
	i.findings = adapter.NewFindingsAdapter(client)
	i.survey = adapter.NewSurveyAdapter(client)
	i.status = StatusConnected
	i.lastSeen = time.Now()
	return nil
}

// Disconnect closes the gRPC connection.
func (i *Instance) Disconnect() {
	i.mu.Lock()
	defer i.mu.Unlock()

	if i.client != nil {
		i.client.Close()
		i.client = nil
	}
	i.store = nil
	i.findings = nil
	i.survey = nil
	i.status = StatusDisconnected
}

// HealthCheck pings the daemon and updates status.
func (i *Instance) HealthCheck() bool {
	i.mu.Lock()
	if i.client == nil {
		i.mu.Unlock()
		return false
	}
	client := i.client
	i.mu.Unlock()

	ctx, cancel := contextWithTimeout()
	defer cancel()

	if err := client.Ping(ctx); err != nil {
		i.mu.Lock()
		i.status = StatusDisconnected
		if i.client != nil {
			i.client.Close()
		}
		i.client = nil
		i.store = nil
		i.findings = nil
		i.survey = nil
		i.mu.Unlock()
		return false
	}

	i.mu.Lock()
	i.lastSeen = time.Now()
	i.mu.Unlock()
	return true
}
