package runtime

import (
	"github.com/rorym/dock-fire/internal/container"
	"github.com/rorym/dock-fire/internal/network"
	"github.com/rorym/dock-fire/internal/rootfs"
	"github.com/rorym/dock-fire/internal/vm"

	specs "github.com/opencontainers/runtime-spec/specs-go"
)

func createRootfsImage(rootDir, id, rootfsPath string, spec *specs.Spec) (string, error) {
	return rootfs.CreateImage(rootDir, id, rootfsPath, spec)
}

func setupNetworking(ctr *container.Container) error {
	return network.Setup(ctr)
}

func startVM(ctr *container.Container, spec *specs.Spec, consoleSocket string) error {
	return vm.Start(ctr, spec, consoleSocket)
}

func stopVM(ctr *container.Container) error {
	return vm.Stop(ctr)
}
