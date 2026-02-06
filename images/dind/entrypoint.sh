#!/bin/sh
set -e

# Mount cgroup2 (or fall back to cgroup v1)
if [ ! -d /sys/fs/cgroup/cgroup.controllers ] && [ ! -d /sys/fs/cgroup/cpu ]; then
    mkdir -p /sys/fs/cgroup
    if mount -t cgroup2 cgroup2 /sys/fs/cgroup 2>/dev/null; then
        : # cgroup2 mounted
    else
        # Fall back to cgroup v1 hierarchies
        mount -t tmpfs cgroup /sys/fs/cgroup
        for subsys in cpu cpuacct cpuset memory blkio devices freezer pids net_cls net_prio; do
            mkdir -p /sys/fs/cgroup/$subsys
            mount -t cgroup -o $subsys cgroup /sys/fs/cgroup/$subsys 2>/dev/null || true
        done
    fi
fi

# Mount devpts for PTY allocation
if ! mountpoint -q /dev/pts 2>/dev/null; then
    mkdir -p /dev/pts
    mount -t devpts devpts /dev/pts -o newinstance,ptmxmode=0666
fi

# Mount shared memory
if ! mountpoint -q /dev/shm 2>/dev/null; then
    mkdir -p /dev/shm
    mount -t tmpfs shm /dev/shm
fi

# Mount /run for daemon sockets
if ! mountpoint -q /run 2>/dev/null; then
    mount -t tmpfs run /run
fi

# Start dockerd in background
dockerd --log-level=warn > /var/log/dockerd.log 2>&1 &

# Wait for docker socket (up to 30s)
timeout=30
while [ $timeout -gt 0 ]; do
    if docker info >/dev/null 2>&1; then
        break
    fi
    sleep 1
    timeout=$((timeout - 1))
done

if [ $timeout -eq 0 ]; then
    echo "ERROR: dockerd failed to start within 30s" >&2
    cat /var/log/dockerd.log >&2
    exit 1
fi

exec "$@"
