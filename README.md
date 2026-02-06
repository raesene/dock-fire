# dock-fire

An OCI-compliant container runtime that boots Docker containers inside [Firecracker](https://firecracker-microvm.github.io/) microVMs. Each container runs in its own lightweight VM, providing hardware-level isolation while preserving the standard `docker run` workflow.

```
docker run --runtime=dock-fire --net=none --rm alpine echo "Hello from Firecracker"
Hello from Firecracker
```

## How it works

```
Docker CLI  -->  dockerd (--runtime=dock-fire)
                    |
              containerd + containerd-shim-runc-v2
                    |
              dock-fire create/start/state/kill/delete
                    |
              Firecracker VMM  -->  Guest Kernel  -->  dock-fire-init (PID 1)
                                                            |
                                                       Container process
```

When Docker runs a container with `--runtime=dock-fire`, containerd invokes the `dock-fire` binary as an OCI runtime. Instead of using Linux namespaces and cgroups (like runc), dock-fire:

1. Converts the OCI rootfs into an ext4 block device image
2. Creates a TAP network device with NAT for internet access
3. Boots a Firecracker microVM with the rootfs as its root drive
4. Runs `dock-fire-init` as PID 1 inside the VM, which executes the container command
5. Serial console output from the VM flows back to Docker as container output

## Prerequisites

dock-fire requires an **x86_64 Linux host** with:

- KVM support (`/dev/kvm` must be accessible)
- Docker (tested with 28.2.2) and containerd (tested with 1.7.28)
- Root access (for Firecracker, TAP devices, and iptables)

## Quick start

The install script handles everything — Go, Firecracker, guest kernel, build, Docker config, and a smoke test:

```bash
git clone https://github.com/raesene/dock-fire.git
cd dock-fire
sudo ./install.sh
```

The script is idempotent and will skip components that are already installed.

## Manual installation

### 1. Install Firecracker

Download the Firecracker binary (tested with v1.11.0):

```bash
FIRECRACKER_VERSION=1.11.0
curl -fsSL "https://github.com/firecracker-microvm/firecracker/releases/download/v${FIRECRACKER_VERSION}/firecracker-v${FIRECRACKER_VERSION}-x86_64.tgz" | \
  sudo tar -xz -C /usr/local/bin --strip-components=1 "release-v${FIRECRACKER_VERSION}-x86_64/firecracker-v${FIRECRACKER_VERSION}-x86_64"
sudo mv /usr/local/bin/firecracker-v${FIRECRACKER_VERSION}-x86_64 /usr/local/bin/firecracker
```

Verify:

```bash
firecracker --version
```

### 2. Install a guest kernel

Firecracker needs an uncompressed Linux kernel (`vmlinux`). The default path is `/var/lib/vmm/images/kernels/vmlinux.bin`.

```bash
sudo mkdir -p /var/lib/vmm/images/kernels

# Download the latest prebuilt kernel from GitHub Releases
TAG=$(curl -fsSL https://api.github.com/repos/raesene/dock-fire/releases \
  | jq -r '[.[] | select(.tag_name | startswith("kernel-6.1."))] | sort_by(.created_at) | last | .tag_name')
curl -fsSL -L "https://github.com/raesene/dock-fire/releases/download/${TAG}/vmlinux.bin" \
  -o /tmp/vmlinux.bin
sudo mv /tmp/vmlinux.bin /var/lib/vmm/images/kernels/vmlinux.bin
```

You can also override the kernel path with an environment variable:

```bash
export DOCK_FIRE_KERNEL_PATH=/path/to/your/vmlinux.bin
```

### 3. Install system dependencies

dock-fire uses standard Linux tools for rootfs image creation and networking:

```bash
sudo apt-get update
sudo apt-get install -y e2fsprogs iproute2 iptables
```

### 4. Build dock-fire

Requires Go 1.21 or later:

```bash
# Install Go if not already installed
# See https://go.dev/doc/install

git clone https://github.com/raesene/dock-fire.git
cd dock-fire
make all
sudo make install
```

This installs two binaries:
- `/usr/local/bin/dock-fire` - The OCI runtime binary
- `/usr/local/bin/dock-fire-init` - The guest init process (statically linked)

### 5. Configure Docker

Register dock-fire as a Docker runtime by adding it to `/etc/docker/daemon.json`:

```bash
# Create or edit /etc/docker/daemon.json
cat <<EOF | sudo tee /etc/docker/daemon.json
{
  "runtimes": {
    "dock-fire": {
      "path": "/usr/local/bin/dock-fire"
    }
  }
}
EOF

# Restart Docker to pick up the new runtime
sudo systemctl restart docker
```

### 6. Verify installation

```bash
sudo docker run --runtime=dock-fire --net=none --rm alpine echo "Hello from Firecracker"
```

You should see:

```
Hello from Firecracker
```

## Usage

### Running containers

Use `--runtime=dock-fire` and `--net=none` with any Docker command:

```bash
# Basic command
sudo docker run --runtime=dock-fire --net=none --rm alpine echo "Hello from Firecracker"

# Interactive shell
sudo docker run --runtime=dock-fire --net=none --rm -it alpine sh

# Background container
sudo docker run --runtime=dock-fire --net=none -d --name my-vm alpine sleep 3600

# Check running containers
sudo docker ps

# Stop and remove
sudo docker stop my-vm
sudo docker rm my-vm
```

### Networking

dock-fire provides its own networking via TAP devices and NAT. Each container gets a dedicated /30 subnet from the `10.0.0.0/16` range with full internet access:

```bash
# Ping external hosts
sudo docker run --runtime=dock-fire --net=none --rm alpine ping -c3 8.8.8.8

# DNS resolution works automatically
sudo docker run --runtime=dock-fire --net=none --rm alpine nslookup google.com

# HTTP access
sudo docker run --runtime=dock-fire --net=none --rm alpine wget -qO- http://ifconfig.me/ip
```

The `--net=none` flag is **required** because Docker's default bridge networking conflicts with dock-fire's TAP-based networking. dock-fire handles all networking internally.

### Guest kernel

The container runs inside a VM with its own Linux kernel (separate from the host). You can verify this:

```bash
# Show the guest kernel version (different from the host)
sudo docker run --runtime=dock-fire --net=none --rm alpine uname -r
```

To use a different kernel, set `DOCK_FIRE_KERNEL_PATH` in the environment where the Docker daemon runs:

```bash
export DOCK_FIRE_KERNEL_PATH=/path/to/your/vmlinux.bin
```

## Building a custom kernel

The `scripts/build-kernel.sh` script builds a Firecracker-compatible kernel from source. It auto-detects the latest patch version for a given kernel series:

```bash
# Build the latest 6.1.x kernel (default)
./scripts/build-kernel.sh

# Build a different series
./scripts/build-kernel.sh 6.6
```

This outputs `vmlinux.bin` and `kernel-version.txt` to the current directory. Build dependencies: `build-essential flex bison bc libelf-dev libssl-dev wget curl jq`.

The kernel is based on Firecracker's official config with additional options enabled for Docker-in-VM support (overlayfs, cgroups, namespaces, netfilter) and `CONFIG_PCI` for mainline kernel compatibility.

A [GitHub Actions workflow](.github/workflows/build-kernel.yml) automatically builds and publishes new kernel patch releases weekly to [GitHub Releases](https://github.com/raesene/dock-fire/releases).

## Resource defaults

Each Firecracker VM is configured with:

| Resource | Default |
|----------|---------|
| vCPUs | 1 |
| Memory | 128 MB |
| Root disk | rootfs size + 100 MB padding |
| Network | /30 subnet with NAT |

## Troubleshooting

### "cannot program address ... conflicts with existing route"

This error occurs when using Docker's default networking. Always pass `--net=none`:

```bash
sudo docker run --runtime=dock-fire --net=none --rm alpine echo hello
```

### Container fails to start

Check that Firecracker and the guest kernel are installed:

```bash
firecracker --version
ls -la /var/lib/vmm/images/kernels/vmlinux.bin
```

Verify KVM is available:

```bash
ls -la /dev/kvm
```

### Stale TAP devices after unclean shutdown

If dock-fire containers are not properly cleaned up (e.g. host crash), stale TAP devices and iptables rules may remain:

```bash
# List stale TAP devices
ip link show | grep "df-"

# Remove a stale TAP device
sudo ip link del df-XXXXXXXX

# List stale iptables rules
sudo iptables -t nat -L POSTROUTING -n | grep "10.0.0"
sudo iptables -L FORWARD -n | grep "df-"
```

### Debugging

Enable debug logging:

```bash
sudo dock-fire --debug --root /run/dock-fire state <container-id>
```

VM stderr logs are stored in the container state directory during the container's lifetime.

## Uninstallation

### 1. Stop all dock-fire containers

```bash
# List dock-fire containers (if any are running)
sudo docker ps

# Stop and remove them
sudo docker stop <container-id>
sudo docker rm <container-id>
```

### 2. Remove Docker runtime configuration

Edit `/etc/docker/daemon.json` and remove the `dock-fire` runtime entry. If dock-fire was the only custom runtime, you can remove the file entirely:

```bash
sudo rm /etc/docker/daemon.json
sudo systemctl restart docker
```

Or if you have other runtimes configured, edit the file to remove just the `dock-fire` entry.

### 3. Remove binaries

```bash
sudo rm /usr/local/bin/dock-fire /usr/local/bin/dock-fire-init
```

### 4. Remove the guest kernel (optional)

```bash
sudo rm -rf /var/lib/vmm/images/kernels
```

### 5. Clean up any remaining state

```bash
# Remove container state directory
sudo rm -rf /run/dock-fire

# Remove any leftover socket or log files
sudo rm -f /tmp/fc-*.sock /tmp/fc-*.log

# Remove stale TAP devices (if any)
for tap in $(ip -o link show | grep "df-" | awk -F: '{print $2}' | tr -d ' '); do
  sudo ip link del "$tap"
done
```

## License

See [LICENSE](LICENSE) for details.
