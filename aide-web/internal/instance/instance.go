package instance

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/grpcapi"
	"github.com/jmylchreest/aide/aide/pkg/grpcapi/adapter"
	"github.com/jmylchreest/aide/aide/pkg/store"
)

// slugHashLen is how many hex chars of the project-root hash to append to a
// slug. 7 (git-short-SHA length) gives 28 bits — ample to disambiguate the
// handful of instances a machine ever runs while keeping the URL short.
const slugHashLen = 7

// Slug builds a stable, URL-safe identifier that stays unique even when two
// projects share a base name (e.g. .../jmylchreest and .../jmylchreest/jmylchreest
// both have ProjectName "jmylchreest"). It is the readable name plus a short
// hash of the project root — the root is the instance's true identity (the
// manager keys its map by it), so the hash guarantees uniqueness and stays
// stable across restarts. ProjectName remains the display label.
func Slug(projectName, projectRoot string) string {
	sum := sha256.Sum256([]byte(projectRoot))
	return projectName + "-" + hex.EncodeToString(sum[:])[:slugHashLen]
}

// Status represents the connection state of an instance.
type Status string

const (
	StatusConnected    Status = "connected"
	StatusDisconnected Status = "disconnected"
	StatusConnecting   Status = "connecting"
	// StatusIdle marks a known project whose daemon is not running. Idle
	// instances stay listed (the instances page is a directory of known
	// projects) but are not dialed until their registry entry is rewritten
	// by a daemon starting up.
	StatusIdle Status = "idle"
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
	instinct    store.InstinctProposalStore
	code        store.CodeIndexStore
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

// Slug returns the disambiguated routing identifier for this instance
// (ProjectName + short project-root hash). See the package-level Slug.
func (i *Instance) Slug() string {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return Slug(i.projectName, i.projectRoot)
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

// SetIdle parks a disconnected instance: still listed, no longer dialed.
func (i *Instance) SetIdle() {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.status == StatusDisconnected {
		i.status = StatusIdle
	}
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

// CodeStore returns the code index adapter, or nil if disconnected.
func (i *Instance) CodeStore() store.CodeIndexStore {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.code
}

// InstinctStore returns the instinct proposal store adapter.
func (i *Instance) InstinctStore() store.InstinctProposalStore {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.instinct
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
	i.instinct = adapter.NewInstinctAdapter(client)
	i.code = adapter.NewCodeAdapter(client)
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
	i.code = nil
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
