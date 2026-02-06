package network

import (
	"fmt"

	"github.com/rorym/dock-fire/internal/container"
	"github.com/sirupsen/logrus"
)

// TAPName returns the TAP device name for a container.
func TAPName(id string) string {
	name := id
	if len(name) > 8 {
		name = name[:8]
	}
	return "df-" + name
}

// Setup configures networking for a container (TAP device, IP allocation, NAT).
func Setup(ctr *container.Container) error {
	// Allocate a subnet
	subnet, err := AllocateSubnet(ctr.RootDir)
	if err != nil {
		return fmt.Errorf("allocate subnet: %w", err)
	}

	tapName := TAPName(ctr.ID)

	// Create TAP device
	if err := CreateTAP(tapName, subnet.HostIP); err != nil {
		return fmt.Errorf("create TAP: %w", err)
	}

	// Set up NAT
	if err := SetupNAT(tapName, subnet.CIDR); err != nil {
		// Clean up TAP on failure
		DeleteTAP(tapName)
		return fmt.Errorf("setup NAT: %w", err)
	}

	// Store networking info in container state
	ctr.TapDevice = tapName
	ctr.GuestIP = subnet.GuestIP
	ctr.HostIP = subnet.HostIP
	ctr.SubnetCIDR = subnet.CIDR

	logrus.Debugf("networking configured: tap=%s host=%s guest=%s", tapName, subnet.HostIP, subnet.GuestIP)
	return nil
}

// Teardown removes networking resources for a container.
func Teardown(ctr *container.Container) error {
	if ctr.TapDevice == "" {
		return nil
	}

	// Remove NAT rules
	if ctr.SubnetCIDR != "" {
		TeardownNAT(ctr.TapDevice, ctr.SubnetCIDR)
	}

	// Delete TAP device
	if err := DeleteTAP(ctr.TapDevice); err != nil {
		logrus.Debugf("TAP cleanup: %v", err)
	}

	return nil
}
