package network

import (
	"fmt"
	"net"
	"os/exec"
	"strings"

	"github.com/rorym/dock-fire/internal/container"
	"github.com/sirupsen/logrus"
)

// Subnet represents a /30 subnet allocation for a container.
type Subnet struct {
	HostIP   string // e.g. 10.0.0.1 (assigned to TAP device)
	GuestIP  string // e.g. 10.0.0.2 (assigned to guest eth0 via boot args)
	CIDR     string // e.g. 10.0.0.0/30
}

// AllocateSubnet finds the next free /30 subnet from 10.0.0.0/16.
// It scans existing containers to avoid collisions.
func AllocateSubnet(rootDir string) (*Subnet, error) {
	// Collect used subnets from both container state and live TAP devices.
	// Stale TAPs from crashed containers won't appear in state files,
	// so we also scan the host's network interfaces.
	used := make(map[string]bool)
	ids, err := container.List(rootDir)
	if err != nil {
		return nil, fmt.Errorf("list containers: %w", err)
	}

	for _, id := range ids {
		ctr, err := container.Load(rootDir, id)
		if err != nil {
			continue
		}
		if ctr.SubnetCIDR != "" {
			used[ctr.SubnetCIDR] = true
		}
	}

	// Also check IPs assigned to existing df-* TAP devices.
	for _, cidr := range usedTAPSubnets() {
		used[cidr] = true
	}

	// Iterate /30 subnets within 10.0.0.0/16
	// Each /30 gives us 4 IPs: network, host, guest, broadcast
	// Start from 10.0.0.0/30, step by 4
	base := net.ParseIP("10.0.0.0").To4()
	for i := 0; i < 16384; i++ {
		offset := uint32(i * 4)
		networkIP := make(net.IP, 4)
		copy(networkIP, base)
		networkIP[2] = byte(offset >> 8)
		networkIP[3] = byte(offset & 0xff)

		cidr := fmt.Sprintf("%s/30", networkIP.String())
		if used[cidr] {
			continue
		}

		hostIP := make(net.IP, 4)
		copy(hostIP, networkIP)
		hostIP[3] += 1

		guestIP := make(net.IP, 4)
		copy(guestIP, networkIP)
		guestIP[3] += 2

		logrus.Debugf("allocated subnet %s (host=%s, guest=%s)", cidr, hostIP, guestIP)
		return &Subnet{
			HostIP:  hostIP.String(),
			GuestIP: guestIP.String(),
			CIDR:    cidr,
		}, nil
	}

	return nil, fmt.Errorf("no free /30 subnets available in 10.0.0.0/16")
}

// usedTAPSubnets returns the /30 CIDRs assigned to existing df-* TAP devices.
func usedTAPSubnets() []string {
	// "ip -o addr show" gives one-line-per-address output like:
	// 131: df-3bf20a12    inet 10.0.0.1/30 scope global df-3bf20a12\...
	out, err := exec.Command("ip", "-o", "addr", "show").CombinedOutput()
	if err != nil {
		return nil
	}

	var cidrs []string
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		devName := fields[1]
		if !strings.HasPrefix(devName, "df-") {
			continue
		}
		for i, f := range fields {
			if f == "inet" && i+1 < len(fields) {
				// fields[i+1] is like "10.0.0.1/30"
				ip, ipNet, err := net.ParseCIDR(fields[i+1])
				if err != nil {
					continue
				}
				_ = ip
				cidrs = append(cidrs, ipNet.String())
			}
		}
	}
	return cidrs
}
