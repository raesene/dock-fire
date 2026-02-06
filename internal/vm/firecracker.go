package vm

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	firecracker "github.com/firecracker-microvm/firecracker-go-sdk"
	"github.com/rorym/dock-fire/internal/container"
	"github.com/sirupsen/logrus"

	specs "github.com/opencontainers/runtime-spec/specs-go"
)

// Start boots a Firecracker VM for the given container.
// The VMM process runs independently after this function returns.
func Start(ctr *container.Container, spec *specs.Spec) error {
	_ = spec // spec fields already baked into the rootfs image's init config

	bootArgs := BuildBootArgs(ctr)
	cfg := BuildConfig(ctr, bootArgs)

	logrus.Debugf("VM config: kernel=%s rootfs=%s socket=%s", cfg.KernelImagePath, ctr.ImagePath, cfg.SocketPath)
	logrus.Debugf("boot args: %s", bootArgs)

	// Remove stale socket if any
	os.Remove(cfg.SocketPath)

	stateDir := filepath.Join(ctr.RootDir, ctr.ID)

	// Firecracker serial console goes to stdout, and we want it to reach
	// Docker via the containerd shim's pipe. Pass os.Stdout directly -- the
	// child process inherits the fd, so the pipe stays open after dock-fire
	// exits. Stderr captures Firecracker's own API log messages.
	stderrPath := filepath.Join(stateDir, "vm-stderr.log")
	stderrFile, err := os.OpenFile(stderrPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("open stderr log: %w", err)
	}

	// Redirect Firecracker's own log messages (e.g. "[anonymous-instance:main]
	// Running Firecracker v1.11.0") to a file via CLI flag. This must be done
	// via --log-path (not the API) to catch messages emitted before the API is ready.
	// Firecracker requires the log file to exist.
	logPath := filepath.Join(stateDir, "vm-log.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		stderrFile.Close()
		return fmt.Errorf("create log file: %w", err)
	}
	logFile.Close()

	ctx := context.Background()
	cmd := firecracker.VMCommandBuilder{}.
		WithBin(DefaultFirecracker).
		WithSocketPath(cfg.SocketPath).
		AddArgs("--log-path", logPath, "--level", "Error").
		WithStdout(os.Stdout).
		WithStderr(stderrFile).
		Build(ctx)

	// The SDK creates its own logrus logger; redirect it to the stderr log
	// file so it doesn't pollute the serial console output.
	sdkLogger := logrus.New()
	sdkLogger.SetOutput(stderrFile)
	sdkLogger.SetLevel(logrus.WarnLevel)

	machine, err := firecracker.NewMachine(ctx, cfg,
		firecracker.WithProcessRunner(cmd),
		firecracker.WithLogger(logrus.NewEntry(sdkLogger)),
	)
	if err != nil {
		stderrFile.Close()
		return fmt.Errorf("create machine: %w", err)
	}

	if err := machine.Start(ctx); err != nil {
		stderrFile.Close()
		return fmt.Errorf("start machine: %w", err)
	}

	pid, err := machine.PID()
	if err != nil {
		stderrFile.Close()
		return fmt.Errorf("get VMM PID: %w", err)
	}

	ctr.PID = pid
	logrus.Debugf("VM started with PID %d", pid)

	stderrFile.Close()
	return nil
}

// Stop terminates the Firecracker VMM process.
func Stop(ctr *container.Container) error {
	if ctr.PID <= 0 {
		return nil
	}

	proc, err := os.FindProcess(ctr.PID)
	if err != nil {
		return nil
	}

	if err := proc.Signal(syscall.SIGTERM); err != nil {
		logrus.Debugf("SIGTERM to PID %d: %v", ctr.PID, err)
		return nil
	}

	if err := proc.Signal(syscall.Signal(0)); err == nil {
		proc.Signal(syscall.SIGKILL)
	}

	return nil
}

// OutputPath returns the path to the VM stderr log (for debugging).
func OutputPath(rootDir, id string) string {
	return filepath.Join(rootDir, id, "vm-stderr.log")
}
