#!/usr/bin/env bash
set -euo pipefail

# dock-fire installer
# Installs all dependencies and configures Docker to use dock-fire as a runtime.
# Usage: curl -fsSL https://raw.githubusercontent.com/raesene/dock-fire/main/install.sh | sudo bash

FIRECRACKER_VERSION="1.11.0"
KERNEL_SERIES="6.1"
GITHUB_REPO="raesene/dock-fire"
KERNEL_PATH="/var/lib/vmm/images/kernels/vmlinux.bin"

# --- Colours and output helpers ---

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No colour

ok()      { echo -e "  ${GREEN}[OK]${NC}      $1"; }
skip()    { echo -e "  ${YELLOW}[SKIP]${NC}    $1"; }
install() { echo -e "  ${BLUE}[INSTALL]${NC} $1"; }
step()    { echo -e "\n${GREEN}==>${NC} $1"; }
fail()    { echo -e "  ${RED}[FAIL]${NC}    $1"; exit 1; }

# --- Validation ---

validate_environment() {
    step "Validating environment"

    if [[ $EUID -ne 0 ]]; then
        fail "This script must be run as root (use sudo)"
    fi
    ok "Running as root"

    if [[ "$(uname -m)" != "x86_64" ]]; then
        fail "dock-fire requires x86_64 architecture (found: $(uname -m))"
    fi
    ok "Architecture: x86_64"

    if [[ ! -e /dev/kvm ]]; then
        fail "/dev/kvm not found — KVM support is required"
    fi
    ok "/dev/kvm accessible"

    if ! command -v docker &>/dev/null; then
        fail "Docker is not installed"
    fi
    ok "Docker found: $(docker --version | head -1)"

    if ! docker info &>/dev/null; then
        fail "Docker daemon is not running"
    fi
    ok "Docker daemon is running"
}

# --- System packages ---

