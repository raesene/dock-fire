package runtime

import (
	"fmt"
	"strconv"
	"strings"
	"syscall"

	"github.com/rorym/dock-fire/internal/container"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

var KillCommand = &cli.Command{
	Name:  "kill",
	Usage: "send a signal to a container",
	ArgsUsage: `<container-id> [signal]

Where "<container-id>" is your name for the instance of the container.`,
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "all",
			Usage: "send signal to all processes (ignored, VM has single process tree)",
		},
	},
	Action: func(c *cli.Context) error {
		id := c.Args().First()
		if id == "" {
			return fmt.Errorf("container ID is required")
		}
		sigStr := c.Args().Get(1)
		if sigStr == "" {
			sigStr = "SIGTERM"
		}
		rootDir := c.String("root")

		logrus.Debugf("kill: id=%s signal=%s", id, sigStr)

		ctr, err := container.Load(rootDir, id)
		if err != nil {
			return err
		}

		// The VM runs from the create phase, so accept kill in both created and running states
		status := ctr.EffectiveStatus()
		if status != container.Running && status != container.Created {
			return fmt.Errorf("container %q is not running (status: %s)", id, status)
		}

		sig := parseSignal(sigStr)
		if sig == 0 {
			return fmt.Errorf("unknown signal: %s", sigStr)
		}

		if err := syscall.Kill(ctr.PID, sig); err != nil {
			return fmt.Errorf("kill VMM process %d: %w", ctr.PID, err)
		}

		logrus.Infof("sent signal %s to container %s (PID %d)", sigStr, id, ctr.PID)
		return nil
	},
}

func parseSignal(s string) syscall.Signal {
	s = strings.TrimPrefix(strings.ToUpper(s), "SIG")

	signals := map[string]syscall.Signal{
		"HUP":  syscall.SIGHUP,
		"INT":  syscall.SIGINT,
		"QUIT": syscall.SIGQUIT,
		"KILL": syscall.SIGKILL,
		"TERM": syscall.SIGTERM,
		"USR1": syscall.SIGUSR1,
		"USR2": syscall.SIGUSR2,
	}

	if sig, ok := signals[s]; ok {
		return sig
	}

	// Try parsing as a number
	if n, err := strconv.Atoi(s); err == nil {
		return syscall.Signal(n)
	}

	return 0
}
