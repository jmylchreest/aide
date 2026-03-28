package instance

import (
	"context"
	"log"
	"os"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/jmylchreest/aide/aide/pkg/grpcapi/registry"
)

const healthCheckInterval = 5 * time.Second

func contextWithTimeout() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 2*time.Second)
}

// Manager manages connections to multiple aide daemon instances.
type Manager struct {
	mu        sync.RWMutex
	instances map[string]*Instance // keyed by project root
	watcher   *fsnotify.Watcher
	done      chan struct{}
}

// NewManager creates a new instance manager that watches the registry directory.
func NewManager() (*Manager, error) {
	m := &Manager{
		instances: make(map[string]*Instance),
		done:      make(chan struct{}),
	}

	if err := m.loadFromRegistry(); err != nil {
		log.Printf("warning: failed to load registry: %v", err)
	}

	dir, err := registry.InstancesDir()
	if err == nil {
		if err := os.MkdirAll(dir, 0o755); err == nil {
			watcher, err := fsnotify.NewWatcher()
			if err == nil {
				m.watcher = watcher
				if err := watcher.Add(dir); err != nil {
					log.Printf("warning: cannot watch instances directory: %v", err)
				}
			}
		}
	}

	go m.run()
	return m, nil
}

// Close stops the manager and disconnects all instances.
func (m *Manager) Close() {
	close(m.done)
	if m.watcher != nil {
		m.watcher.Close()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, inst := range m.instances {
		inst.Disconnect()
	}
}

// Instances returns a snapshot of all known instances.
func (m *Manager) Instances() []*Instance {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]*Instance, 0, len(m.instances))
	for _, inst := range m.instances {
		result = append(result, inst)
	}
	return result
}

// Get returns an instance by project root, or nil.
func (m *Manager) Get(projectRoot string) *Instance {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.instances[projectRoot]
}

// ConnectedInstances returns only instances with active connections.
func (m *Manager) ConnectedInstances() []*Instance {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []*Instance
	for _, inst := range m.instances {
		if inst.Status() == StatusConnected {
			result = append(result, inst)
		}
	}
	return result
}

func (m *Manager) loadFromRegistry() error {
	entries, err := registry.List()
	if err != nil {
		return err
	}
	for _, entry := range entries {
		m.addOrUpdate(entry)
	}
	return nil
}

func (m *Manager) addOrUpdate(reg registry.Instance) {
	m.mu.Lock()
	inst, exists := m.instances[reg.ProjectRoot]
	if !exists {
		inst = NewInstance(reg.ProjectRoot, reg.ProjectName, reg.SocketPath, reg.DBPath, reg.Version)
		m.instances[reg.ProjectRoot] = inst
	} else {
		inst.UpdateMeta(reg.SocketPath, reg.DBPath, reg.Version)
	}
	m.mu.Unlock()

	// Connect() is idempotent — returns nil if already connected/connecting
	go func() {
		if err := inst.Connect(); err != nil {
			log.Printf("instance %s: connection failed: %v", inst.ProjectName(), err)
		}
	}()
}

func (m *Manager) run() {
	ticker := time.NewTicker(healthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.done:
			return

		case event, ok := <-m.watcherEvents():
			if !ok {
				// Watcher closed; stop selecting on it
				m.watcher = nil
				continue
			}
			if event.Has(fsnotify.Create) || event.Has(fsnotify.Write) || event.Has(fsnotify.Remove) {
				m.loadFromRegistry()
			}

		case err, ok := <-m.watcherErrors():
			if ok {
				log.Printf("watcher error: %v", err)
			}

		case <-ticker.C:
			m.healthCheckAll()
		}
	}
}

func (m *Manager) watcherEvents() <-chan fsnotify.Event {
	if m.watcher != nil {
		return m.watcher.Events
	}
	return nil
}

func (m *Manager) watcherErrors() <-chan error {
	if m.watcher != nil {
		return m.watcher.Errors
	}
	return nil
}

func (m *Manager) healthCheckAll() {
	m.mu.RLock()
	instances := make([]*Instance, 0, len(m.instances))
	for _, inst := range m.instances {
		instances = append(instances, inst)
	}
	m.mu.RUnlock()

	for _, inst := range instances {
		switch inst.Status() {
		case StatusConnected:
			if !inst.HealthCheck() {
				log.Printf("instance %s: health check failed", inst.ProjectName())
				// Connect() is idempotent — won't double-connect
				go func(i *Instance) {
					if err := i.Connect(); err == nil {
						log.Printf("instance %s: reconnected", i.ProjectName())
					}
				}(inst)
			}
		case StatusDisconnected:
			// Connect() checks status and returns immediately if already connecting
			go func(i *Instance) {
				if err := i.Connect(); err == nil {
					log.Printf("instance %s: reconnected", i.ProjectName())
				}
			}(inst)
		}
	}
}
