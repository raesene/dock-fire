package vm

import (
	"fmt"

	firecracker "github.com/firecracker-microvm/firecracker-go-sdk"
	models "github.com/firecracker-microvm/firecracker-go-sdk/client/models"
	"github.com/rorym/dock-fire/internal/container"
)

const (
	DefaultKernelPath   = "/var/lib/vmm/images/kernels/vmlinux.bin"
	DefaultVCPUs        = 1
	DefaultMemMB        = 128
	DefaultFirecracker  = "firecracker"
)

// BuildConfig creates a Firecracker VM config from container state.
func BuildConfig(ctr *container.Container, bootArgs string) firecracker.Config {
	// Use a short socket path to stay under the 108-char Unix socket limit.
	// Docker sets root to long paths like /var/run/docker/runtime-runc/moby
	// combined with 64-char container IDs.
	socketPath := fmt.Sprintf("/tmp/fc-%s.sock", ctr.ID[:min(len(ctr.ID), 12)])
	ctr.SocketPath = socketPath

	cfg := firecracker.Config{
		SocketPath:      socketPath,
		KernelImagePath: DefaultKernelPath,
		KernelArgs:      bootArgs,
		Drives:          firecracker.NewDrivesBuilder(ctr.ImagePath).Build(),
		MachineCfg: models.MachineConfiguration{
			VcpuCount:  firecracker.Int64(DefaultVCPUs),
			MemSizeMib: firecracker.Int64(DefaultMemMB),
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
	args := "console=ttyS0 loglevel=0 reboot=k panic=1 pci=off i8042.noaux i8042.nomux i8042.nopnp i8042.dumbkbd init=/sbin/dock-fire-init"

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
