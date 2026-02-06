package runtime

import (
	"fmt"
	"os"
	"github.com/rorym/dock-fire/internal/container"
	"github.com/rorym/dock-fire/internal/network"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

var DeleteCommand = &cli.Command{
	Name:  "delete",
	Usage: "delete a container",
	ArgsUsage: `<container-id>

Where "<container-id>" is your name for the instance of the container.`,
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "force",
			Usage: "force delete the container",
		},
	},
	Action: func(c *cli.Context) error {
		id := c.Args().First()
		if id == "" {
			return fmt.Errorf("container ID is required")
		}
		force := c.Bool("force")
		rootDir := c.String("root")

		logrus.Debugf("delete: id=%s force=%v", id, force)

		ctr, err := container.Load(rootDir, id)
		if err != nil {
			if force {
				// Force delete: just remove the state dir even if state can't be loaded
				return container.Delete(rootDir, id)
			}
			return err
		}

		// If the VMM is still alive, either force-kill or error.
		// The VM runs from the create phase, so check in both created and running states.
		if ctr.IsVMMAlive() {
			if !force {
				return fmt.Errorf("container %q has a running VM, use --force to delete", id)
			}
			if err := stopVM(ctr); err != nil {
				logrus.Warnf("failed to stop VMM: %v", err)
			}
		}

		// Clean up networking
		if err := network.Teardown(ctr); err != nil {
			logrus.Warnf("failed to tear down networking: %v", err)
		}

		// Clean up socket file
		if ctr.SocketPath != "" {
			os.Remove(ctr.SocketPath)
		}

		// Remove state directory and all artifacts
		if err := container.Delete(rootDir, id); err != nil {
			return fmt.Errorf("delete state: %w", err)
		}

		logrus.Infof("container %s deleted", id)
		return nil
	},
}
