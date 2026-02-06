package network

import (
	"fmt"
	"os/exec"

	"github.com/sirupsen/logrus"
)

// CreateTAP creates a TAP device and assigns an IP address to it.
func CreateTAP(name, hostIP string) error {
	logrus.Debugf("creating TAP device %s with IP %s", name, hostIP)

	cmds := [][]string{
		{"ip", "tuntap", "add", "dev", name, "mode", "tap"},
		{"ip", "addr", "add", hostIP + "/30", "dev", name},
		{"ip", "link", "set", name, "up"},
	}

	for _, args := range cmds {
		if out, err := exec.Command(args[0], args[1:]...).CombinedOutput(); err != nil {
			return fmt.Errorf("%v: %w: %s", args, err, out)
		}
	}

	return nil
}

// DeleteTAP removes a TAP device.
func DeleteTAP(name string) error {
	logrus.Debugf("deleting TAP device %s", name)
	out, err := exec.Command("ip", "link", "del", name).CombinedOutput()
	if err != nil {
		return fmt.Errorf("delete TAP %s: %w: %s", name, err, out)
	}
	return nil
}
