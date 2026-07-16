//go:build darwin || linux

package cli

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

const maxHiddenInputBytes = 16 * 1024

// terminalSecretReady is a test seam used only to coordinate an actual PTY
// after raw, no-echo mode is active. Production leaves it nil.
var terminalSecretReady func()

func terminalSecretReader(file *os.File) func() ([]byte, error) {
	return func() ([]byte, error) {
		return readRawTerminalSecret(file)
	}
}

func readRawTerminalSecret(file *os.File) (secret []byte, err error) {
	fd := int(file.Fd())
	state, err := term.MakeRaw(fd)
	if err != nil {
		return nil, err
	}
	defer func() {
		if restoreErr := term.Restore(fd, state); restoreErr != nil {
			secret = nil
			err = fmt.Errorf("restore terminal after hidden input: %w", restoreErr)
		}
	}()

	if terminalSecretReady != nil {
		terminalSecretReady()
	}

	secret = make([]byte, 0, 256)
	buffer := []byte{0}
	for {
		count, readErr := file.Read(buffer)
		if readErr != nil {
			return nil, readErr
		}
		if count == 0 {
			continue
		}

		switch current := buffer[0]; current {
		case '\r', '\n':
			return secret, nil
		case 0x03, 0x1b:
			return nil, errSecretInputCancelled
		case 'q', 'Q':
			if len(secret) == 0 {
				return nil, errSecretInputCancelled
			}
			secret = append(secret, current)
		case 0x08, 0x7f:
			if len(secret) > 0 {
				secret = secret[:len(secret)-1]
			}
		case 0x15:
			secret = secret[:0]
		default:
			if len(secret) >= maxHiddenInputBytes {
				return nil, fmt.Errorf("hidden terminal input exceeds %d bytes", maxHiddenInputBytes)
			}
			secret = append(secret, current)
		}
	}
}

func readEscapeTail(file *os.File) []byte {
	poll := []unix.PollFd{{Fd: int32(file.Fd()), Events: unix.POLLIN}}
	ready, err := unix.Poll(poll, 30)
	if err != nil || ready == 0 || poll[0].Revents&(unix.POLLIN|unix.POLLHUP) == 0 {
		return nil
	}
	buffer := make([]byte, 2)
	count, _ := file.Read(buffer)
	return buffer[:count]
}
