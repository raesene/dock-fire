package main

import (
	"fmt"
	"os"

	"github.com/rorym/dock-fire/internal/runtime"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

func main() {
	app := &cli.App{
		Name:  "dock-fire",
		Usage: "OCI runtime that boots containers inside Firecracker microVMs",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "root",
				Value: "/run/dock-fire",
				Usage: "root directory for container state",
			},
			&cli.StringFlag{
				Name:  "log",
				Usage: "log file path (default: stderr)",
			},
			&cli.StringFlag{
				Name:  "log-format",
				Value: "text",
				Usage: "log format (text or json)",
			},
			&cli.BoolFlag{
				Name:  "debug",
				Usage: "enable debug logging",
			},
			// systemd-cgroup is expected by containerd but we don't use it
			&cli.BoolFlag{
				Name:   "systemd-cgroup",
				Hidden: true,
			},
		},
		Before: func(c *cli.Context) error {
			if c.Bool("debug") {
				logrus.SetLevel(logrus.DebugLevel)
			} else {
				logrus.SetLevel(logrus.WarnLevel)
			}
			if c.String("log-format") == "json" {
				logrus.SetFormatter(&logrus.JSONFormatter{})
			}
			if logFile := c.String("log"); logFile != "" {
				f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
				if err != nil {
					return fmt.Errorf("open log file: %w", err)
				}
				logrus.SetOutput(f)
			}
			return nil
		},
		Commands: []*cli.Command{
			runtime.CreateCommand,
			runtime.StartCommand,
			runtime.StateCommand,
			runtime.KillCommand,
			runtime.DeleteCommand,
		},
	}

	if err := app.Run(os.Args); err != nil {
		logrus.Fatal(err)
	}
}
