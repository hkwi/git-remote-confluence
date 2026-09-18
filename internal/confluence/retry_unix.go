//go:build !windows

package confluence

import "syscall"

const (
	errConnectionRefused = syscall.ECONNREFUSED
	errConnectionReset   = syscall.ECONNRESET
	errConnectionAborted = syscall.ECONNABORTED
	errBrokenPipe        = syscall.EPIPE
)
