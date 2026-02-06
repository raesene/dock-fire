package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

const configPath = "/etc/dock-fire/config.json"

type initConfig struct {
	Args []string `json:"args"`
	Env  []string `json:"env"`
	Cwd  string   `json:"cwd"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "dock-fire-init: %v\n", err)
		reboot()
	}
}

func run() error {
	// Mount essential filesystems
	mounts := []struct {
		source string
		target string
		fstype string
		flags  uintptr
	}{
		{"proc", "/proc", "proc", 0},
		{"sysfs", "/sys", "sysfs", 0},
		{"devtmpfs", "/dev", "devtmpfs", 0},
	}

	for _, m := range mounts {
		os.MkdirAll(m.target, 0o755)
		syscall.Mount(m.source, m.target, m.fstype, m.flags, "")
	}

	// Set up DNS if not already configured (Docker creates empty resolv.conf with --net=none)
	if data, err := os.ReadFile("/etc/resolv.conf"); err != nil || len(data) == 0 {
		os.WriteFile("/etc/resolv.conf", []byte("nameserver 8.8.8.8\nnameserver 8.8.4.4\n"), 0o644)
	}

	// Read config
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}
	var cfg initConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	if len(cfg.Args) == 0 {
		return fmt.Errorf("no command specified")
	}

	// Change working directory
	if cfg.Cwd != "" {
		if err := os.Chdir(cfg.Cwd); err != nil {
			return fmt.Errorf("chdir %s: %w", cfg.Cwd, err)
		}
	}

	// Set up environment early so LookPath can use PATH
	env := cfg.Env
	if len(env) == 0 {
		env = []string{
			"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
			"TERM=xterm",
		}
	}
	// Apply env to current process so LookPath works
	for _, e := range env {
		parts := splitEnvVar(e)
		if parts[0] == "PATH" {
			os.Setenv("PATH", parts[1])
		}
	}

	// Resolve the command
	binary, err := exec.LookPath(cfg.Args[0])
	if err != nil {
		return fmt.Errorf("resolve command %q: %w", cfg.Args[0], err)
	}

	// Start the child process
	cmd := exec.Command(binary, cfg.Args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = env

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start command: %w", err)
	}

	// Forward signals to the child
	sigCh := make(chan os.Signal, 16)
	signal.Notify(sigCh)
	go func() {
		for sig := range sigCh {
			if s, ok := sig.(syscall.Signal); ok {
				cmd.Process.Signal(s)
			}
		}
	}()

	// Wait for the child to exit
	err = cmd.Wait()

	// Shut down the VM
	reboot()
	return err // unreachable, but for completeness
}

func splitEnvVar(s string) [2]string {
	for i := 0; i < len(s); i++ {
		if s[i] == '=' {
			return [2]string{s[:i], s[i+1:]}
		}
	}
	return [2]string{s, ""}
}

func reboot() {
	syscall.Sync()
	syscall.Reboot(syscall.LINUX_REBOOT_CMD_RESTART)
	os.Exit(0)
}
