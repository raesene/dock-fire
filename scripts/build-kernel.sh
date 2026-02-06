#!/bin/bash
#
# build-kernel.sh - Build a Firecracker-compatible Linux kernel
#
# Downloads, configures, and builds a Linux kernel suitable for Firecracker
# microVMs. Auto-detects the latest patch version for a given kernel series.
#
# Usage: ./build-kernel.sh [SERIES]
#   SERIES defaults to 6.1 (matches Firecracker's official config)
#
# Outputs vmlinux.bin and kernel-version.txt to the current directory.
#
# Dependencies: build-essential flex bison bc libelf-dev libssl-dev wget curl jq
#

set -euo pipefail

SERIES="${1:-6.1}"
BUILD_DIR=""

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info()  { echo -e "${GREEN}[INFO]${NC} $1" >&2; }
log_warn()  { echo -e "${YELLOW}[WARN]${NC} $1" >&2; }
log_error() { echo -e "${RED}[ERROR]${NC} $1" >&2; }

cleanup() {
    if [[ -n "$BUILD_DIR" && -d "$BUILD_DIR" ]]; then
        log_info "Cleaning up build directory..."
        rm -rf "$BUILD_DIR"
    fi
}
trap cleanup EXIT

check_dependencies() {
    log_info "Checking build dependencies..."

    local packages=(build-essential flex bison bc libelf-dev libssl-dev wget curl jq)
    local missing=()

    for pkg in "${packages[@]}"; do
        if ! dpkg -s "$pkg" &>/dev/null; then
            missing+=("$pkg")
        fi
    done

    if [[ ${#missing[@]} -ne 0 ]]; then
        log_error "Missing required packages: ${missing[*]}"
        log_info "Install them with: sudo apt-get install ${missing[*]}"
        exit 1
    fi

    log_info "All dependencies satisfied"
}

detect_latest_patch() {
    local series="$1"

    log_info "Detecting latest patch version for ${series}.x ..."

    local version
    version=$(curl -fsSL https://www.kernel.org/releases.json \
        | jq -r --arg s "$series." '.releases[] | select(.version | startswith($s)) | .version' \
        | head -1)

    if [[ -z "$version" ]]; then
        log_error "Could not find any ${series}.x release on kernel.org"
        exit 1
    fi

    echo "$version"
}

download_kernel() {
    local version="$1"
    local major="${version%%.*}"

    local url="https://cdn.kernel.org/pub/linux/kernel/v${major}.x/linux-${version}.tar.xz"
    local filename="linux-${version}.tar.xz"

    log_info "Downloading kernel source: ${url}"

    wget -q --show-progress -O "$BUILD_DIR/$filename" "$url"

    log_info "Extracting kernel source..."
    tar -xf "$BUILD_DIR/$filename" -C "$BUILD_DIR"

    echo "$BUILD_DIR/linux-${version}"
}

create_kernel_config() {
    local kernel_dir="$1"
    local arch
    arch="$(uname -m)"

    cd "$kernel_dir"

    # Download Firecracker's recommended config for this series
    local config_url="https://raw.githubusercontent.com/firecracker-microvm/firecracker/main/resources/guest_configs/microvm-kernel-ci-${arch}-${SERIES}.config"

    log_info "Downloading Firecracker kernel config: ${config_url}"
    if ! wget -q -O .config "$config_url"; then
        log_warn "Failed to download Firecracker config, trying non-CI config..."
        config_url="https://raw.githubusercontent.com/firecracker-microvm/firecracker/main/resources/guest_configs/microvm-kernel-${arch}-${SERIES}.config"
        if ! wget -q -O .config "$config_url"; then
            log_error "Could not download Firecracker kernel config"
            exit 1
        fi
    fi

    log_info "Customizing kernel configuration..."

    # Essential Firecracker options
    # PCI is required on mainline kernels for ACPI device discovery.
    # Firecracker's CI config targets Amazon Linux which has patches
    # to support ACPI without PCI; mainline kernels need PCI enabled.
    ./scripts/config --enable CONFIG_PCI
    ./scripts/config --enable CONFIG_VIRTIO
    ./scripts/config --enable CONFIG_VIRTIO_MMIO
    ./scripts/config --enable CONFIG_VIRTIO_MMIO_CMDLINE_DEVICES
    ./scripts/config --enable CONFIG_VIRTIO_BLK
    ./scripts/config --enable CONFIG_VIRTIO_NET
    ./scripts/config --enable CONFIG_SERIAL_8250
    ./scripts/config --enable CONFIG_SERIAL_8250_CONSOLE
    ./scripts/config --enable CONFIG_EXT4_FS
    ./scripts/config --enable CONFIG_NET
    ./scripts/config --enable CONFIG_INET

    # Overlay filesystem (required for Docker)
    ./scripts/config --enable CONFIG_OVERLAY_FS

    # Netfilter/iptables (required for Docker networking)
    ./scripts/config --enable CONFIG_NETFILTER
    ./scripts/config --enable CONFIG_NETFILTER_ADVANCED
    ./scripts/config --enable CONFIG_NETFILTER_XTABLES
    ./scripts/config --enable CONFIG_NETFILTER_NETLINK
    ./scripts/config --enable CONFIG_NETFILTER_NETLINK_QUEUE
    ./scripts/config --enable CONFIG_NETFILTER_NETLINK_LOG

    # nf_tables (used by iptables-nft on modern systems)
    ./scripts/config --enable CONFIG_NF_TABLES
    ./scripts/config --enable CONFIG_NF_TABLES_INET
    ./scripts/config --enable CONFIG_NF_TABLES_NETDEV
    ./scripts/config --enable CONFIG_NFT_NUMGEN
    ./scripts/config --enable CONFIG_NFT_CT
    ./scripts/config --enable CONFIG_NFT_COUNTER
    ./scripts/config --enable CONFIG_NFT_CONNLIMIT
    ./scripts/config --enable CONFIG_NFT_LOG
    ./scripts/config --enable CONFIG_NFT_LIMIT
    ./scripts/config --enable CONFIG_NFT_MASQ
    ./scripts/config --enable CONFIG_NFT_REDIR
    ./scripts/config --enable CONFIG_NFT_NAT
    ./scripts/config --enable CONFIG_NFT_REJECT
    ./scripts/config --enable CONFIG_NFT_COMPAT
    ./scripts/config --enable CONFIG_NFT_HASH
    ./scripts/config --enable CONFIG_NFT_FIB
    ./scripts/config --enable CONFIG_NFT_FIB_INET

    # Connection tracking (required for NAT/masquerade)
    ./scripts/config --enable CONFIG_NF_CONNTRACK
    ./scripts/config --enable CONFIG_NF_NAT
    ./scripts/config --enable CONFIG_NF_NAT_MASQUERADE

    # IPv4 netfilter
    ./scripts/config --enable CONFIG_NF_TABLES_IPV4
    ./scripts/config --enable CONFIG_NFT_CHAIN_ROUTE_IPV4
    ./scripts/config --enable CONFIG_NFT_FIB_IPV4
    ./scripts/config --enable CONFIG_NF_REJECT_IPV4
    ./scripts/config --enable CONFIG_IP_NF_IPTABLES
    ./scripts/config --enable CONFIG_IP_NF_FILTER
    ./scripts/config --enable CONFIG_IP_NF_TARGET_REJECT
    ./scripts/config --enable CONFIG_IP_NF_NAT
    ./scripts/config --enable CONFIG_IP_NF_TARGET_MASQUERADE

    # IPv6 netfilter (Docker also uses IPv6)
    ./scripts/config --enable CONFIG_NF_TABLES_IPV6
    ./scripts/config --enable CONFIG_NFT_CHAIN_ROUTE_IPV6
    ./scripts/config --enable CONFIG_NFT_FIB_IPV6
    ./scripts/config --enable CONFIG_NF_REJECT_IPV6
    ./scripts/config --enable CONFIG_IP6_NF_IPTABLES
    ./scripts/config --enable CONFIG_IP6_NF_FILTER
    ./scripts/config --enable CONFIG_IP6_NF_TARGET_REJECT
    ./scripts/config --enable CONFIG_IP6_NF_NAT
    ./scripts/config --enable CONFIG_IP6_NF_TARGET_MASQUERADE

    # Network device drivers (required for Docker)
    ./scripts/config --enable CONFIG_BRIDGE
    ./scripts/config --enable CONFIG_VETH
    ./scripts/config --enable CONFIG_VLAN_8021Q
    ./scripts/config --enable CONFIG_MACVLAN
    ./scripts/config --enable CONFIG_IPVLAN
    ./scripts/config --enable CONFIG_DUMMY

    # Bridge netfilter (for Docker bridge networks)
    ./scripts/config --enable CONFIG_NF_TABLES_BRIDGE
    ./scripts/config --enable CONFIG_BRIDGE_NF_EBTABLES
    ./scripts/config --enable CONFIG_BRIDGE_EBT_BROUTE
    ./scripts/config --enable CONFIG_BRIDGE_EBT_T_FILTER
    ./scripts/config --enable CONFIG_BRIDGE_EBT_T_NAT

    # BPF support (required for Docker device cgroup)
    ./scripts/config --enable CONFIG_BPF
    ./scripts/config --enable CONFIG_BPF_SYSCALL
    ./scripts/config --enable CONFIG_BPF_JIT
    ./scripts/config --enable CONFIG_BPF_JIT_ALWAYS_ON

    # Cgroups (required for Docker container resource management)
    ./scripts/config --enable CONFIG_CGROUPS
    ./scripts/config --enable CONFIG_CGROUP_FREEZER
    ./scripts/config --enable CONFIG_CGROUP_PIDS
    ./scripts/config --enable CONFIG_CGROUP_DEVICE
    ./scripts/config --enable CONFIG_CPUSETS
    ./scripts/config --enable CONFIG_CGROUP_CPUACCT
    ./scripts/config --enable CONFIG_MEMCG
    ./scripts/config --enable CONFIG_CGROUP_SCHED
    ./scripts/config --enable CONFIG_CGROUP_BPF

    # Namespaces (required for Docker container isolation)
    ./scripts/config --enable CONFIG_NAMESPACES
    ./scripts/config --enable CONFIG_UTS_NS
    ./scripts/config --enable CONFIG_IPC_NS
    ./scripts/config --enable CONFIG_USER_NS
    ./scripts/config --enable CONFIG_PID_NS
    ./scripts/config --enable CONFIG_NET_NS

    # Disable modules - we want everything built-in
    ./scripts/config --disable CONFIG_MODULES

    # Disable initramfs - we boot directly to rootfs
    ./scripts/config --disable CONFIG_BLK_DEV_INITRD

    # Resolve dependencies
    make olddefconfig
}

build_kernel() {
    local kernel_dir="$1"
    local nproc
    nproc="$(nproc)"

    log_info "Building kernel with ${nproc} parallel jobs..."
    log_info "This may take 10-30 minutes depending on your system."

    cd "$kernel_dir"
    make -j"$nproc" vmlinux

    if [[ ! -f vmlinux ]]; then
        log_error "Kernel build failed - vmlinux not found"
        exit 1
    fi

    log_info "Kernel build complete"
}

# --- Main ---

OUTPUT_DIR="$(pwd)"

check_dependencies

FULL_VERSION="$(detect_latest_patch "$SERIES")"
log_info "Building kernel ${FULL_VERSION}"

BUILD_DIR="$(mktemp -d -t dock-fire-kernel-build-XXXXXX)"
log_info "Build directory: ${BUILD_DIR}"

KERNEL_DIR="$(download_kernel "$FULL_VERSION")"
create_kernel_config "$KERNEL_DIR"
build_kernel "$KERNEL_DIR"

# Copy outputs to the original working directory
cp "$KERNEL_DIR/vmlinux" "$OUTPUT_DIR/vmlinux.bin"
echo "$FULL_VERSION" > "$OUTPUT_DIR/kernel-version.txt"

SIZE=$(du -h "$OUTPUT_DIR/vmlinux.bin" | cut -f1)
log_info "Output: vmlinux.bin (${SIZE}), kernel-version.txt (${FULL_VERSION})"
log_info "Done!"
