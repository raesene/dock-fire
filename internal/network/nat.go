package network

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/sirupsen/logrus"
)

// SetupNAT configures iptables rules for NAT and forwarding.
func SetupNAT(tapName, subnetCIDR string) error {
	outIface, err := detectDefaultInterface()
	if err != nil {
		return fmt.Errorf("detect default interface: %w", err)
	}
	logrus.Debugf("using %s as default outbound interface", outIface)

	rules := [][]string{
		// Enable IP forwarding via sysctl
		{"sysctl", "-w", "net.ipv4.ip_forward=1"},
		// MASQUERADE traffic from the VM subnet
		{"iptables", "-t", "nat", "-A", "POSTROUTING", "-s", subnetCIDR, "-o", outIface, "-j", "MASQUERADE"},
		// Allow forwarded traffic from the TAP
		{"iptables", "-A", "FORWARD", "-i", tapName, "-o", outIface, "-j", "ACCEPT"},
		// Allow return traffic
		{"iptables", "-A", "FORWARD", "-i", outIface, "-o", tapName, "-m", "state", "--state", "RELATED,ESTABLISHED", "-j", "ACCEPT"},
	}

	for _, args := range rules {
		if out, err := exec.Command(args[0], args[1:]...).CombinedOutput(); err != nil {
			return fmt.Errorf("%v: %w: %s", args, err, out)
		}
	}

	return nil
}

// TeardownNAT removes the iptables rules for a container.
func TeardownNAT(tapName, subnetCIDR string) error {
	outIface, err := detectDefaultInterface()
	if err != nil {
		logrus.Debugf("could not detect default interface for NAT teardown: %v", err)
		return nil
	}

	// Remove rules (best-effort, ignore errors)
	rules := [][]string{
		{"iptables", "-t", "nat", "-D", "POSTROUTING", "-s", subnetCIDR, "-o", outIface, "-j", "MASQUERADE"},
		{"iptables", "-D", "FORWARD", "-i", tapName, "-o", outIface, "-j", "ACCEPT"},
		{"iptables", "-D", "FORWARD", "-i", outIface, "-o", tapName, "-m", "state", "--state", "RELATED,ESTABLISHED", "-j", "ACCEPT"},
	}

	for _, args := range rules {
		if out, err := exec.Command(args[0], args[1:]...).CombinedOutput(); err != nil {
			logrus.Debugf("iptables cleanup %v: %v: %s", args, err, out)
		}
	}

	return nil
}

func detectDefaultInterface() (string, error) {
	out, err := exec.Command("ip", "route", "show", "default").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("ip route: %w: %s", err, out)
	}

	// Parse: "default via X.X.X.X dev <iface> ..."
	fields := strings.Fields(string(out))
	for i, f := range fields {
		if f == "dev" && i+1 < len(fields) {
			return fields[i+1], nil
		}
	}

	return "", fmt.Errorf("no default route found")
}
