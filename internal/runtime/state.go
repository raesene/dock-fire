package runtime

import (
	"fmt"

	"github.com/rorym/dock-fire/internal/container"
	"github.com/rorym/dock-fire/internal/oci"
	"github.com/urfave/cli/v2"
)

var StateCommand = &cli.Command{
	Name:  "state",
	Usage: "output the state of a container",
	ArgsUsage: `<container-id>

Where "<container-id>" is your name for the instance of the container.`,
	Action: func(c *cli.Context) error {
		id := c.Args().First()
		if id == "" {
			return fmt.Errorf("container ID is required")
		}
		rootDir := c.String("root")

		ctr, err := container.Load(rootDir, id)
		if err != nil {
			return err
		}

		data, err := oci.MarshalState(ctr)
		if err != nil {
			return err
		}

		fmt.Println(string(data))
		return nil
	},
}
