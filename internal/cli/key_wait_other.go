//go:build !darwin && !linux

package cli

import "os"

// The line-mode scanner remains the compatibility fallback on platforms where
// this package does not provide terminal-specific hidden input.
func terminalSecretReader(_ *os.File) func() ([]byte, error) {
	return nil
}

// j/k remains available on platforms without the poll helper. A standalone
// Escape cancels immediately; arrow escape sequences are not consumed here.
func readEscapeTail(_ *os.File) []byte {
	return nil
}
