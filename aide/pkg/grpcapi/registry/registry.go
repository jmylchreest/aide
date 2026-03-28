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
type Instance struct {
	ProjectRoot  string    `json:"project_root"`
	ProjectName  string    `json:"project_name"`
	SocketPath   string    `json:"socket_path"`
	DBPath       string    `json:"db_path"`
	PID          int       `json:"pid"`
	Version      string    `json:"version"`
	RegisteredAt time.Time `json:"registered_at"`
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

// Register writes an instance registry file for the given daemon.
// The file is named by the SHA-256 hash of the project root.
func Register(projectRoot, socketPath, dbPath string) error {
	dir, err := InstancesDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("cannot create instances directory: %w", err)
	}

	inst := Instance{
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
