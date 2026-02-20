package registry

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FileRegistry discovers copts by scanning a base directory for endpoint files.
// Each copt writes its endpoint to <baseDir>/<name>/endpoint.sock.
// This is suitable for single-host discovery.
type FileRegistry struct {
	baseDir string // e.g. /run/nile
}

// NewFileRegistry creates a registry that scans baseDir for copt endpoints.
func NewFileRegistry(baseDir string) *FileRegistry {
	return &FileRegistry{baseDir: baseDir}
}

func (r *FileRegistry) Register(name string, endpoint string) error {
	dir := filepath.Join(r.baseDir, name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("registry: create dir: %w", err)
	}
	path := filepath.Join(dir, "endpoint.sock")
	return os.WriteFile(path, []byte(endpoint), 0644)
}

func (r *FileRegistry) Lookup(name string) (string, error) {
	path := filepath.Join(r.baseDir, name, "endpoint.sock")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", fmt.Errorf("registry: copt %q not found", name)
	}
	if err != nil {
		return "", fmt.Errorf("registry: read endpoint: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}

func (r *FileRegistry) List() ([]CoptInfo, error) {
	entries, err := os.ReadDir(r.baseDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("registry: list: %w", err)
	}

	var copts []CoptInfo
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		endpoint, err := r.Lookup(e.Name())
		if err != nil {
			continue // skip copts without endpoint files
		}
		copts = append(copts, CoptInfo{
			Name:     e.Name(),
			Endpoint: endpoint,
		})
	}
	return copts, nil
}

func (r *FileRegistry) Deregister(name string) error {
	path := filepath.Join(r.baseDir, name, "endpoint.sock")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("registry: deregister: %w", err)
	}
	return nil
}
