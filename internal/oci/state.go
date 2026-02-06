package oci

import (
	"encoding/json"

	"github.com/rorym/dock-fire/internal/container"
)

const OCIVersion = "1.0.2"

// State is the OCI runtime state output format.
type State struct {
	OCIVersion string           `json:"ociVersion"`
	ID         string           `json:"id"`
	Status     container.Status `json:"status"`
	PID        int              `json:"pid,omitempty"`
	Bundle     string           `json:"bundle"`
}

// MarshalState returns the JSON-encoded OCI state for a container.
func MarshalState(c *container.Container) ([]byte, error) {
	s := State{
		OCIVersion: OCIVersion,
		ID:         c.ID,
		Status:     c.EffectiveStatus(),
		PID:        c.PID,
		Bundle:     c.Bundle,
	}
	return json.MarshalIndent(s, "", "  ")
}
