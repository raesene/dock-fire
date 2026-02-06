package runtime

import (
	"fmt"

	"github.com/rorym/dock-fire/internal/container"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

var StartCommand = &cli.Command{
	Name:  "start",
	Usage: "start a created container",
	ArgsUsage: `<container-id>

Where "<container-id>" is your name for the instance of the container.`,
	Action: func(c *cli.Context) error {
		id := c.Args().First()
		if id == "" {
			return fmt.Errorf("container ID is required")
		}
		rootDir := c.String("root")

		logrus.Debugf("start: id=%s", id)

		ctr, err := container.Load(rootDir, id)
		if err != nil {
			return err
		}

		if ctr.Status != container.Created {
			return fmt.Errorf("container %q is not in created state (status: %s)", id, ctr.Status)
		}

		// The VM was already booted during create.
		// Just transition the state to running.
		if err := ctr.Transition(container.Running); err != nil {
			return fmt.Errorf("transition to running: %w", err)
		}
		if err := ctr.Save(); err != nil {
			return fmt.Errorf("save state: %w", err)
		}

		logrus.Infof("container %s started (VMM PID: %d)", id, ctr.PID)
		return nil
	},
}
