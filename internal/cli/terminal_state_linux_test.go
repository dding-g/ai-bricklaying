//go:build linux

package cli

import "golang.org/x/sys/unix"

func terminalStateForTest(fd int) (any, error) {
	return unix.IoctlGetTermios(fd, unix.TCGETS)
}

func terminalStatesEqualForTest(before any, after any) bool {
	return *before.(*unix.Termios) == *after.(*unix.Termios)
}
