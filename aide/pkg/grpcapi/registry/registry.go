// Package registry manages instance registration for aide daemons.
// When a daemon starts, it writes a JSON file to ~/.aide/instances/
// so that aide-web and other tools can discover running instances.
package registry

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jmylchreest/aide/aide/internal/version"
)

// Instance describes a running aide daemon for discovery.
// Parents carries the anchor chain's ancestor roots (nearest first) so
// consumers like aide-web can assemble the estate graph — which instances
// contain which — without re-resolving anything. Identity and topology
// only, never liveness: whether a parent has its own live daemon is
// answered by the parent's own registry entry.
type Instance struct {
	ProjectRoot  string    `json:"project_root"`
	ProjectName  string    `json:"project_name"`
	SocketPath   string    `json:"socket_path"`
	DBPath       string    `json:"db_path"`
	PID          int       `json:"pid"`
	Version      string    `json:"version"`
	RegisteredAt time.Time `json:"registered_at"`
	Parents      []string  `json:"parents,omitempty"`
}

// InstancesDir returns the directory where instance registry files are stored.
func InstancesDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".aide", "instances"), nil
}

// hashProjectRoot returns a hex-encoded SHA-256 hash of the project root path.
func hashProjectRoot(projectRoot string) string {
	h := sha256.Sum256([]byte(projectRoot))
	return hex.EncodeToString(h[:])
}

// NormalizeRoot resolves symlinks so aliased spellings of the same project
// directory (a ~/src tree pointing at a data volume) share one registry
// identity. Two daemons registering "the same" project under different
// spellings otherwise produce duplicate instances that both connect.
func NormalizeRoot(projectRoot string) string {
	if resolved, err := filepath.EvalSymlinks(projectRoot); err == nil {
		return resolved
	}
	return filepath.Clean(projectRoot)
}

// Register writes an instance registry file for the given daemon.
// The file is named by the SHA-256 hash of the project root.
// Register records an instance without estate context (legacy signature).
func Register(projectRoot, socketPath, dbPath string) error {
	return RegisterWithParents(projectRoot, socketPath, dbPath, nil)
}

// RegisterWithParents records an instance including its anchor-chain
// ancestor roots for estate-aware consumers.
func RegisterWithParents(projectRoot, socketPath, dbPath string, parents []string) error {
	dir, err := InstancesDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("cannot create instances directory: %w", err)
	}

	normalized := NormalizeRoot(projectRoot)
	if normalized != projectRoot {
		// Best-effort: drop a registration written under the alias spelling
		// so the project has exactly one registry identity.
		_ = os.Remove(filepath.Join(dir, hashProjectRoot(projectRoot)+".json"))
		projectRoot = normalized
	}

	inst := Instance{
		Parents:      parents,
		ProjectRoot:  projectRoot,
		ProjectName:  filepath.Base(projectRoot),
		SocketPath:   socketPath,
		DBPath:       dbPath,
		PID:          os.Getpid(),
		Version:      version.Version,
		RegisteredAt: time.Now().UTC(),
	}

	data, err := json.MarshalIndent(inst, "", "  ")
	if err != nil {
		return fmt.Errorf("cannot marshal instance: %w", err)
	}

	filePath := filepath.Join(dir, hashProjectRoot(projectRoot)+".json")
	if err := os.WriteFile(filePath, data, 0o644); err != nil {
		return fmt.Errorf("cannot write instance file: %w", err)
	}
	return nil
}

// Unregister removes the instance registry file for the given project root.
func Unregister(projectRoot string) error {
	dir, err := InstancesDir()
	if err != nil {
		return err
	}
	// Remove the registration under both the given and the normalized
	// spelling — either may have been written by older binaries.
	if normalized := NormalizeRoot(projectRoot); normalized != projectRoot {
		_ = os.Remove(filepath.Join(dir, hashProjectRoot(normalized)+".json"))
	}
	filePath := filepath.Join(dir, hashProjectRoot(projectRoot)+".json")
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("cannot remove instance file: %w", err)
	}
	return nil
}

// List reads all instance registry files and returns them.
// Callers should health-check each instance to determine if it's still alive.
func List() ([]Instance, error) {
	dir, err := InstancesDir()
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("cannot read instances directory: %w", err)
	}

	var instances []Instance
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		var inst Instance
		if err := json.Unmarshal(data, &inst); err != nil {
			continue
		}
		instances = append(instances, inst)
	}
	return instances, nil
}
