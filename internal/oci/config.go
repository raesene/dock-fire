package oci

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	specs "github.com/opencontainers/runtime-spec/specs-go"
)

// LoadConfig reads and parses the OCI config.json from a bundle directory.
func LoadConfig(bundlePath string) (*specs.Spec, error) {
	configPath := filepath.Join(bundlePath, "config.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("read config.json: %w", err)
	}
	var spec specs.Spec
	if err := json.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("parse config.json: %w", err)
	}
	return &spec, nil
}
