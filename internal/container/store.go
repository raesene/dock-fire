package container

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Load reads container state from the state directory.
func Load(rootDir, id string) (*Container, error) {
	statePath := filepath.Join(rootDir, id, "state.json")
	data, err := os.ReadFile(statePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("container %q does not exist", id)
		}
		return nil, fmt.Errorf("read state: %w", err)
	}
	var c Container
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("unmarshal state: %w", err)
	}
	return &c, nil
}

// Delete removes the container's state directory.
func Delete(rootDir, id string) error {
	dir := filepath.Join(rootDir, id)
	return os.RemoveAll(dir)
}

// List returns all container IDs in the state directory.
func List(rootDir string) ([]string, error) {
	entries, err := os.ReadDir(rootDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var ids []string
	for _, e := range entries {
		if e.IsDir() {
			ids = append(ids, e.Name())
		}
	}
	return ids, nil
}

// Exists checks if a container with the given ID already exists.
func Exists(rootDir, id string) bool {
	statePath := filepath.Join(rootDir, id, "state.json")
	_, err := os.Stat(statePath)
	return err == nil
}