install_system_packages() {
    step "Installing system packages"

    local packages=(e2fsprogs iproute2 iptables curl jq)
    local to_install=()

    for pkg in "${packages[@]}"; do
        if dpkg -s "$pkg" &>/dev/null; then
            skip "$pkg already installed"
        else
            to_install+=("$pkg")
        fi
    done

    if [[ ${#to_install[@]} -gt 0 ]]; then
        install "apt-get install ${to_install[*]}"
        apt-get update -qq
        apt-get install -y -qq "${to_install[@]}"
        ok "Installed: ${to_install[*]}"
    fi
}

# --- Firecracker ---

install_firecracker() {
    step "Checking Firecracker"

    if command -v firecracker &>/dev/null; then
        local current
        current=$(firecracker --version 2>&1 | head -1 | grep -oP '[0-9]+\.[0-9]+\.[0-9]+' || true)
        if [[ "$current" == "$FIRECRACKER_VERSION" ]]; then
            skip "Firecracker v${FIRECRACKER_VERSION} already installed"
            return
        fi
        install "Upgrading Firecracker from v${current} to v${FIRECRACKER_VERSION}"
    else
        install "Downloading Firecracker v${FIRECRACKER_VERSION}"
    fi

    local tarball="firecracker-v${FIRECRACKER_VERSION}-x86_64.tgz"
    local url="https://github.com/firecracker-microvm/firecracker/releases/download/v${FIRECRACKER_VERSION}/${tarball}"

    curl -fsSL "$url" -o "/tmp/${tarball}"
    tar -xzf "/tmp/${tarball}" -C /tmp "release-v${FIRECRACKER_VERSION}-x86_64/firecracker-v${FIRECRACKER_VERSION}-x86_64"
    mv "/tmp/release-v${FIRECRACKER_VERSION}-x86_64/firecracker-v${FIRECRACKER_VERSION}-x86_64" /usr/local/bin/firecracker
    chmod +x /usr/local/bin/firecracker
    rm -rf "/tmp/${tarball}" "/tmp/release-v${FIRECRACKER_VERSION}-x86_64"

    ok "Firecracker v${FIRECRACKER_VERSION} installed to /usr/local/bin/firecracker"
}

# --- Guest kernel ---

install_kernel() {
    step "Checking guest kernel"

    if [[ -f "$KERNEL_PATH" ]]; then
        skip "Guest kernel already present at ${KERNEL_PATH}"
        return
    fi

    install "Fetching latest kernel-${KERNEL_SERIES}.x release from GitHub"
    mkdir -p "$(dirname "$KERNEL_PATH")"

    # Find the latest kernel release matching our series
    local tag
    tag=$(curl -fsSL "https://api.github.com/repos/${GITHUB_REPO}/releases" \
        | jq -r --arg s "kernel-${KERNEL_SERIES}." \
            '[.[] | select(.tag_name | startswith($s))] | sort_by(.created_at) | last | .tag_name // empty')

    if [[ -z "$tag" ]]; then
        fail "No kernel-${KERNEL_SERIES}.x release found in ${GITHUB_REPO}"
    fi

    local url="https://github.com/${GITHUB_REPO}/releases/download/${tag}/vmlinux.bin"
    install "Downloading ${tag} from GitHub Releases"
    curl -fsSL -L "$url" -o "$KERNEL_PATH"

    ok "Guest kernel (${tag}) installed to ${KERNEL_PATH}"
}

# --- dock-fire binaries ---

install_dock_fire() {
    step "Installing dock-fire binaries"

    # Check if dock-fire is already installed
    if command -v dock-fire &>/dev/null; then
        local current
        current=$(dock-fire --version 2>&1 || true)
        skip "dock-fire already installed: ${current}"
        skip "To upgrade, remove /usr/local/bin/dock-fire and re-run this script"
        return
    fi

    install "Querying latest dock-fire release from GitHub"
    local release_info
    release_info=$(curl -fsSL "https://api.github.com/repos/${GITHUB_REPO}/releases" \
        | jq -r '[.[] | select(.tag_name | startswith("v"))] | sort_by(.created_at) | last')

    if [[ -z "$release_info" || "$release_info" == "null" ]]; then
        fail "No dock-fire release found in ${GITHUB_REPO}"
    fi

    local tag
    tag=$(echo "$release_info" | jq -r '.tag_name')
    local version="${tag#v}"

    local asset_name="dock-fire_${version}_linux_amd64.tar.gz"
    local asset_url
    asset_url=$(echo "$release_info" | jq -r --arg name "$asset_name" \
        '.assets[] | select(.name == $name) | .browser_download_url // empty')

    if [[ -z "$asset_url" ]]; then
        fail "Asset ${asset_name} not found in release ${tag}"
    fi

    install "Downloading ${asset_name}"
    curl -fsSL -L "$asset_url" -o "/tmp/${asset_name}"

    install "Extracting binaries to /usr/local/bin/"
    tar -xzf "/tmp/${asset_name}" -C /usr/local/bin/ dock-fire dock-fire-init
    chmod +x /usr/local/bin/dock-fire /usr/local/bin/dock-fire-init
    rm -f "/tmp/${asset_name}"

    ok "dock-fire ${tag} installed to /usr/local/bin/"
}

# --- Configure Docker ---

configure_docker() {
    step "Configuring Docker runtime"

    local daemon_json="/etc/docker/daemon.json"
    local dock_fire_config='{"runtimes":{"dock-fire":{"path":"/usr/local/bin/dock-fire"}}}'

    if [[ -f "$daemon_json" ]]; then
        # Check if dock-fire is already configured
        if jq -e '.runtimes["dock-fire"]' "$daemon_json" &>/dev/null; then
            skip "dock-fire runtime already configured in ${daemon_json}"
            return
        fi

        # Merge into existing config
        install "Merging dock-fire runtime into existing ${daemon_json}"
        local merged
        merged=$(jq -s '.[0] * .[1]' "$daemon_json" <(echo "$dock_fire_config"))
        echo "$merged" | jq . > "$daemon_json"
    else
        install "Creating ${daemon_json}"
        echo "$dock_fire_config" | jq . > "$daemon_json"
    fi

    install "Restarting Docker daemon"
    systemctl restart docker

    ok "Docker configured with dock-fire runtime"
}

# --- Smoke test ---

smoke_test() {
    step "Running smoke test"

    install "docker run --runtime=dock-fire --net=none --rm alpine echo 'dock-fire is working'"
    local output
    output=$(docker run --runtime=dock-fire --net=none --rm alpine echo "dock-fire is working" 2>&1)

    if echo "$output" | grep -q "dock-fire is working"; then
        ok "Smoke test passed"
    else
        fail "Smoke test failed. Output:\n${output}"
    fi
}

# --- Main ---

main() {
    echo ""
    echo "  dock-fire installer"
    echo "  ==================="
    echo ""

    validate_environment
    install_system_packages
    install_firecracker
    install_kernel
    install_dock_fire
    configure_docker
    smoke_test

    echo ""
    echo -e "  ${GREEN}Installation complete!${NC}"
    echo ""
    echo "  Run containers with:"
    echo "    docker run --runtime=dock-fire --net=none --rm alpine echo hello"
    echo ""
}

main "$@"
