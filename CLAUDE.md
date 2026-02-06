# dock-fire

OCI-compliant container runtime that boots Docker containers inside Firecracker microVMs. Used via `docker run --runtime=dock-fire`.

## Build

```bash
make all        # builds bin/dock-fire and bin/dock-fire-init (static)
make install    # copies binaries to /usr/local/bin (requires root)
make clean      # removes bin/
```

`dock-fire-init` must be statically linked (`CGO_ENABLED=0`). The Makefile handles this.

## Project structure

```
cmd/dock-fire/main.go          CLI entry point (urfave/cli/v2)
cmd/dock-fire-init/main.go     Static binary, PID 1 inside the guest VM
internal/runtime/               OCI commands: create, start, state, kill, delete
internal/container/             Container state machine + JSON persistence
internal/vm/                    Firecracker VM lifecycle via firecracker-go-sdk
internal/network/               TAP devices, /30 IP allocation, iptables NAT
internal/rootfs/                OCI rootfs -> ext4 image conversion
internal/oci/                   OCI config.json parsing + state output
```

## Architecture

Docker/containerd invokes `dock-fire` as an OCI runtime binary. Each container gets its own Firecracker microVM:

1. `create` - Parses OCI config, builds ext4 image from rootfs, sets up TAP networking, boots Firecracker VM, writes VMM PID to pid-file
2. `start` - No-op state transition (created -> running). VM already runs from create.
3. `state` - Returns OCI state JSON, checks if VMM PID is alive
4. `kill` - Sends signal to VMM process
5. `delete` - Kills VM, tears down networking, removes state

The VM boots during `create` (not `start`) because containerd needs a valid VMM PID immediately.

## Key design decisions

- **Socket path**: `/tmp/fc-<12 chars>.sock` to stay under 108-char Unix socket limit
- **Firecracker log**: `--log-path` CLI flag (not API) to catch early messages before API is ready
- **SDK logger**: Uses `WithLogger()` - the SDK creates its own logrus instance, ignoring the global one
- **Kernel boot args**: `loglevel=0` suppresses all console messages; `quiet` alone is insufficient
- **Serial console I/O**: Pass `os.Stdout` (`*os.File`) directly to Firecracker so the fd is inherited by the child process and survives dock-fire exit. Using `io.Writer` creates a pipe+goroutine that breaks.
- **Docker networking**: Must use `--net=none` because Docker bridge conflicts with dock-fire's own TAP networking
- **DNS**: `dock-fire-init` writes `/etc/resolv.conf` with 8.8.8.8 if empty (Docker creates empty resolv.conf with `--net=none`)
- **Networking**: TAP devices named `df-<8 chars>`, /30 subnets from 10.0.0.0/16, iptables MASQUERADE

## Test server

- Host: 192.168.41.108, user: rorym
- Firecracker v1.11.0 at `/usr/local/bin/firecracker`
- Kernel at `/var/lib/vmm/images/kernels/vmlinux.bin`
- Docker 28.2.2, containerd 1.7.28

Deploy and test:
```bash
make all
scp bin/* rorym@192.168.41.108:/tmp/
ssh rorym@192.168.41.108 'sudo cp /tmp/dock-fire* /usr/local/bin/'
ssh rorym@192.168.41.108 'sudo docker run --runtime=dock-fire --net=none --rm alpine echo hello'
```

## Common issues

- **urfave/cli**: Flags must come before positional args
- **firecracker-go-sdk**: Uses pointer types for struct fields (`Int64()`, `Bool()`, `String()` helpers)
- **Stale TAP devices**: If containers aren't cleaned up, stale TAP devices cause routing conflicts. Clean up with `ip link del <tap-name>`
- **ext4 creation**: Uses shell tools (`mkfs.ext4`, `mount`, `cp -a`) since the runtime runs as root
