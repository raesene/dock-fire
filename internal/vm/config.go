package vm

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	firecracker "github.com/firecracker-microvm/firecracker-go-sdk"
	models "github.com/firecracker-microvm/firecracker-go-sdk/client/models"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/rorym/dock-fire/internal/container"
	"github.com/sirupsen/logrus"
)

const (
	DefaultKernelPath   = "/var/lib/vmm/images/kernels/vmlinux.bin"
	DefaultVCPUs        = 1
	DefaultMemMB        = 128
	DefaultFirecracker  = "firecracker"
)

// kernelPath returns the guest kernel path, preferring the DOCK_FIRE_KERNEL_PATH
// environment variable over the compiled-in default.
func kernelPath() string {
	if p := os.Getenv("DOCK_FIRE_KERNEL_PATH"); p != "" {
		return p
	}
	return DefaultKernelPath
}

// vcpuCount returns the number of vCPUs for the VM.
// Priority: annotation "dock-fire/vcpus" > env var DOCK_FIRE_VCPUS > DefaultVCPUs.
func vcpuCount(spec *specs.Spec) int64 {
	if spec.Annotations != nil {
		if v, ok := spec.Annotations["dock-fire/vcpus"]; ok {
			if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
				return n
			}
			logrus.Warnf("ignoring invalid dock-fire/vcpus annotation %q", v)
		}
	}
	if v := os.Getenv("DOCK_FIRE_VCPUS"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			return n
		}
		logrus.Warnf("ignoring invalid DOCK_FIRE_VCPUS=%q", v)
	}
	return DefaultVCPUs
}

// memSizeMB returns the memory size in MiB for the VM.
// Priority: annotation "dock-fire/memory" > env var DOCK_FIRE_MEMORY > DefaultMemMB.
// Accepts plain MiB ("256"), megabytes ("256M"), or gigabytes ("1G").
func memSizeMB(spec *specs.Spec) int64 {
	if spec.Annotations != nil {
		if v, ok := spec.Annotations["dock-fire/memory"]; ok {
			if n, err := parseMemSize(v); err == nil {
				return n
			}
			logrus.Warnf("ignoring invalid dock-fire/memory annotation %q", v)
		}
	}
	if v := os.Getenv("DOCK_FIRE_MEMORY"); v != "" {
		if n, err := parseMemSize(v); err == nil {
			return n
		}
		logrus.Warnf("ignoring invalid DOCK_FIRE_MEMORY=%q", v)
	}
	return DefaultMemMB
}

// parseMemSize parses a memory size string into MiB.
// Accepts plain MiB ("256"), megabytes ("256M"), or gigabytes ("1G").
func parseMemSize(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty size string")
	}
	suffix := strings.ToUpper(s[len(s)-1:])
	switch suffix {
	case "G":
		n, err := strconv.ParseInt(s[:len(s)-1], 10, 64)
		if err != nil || n <= 0 {
			return 0, fmt.Errorf("invalid memory size %q", s)
		}
		return n * 1024, nil
	case "M":
		n, err := strconv.ParseInt(s[:len(s)-1], 10, 64)
		if err != nil || n <= 0 {
			return 0, fmt.Errorf("invalid memory size %q", s)
		}
		return n, nil
	default:
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil || n <= 0 {
			return 0, fmt.Errorf("invalid memory size %q", s)
		}
		return n, nil
	}
}

// BuildConfig creates a Firecracker VM config from container state.
func BuildConfig(ctr *container.Container, bootArgs string, spec *specs.Spec) firecracker.Config {
	// Use a short socket path to stay under the 108-char Unix socket limit.
	// Docker sets root to long paths like /var/run/docker/runtime-runc/moby
	// combined with 64-char container IDs.
	socketPath := fmt.Sprintf("/tmp/fc-%s.sock", ctr.ID[:min(len(ctr.ID), 12)])
	ctr.SocketPath = socketPath

	cfg := firecracker.Config{
		SocketPath:      socketPath,
		KernelImagePath: kernelPath(),
		KernelArgs:      bootArgs,
		Drives:          firecracker.NewDrivesBuilder(ctr.ImagePath).Build(),
		MachineCfg: models.MachineConfiguration{
			VcpuCount:  firecracker.Int64(vcpuCount(spec)),
			MemSizeMib: firecracker.Int64(memSizeMB(spec)),
		},
	}

	// Add network interface if networking is configured
	if ctr.TapDevice != "" {
		cfg.NetworkInterfaces = firecracker.NetworkInterfaces{
			firecracker.NetworkInterface{
				StaticConfiguration: &firecracker.StaticNetworkConfiguration{
					MacAddress:  generateMAC(ctr.ID),
					HostDevName: ctr.TapDevice,
				},
			},
		}
	}

	return cfg
}

// BuildBootArgs constructs the kernel boot arguments.
func BuildBootArgs(ctr *container.Container) string {
	args := "console=ttyS0 reboot=k panic=1 pci=off loglevel=0 i8042.noaux i8042.nomux i8042.nopnp i8042.dumbkbd init=/sbin/dock-fire-init"

	// Add networking if configured
	if ctr.GuestIP != "" && ctr.HostIP != "" {
		// Format: ip=<client-ip>::<gw-ip>:<netmask>::<device>:off
		args += fmt.Sprintf(" ip=%s::%s:255.255.255.252::eth0:off", ctr.GuestIP, ctr.HostIP)
	}

	return args
}

// generateMAC creates a deterministic MAC address from the container ID.
func generateMAC(id string) string {
	// Use first 5 bytes of ID hash for MAC (locally administered, unicast)
	b := []byte(id)
	if len(b) < 5 {
		b = append(b, make([]byte, 5-len(b))...)
	}
	return fmt.Sprintf("02:%02x:%02x:%02x:%02x:%02x", b[0], b[1], b[2], b[3], b[4])
}
