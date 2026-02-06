package vm

import (
	"fmt"
	"net"
	"os"

	"golang.org/x/sys/unix"
)

// openPTY opens a new pseudoterminal pair, returning the master and slave files.
func openPTY() (master *os.File, slave *os.File, err error) {
	master, err = os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("open /dev/ptmx: %w", err)
	}

	defer func() {
		if err != nil {
			master.Close()
		}
	}()

	// Get the slave PTY number
	ptsNum, err := unix.IoctlGetInt(int(master.Fd()), unix.TIOCGPTN)
	if err != nil {
		return nil, nil, fmt.Errorf("TIOCGPTN: %w", err)
	}

	// Unlock the slave PTY (TIOCSPTLCK takes a pointer to int)
	if err := unix.IoctlSetPointerInt(int(master.Fd()), unix.TIOCSPTLCK, 0); err != nil {
		return nil, nil, fmt.Errorf("TIOCSPTLCK: %w", err)
	}

	slavePath := fmt.Sprintf("/dev/pts/%d", ptsNum)
	slave, err = os.OpenFile(slavePath, os.O_RDWR, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("open %s: %w", slavePath, err)
	}

	return master, slave, nil
}

// sendConsoleFd sends the master PTY file descriptor over a Unix socket
// (SCM_RIGHTS) to the containerd shim, which uses it for terminal I/O.
func sendConsoleFd(consoleSocket string, master *os.File) error {
	conn, err := net.Dial("unix", consoleSocket)
	if err != nil {
		return fmt.Errorf("dial console socket: %w", err)
	}
	defer conn.Close()

	uc, ok := conn.(*net.UnixConn)
	if !ok {
		return fmt.Errorf("not a unix connection")
	}

	f, err := uc.File()
	if err != nil {
		return fmt.Errorf("get socket fd: %w", err)
	}
	defer f.Close()

	rights := unix.UnixRights(int(master.Fd()))
	if err := unix.Sendmsg(int(f.Fd()), nil, rights, nil, 0); err != nil {
		return fmt.Errorf("sendmsg SCM_RIGHTS: %w", err)
	}

	return nil
}
