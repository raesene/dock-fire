package runtime

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/rorym/dock-fire/internal/container"
	"github.com/rorym/dock-fire/internal/oci"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

var CreateCommand = &cli.Command{
	Name:  "create",
	Usage: "create a container",
	ArgsUsage: `<container-id>

Where "<container-id>" is your name for the instance of the container.`,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "bundle",
			Value: ".",
			Usage: "path to the root of the OCI bundle",
		},
		&cli.StringFlag{
			Name:  "console-socket",
			Usage: "path to AF_UNIX socket for terminal I/O",
		},
		&cli.StringFlag{
			Name:  "pid-file",
			Usage: "file to write the process ID to",
		},
		// no-pivot is expected by containerd but we don't use it
		&cli.BoolFlag{
			Name:   "no-pivot",
			Hidden: true,
		},
	},
	Action: func(c *cli.Context) error {
		id := c.Args().First()
		if id == "" {
			return fmt.Errorf("container ID is required")
		}
		bundle := c.String("bundle")
		rootDir := c.String("root")
		pidFile := c.String("pid-file")

		// Make bundle path absolute
		if !filepath.IsAbs(bundle) {
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("getwd: %w", err)
			}
			bundle = filepath.Join(cwd, bundle)
		}

		logrus.Debugf("create: id=%s bundle=%s root=%s", id, bundle, rootDir)

		if container.Exists(rootDir, id) {
			return fmt.Errorf("container %q already exists", id)
		}

		// Parse OCI config
		spec, err := oci.LoadConfig(bundle)
		if err != nil {
			return fmt.Errorf("load OCI config: %w", err)
		}
		logrus.Debugf("parsed OCI spec, process args: %v", spec.Process.Args)

		// Create container in "creating" state
		ctr := &container.Container{
			ID:      id,
			Bundle:  bundle,
			Status:  container.Creating,
			RootDir: rootDir,
		}

		// Build ext4 rootfs image
		rootfsPath := filepath.Join(bundle, "rootfs")
		if spec.Root != nil && spec.Root.Path != "" {
			rp := spec.Root.Path
			if filepath.IsAbs(rp) {
				rootfsPath = rp
			} else {
				rootfsPath = filepath.Join(bundle, rp)
			}
		}

		imagePath, err := createRootfsImage(rootDir, id, rootfsPath, spec)
		if err != nil {
			return fmt.Errorf("create rootfs image: %w", err)
		}
		ctr.ImagePath = imagePath

		// Set up networking
		if err := setupNetworking(ctr); err != nil {
			return fmt.Errorf("setup networking: %w", err)
		}

		// Boot the VM now so we have a valid PID for containerd.
		// The guest init will run the user command immediately.
		if err := startVM(ctr, spec); err != nil {
			return fmt.Errorf("start VM: %w", err)
		}

		// Transition directly to running since the VM is started
		if err := ctr.Transition(container.Created); err != nil {
			return fmt.Errorf("transition to created: %w", err)
		}
		if err := ctr.Save(); err != nil {
			return fmt.Errorf("save state: %w", err)
		}

		// Write PID file with the VMM process PID
		if pidFile != "" {
			pid := fmt.Sprintf("%d", ctr.PID)
			if err := os.WriteFile(pidFile, []byte(pid), 0o644); err != nil {
				return fmt.Errorf("write pid file: %w", err)
			}
		}

		logrus.Infof("container %s created (VMM PID: %d)", id, ctr.PID)
		return nil
	},
}
