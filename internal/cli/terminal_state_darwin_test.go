//go:build darwin

package cli

import "golang.org/x/sys/unix"

func terminalStateForTest(fd int) (any, error) {
	return unix.IoctlGetTermios(fd, unix.TIOCGETA)
}

func terminalStatesEqualForTest(before any, after any) bool {
	left := *before.(*unix.Termios)
	right := *after.(*unix.Termios)
	// PENDIN is a kernel-maintained indication that queued input should be
	// reprocessed, not a configured terminal mode. PTY activity may toggle it
	// even when every user-configurable setting was restored exactly.
	left.Lflag &^= unix.PENDIN
	right.Lflag &^= unix.PENDIN
	return left == right
}
