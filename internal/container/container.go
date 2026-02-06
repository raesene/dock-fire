package container

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

type Status string

const (
	Creating Status = "creating"
	Created  Status = "created"
	Running  Status = "running"
	Stopped  Status = "stopped"
)

// Container holds the persistent state for a single container.
type Container struct {
	ID     string `json:"id"`
	Bundle string `json:"bundle"`
	Status Status `json:"status"`
	PID    int    `json:"pid,omitempty"` // VMM process PID
	// Internal fields not in OCI state
	RootDir  string `json:"rootDir"`  // state directory root (e.g. /run/dock-fire)
	ImagePath string `json:"imagePath,omitempty"` // ext4 rootfs image
	SocketPath string `json:"socketPath,omitempty"` // Firecracker API socket
	TapDevice  string `json:"tapDevice,omitempty"`
	GuestIP   string `json:"guestIP,omitempty"`
	HostIP    string `json:"hostIP,omitempty"`
	SubnetCIDR string `json:"subnetCIDR,omitempty"`
}

func (c *Container) stateDir() string {
	return filepath.Join(c.RootDir, c.ID)
}

func (c *Container) statePath() string {
	return filepath.Join(c.stateDir(), "state.json")
}

// Save persists the container state to disk.
func (c *Container) Save() error {
	dir := c.stateDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("mkdir state dir: %w", err)
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	if err := os.WriteFile(c.statePath(), data, 0o600); err != nil {
		return fmt.Errorf("write state: %w", err)
	}
	return nil
}

// Transition moves the container to a new status, enforcing the state machine.
func (c *Container) Transition(to Status) error {
	valid := map[Status][]Status{
		Creating: {Created},
		Created:  {Running},
		Running:  {Stopped},
	}
	allowed, ok := valid[c.Status]
	if !ok {
		return fmt.Errorf("no transitions from %s", c.Status)
	}
	for _, s := range allowed {
		if s == to {
			c.Status = to
			return nil
		}
	}
	return fmt.Errorf("invalid transition: %s -> %s", c.Status, to)
}

// IsVMMAlive checks whether the VMM process is still running.
func (c *Container) IsVMMAlive() bool {
	if c.PID <= 0 {
		return false
	}
	proc, err := os.FindProcess(c.PID)
	if err != nil {
		return false
	}
	// Signal 0 tests if process exists without actually sending a signal
	return proc.Signal(syscall.Signal(0)) == nil
}

// EffectiveStatus returns the real status, checking if a "running" container's VMM has exited.
func (c *Container) EffectiveStatus() Status {
	if c.Status == Running && !c.IsVMMAlive() {
		return Stopped
	}
	return c.Status
}
