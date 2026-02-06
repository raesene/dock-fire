package rootfs

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/sirupsen/logrus"

	specs "github.com/opencontainers/runtime-spec/specs-go"
)

// InitConfig is the configuration written to /etc/dock-fire/config.json inside the VM.
type InitConfig struct {
	Args     []string `json:"args"`
	Env      []string `json:"env"`
	Cwd      string   `json:"cwd"`
	Terminal bool     `json:"terminal,omitempty"`
}

// CreateImage converts an OCI rootfs directory into an ext4 block device image.
// It copies the rootfs contents, the dock-fire-init binary, and the init config.
func CreateImage(rootDir, id, rootfsPath string, spec *specs.Spec) (string, error) {
	stateDir := filepath.Join(rootDir, id)
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return "", fmt.Errorf("mkdir state dir: %w", err)
	}

	imagePath := filepath.Join(stateDir, "rootfs.ext4")
	mountPoint := filepath.Join(stateDir, "mnt")

	// Calculate image size: rootfs + 100MB padding
	rootfsSize, err := dirSize(rootfsPath)
	if err != nil {
		return "", fmt.Errorf("calculate rootfs size: %w", err)
	}
	// Minimum 100MB, rootfs + 100MB padding
	imageSize := rootfsSize + 100*1024*1024
	if imageSize < 100*1024*1024 {
		imageSize = 100 * 1024 * 1024
	}
	logrus.Debugf("rootfs size: %d bytes, image size: %d bytes", rootfsSize, imageSize)

	// Create sparse file
	if err := exec.Command("truncate", "-s", fmt.Sprintf("%d", imageSize), imagePath).Run(); err != nil {
		return "", fmt.Errorf("truncate: %w", err)
	}

	// Format as ext4
	mkfsOut, err := exec.Command("mkfs.ext4", "-q", "-F", imagePath).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("mkfs.ext4: %w: %s", err, mkfsOut)
	}

	// Mount
	if err := os.MkdirAll(mountPoint, 0o755); err != nil {
		return "", fmt.Errorf("mkdir mount point: %w", err)
	}
	if out, err := exec.Command("mount", "-o", "loop", imagePath, mountPoint).CombinedOutput(); err != nil {
		return "", fmt.Errorf("mount: %w: %s", err, out)
	}
	defer func() {
		exec.Command("umount", mountPoint).Run()
		os.Remove(mountPoint)
	}()

	// Copy rootfs contents
	if out, err := exec.Command("cp", "-a", rootfsPath+"/.", mountPoint+"/").CombinedOutput(); err != nil {
		return "", fmt.Errorf("cp rootfs: %w: %s", err, out)
	}

	// Copy dock-fire-init binary
	initBin, err := findInitBinary()
	if err != nil {
		return "", fmt.Errorf("find dock-fire-init: %w", err)
	}
	initDst := filepath.Join(mountPoint, "sbin", "dock-fire-init")
	if err := os.MkdirAll(filepath.Dir(initDst), 0o755); err != nil {
		return "", fmt.Errorf("mkdir /sbin: %w", err)
	}
	if out, err := exec.Command("cp", initBin, initDst).CombinedOutput(); err != nil {
		return "", fmt.Errorf("cp init binary: %w: %s", err, out)
	}
	if err := os.Chmod(initDst, 0o755); err != nil {
		return "", fmt.Errorf("chmod init binary: %w", err)
	}

	// Write init config
	initCfg := InitConfig{
		Cwd: "/",
	}
	if spec.Process != nil {
		initCfg.Args = spec.Process.Args
		initCfg.Env = spec.Process.Env
		initCfg.Terminal = spec.Process.Terminal
		if spec.Process.Cwd != "" {
			initCfg.Cwd = spec.Process.Cwd
		}
	}
	cfgDir := filepath.Join(mountPoint, "etc", "dock-fire")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir config dir: %w", err)
	}
	cfgData, err := json.MarshalIndent(initCfg, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal init config: %w", err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.json"), cfgData, 0o644); err != nil {
		return "", fmt.Errorf("write init config: %w", err)
	}

	logrus.Debugf("created rootfs image at %s", imagePath)
	return imagePath, nil
}

func dirSize(path string) (int64, error) {
	var size int64
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size, err
}

func findInitBinary() (string, error) {
	// Check common locations
	candidates := []string{
		"/usr/local/bin/dock-fire-init",
		"/usr/bin/dock-fire-init",
	}

	// Also check next to the current binary
	exe, err := os.Executable()
	if err == nil {
		candidates = append([]string{filepath.Join(filepath.Dir(exe), "dock-fire-init")}, candidates...)
	}

	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("dock-fire-init not found in any of: %v", candidates)
}
