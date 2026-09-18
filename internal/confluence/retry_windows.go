package confluence

import "syscall"

// Winsock reports native WSA error codes rather than the POSIX compatibility
// constants in syscall on Windows.
const (
	// WSAECONNREFUSED is not exported by Go's syscall package.
	errConnectionRefused = syscall.Errno(10061)
	errConnectionReset   = syscall.WSAECONNRESET
	errConnectionAborted = syscall.WSAECONNABORTED
	errBrokenPipe        = syscall.ERROR_BROKEN_PIPE
)
