//go:build !windows

package manager

import (
	"io"
	"math"
	"os"
	"sync/atomic"

	"golang.org/x/sys/unix"
)

// cancellableStdin wraps terminal input with a self-pipe so that when Close() is called,
// any pending Read() on the terminal unblocks immediately with io.EOF instead of hanging
// and consuming the user's subsequent keystroke after Docker interactive exec terminates.
type cancellableStdin struct {
	fd      int32
	pipeRFd int32
	pipeR   *os.File
	pipeW   *os.File
	closed  uint32
}

func newCancellableStdin(f *os.File) io.ReadCloser {
	rawFd := f.Fd()
	if rawFd > math.MaxInt32 {
		return &noClosableReader{f}
	}
	pr, pw, err := os.Pipe()
	if err != nil {
		return &noClosableReader{f}
	}
	if pr.Fd() > math.MaxInt32 || pw.Fd() > math.MaxInt32 {
		_ = pr.Close()
		_ = pw.Close()
		return &noClosableReader{f}
	}
	return &cancellableStdin{
		fd:      int32(rawFd),   // #nosec G115 -- rawFd bounded by math.MaxInt32
		pipeRFd: int32(pr.Fd()), // #nosec G115 -- pr.Fd() bounded by math.MaxInt32
		pipeR:   pr,
		pipeW:   pw,
	}
}

func (c *cancellableStdin) Read(p []byte) (int, error) {
	if atomic.LoadUint32(&c.closed) == 1 {
		return 0, io.EOF
	}

	fds := []unix.PollFd{
		{Fd: c.fd, Events: unix.POLLIN},
		{Fd: c.pipeRFd, Events: unix.POLLIN | unix.POLLHUP},
	}

	for {
		n, err := unix.Poll(fds, -1)
		if err != nil {
			if err == unix.EINTR {
				continue
			}
			if atomic.LoadUint32(&c.closed) == 1 {
				return 0, io.EOF
			}
			return 0, err
		}
		if n <= 0 {
			continue
		}
		break
	}

	if atomic.LoadUint32(&c.closed) == 1 || fds[1].Revents != 0 {
		return 0, io.EOF
	}

	if fds[0].Revents&unix.POLLIN != 0 {
		n, err := unix.Read(int(c.fd), p)
		if n == 0 && err == nil {
			return 0, io.EOF
		}
		return n, err
	}

	return 0, io.EOF
}

func (c *cancellableStdin) Close() error {
	if atomic.CompareAndSwapUint32(&c.closed, 0, 1) {
		_, _ = c.pipeW.Write([]byte{1})
		_ = c.pipeW.Close()
		_ = c.pipeR.Close()
	}
	return nil
}

// flushTerminalInput discards any pending or unread bytes from the host terminal input buffer
// so that mode transition sequences (like keypad exit or screen clear) never leak into the container PTY.
func flushTerminalInput(fd uintptr) {
	if fd > math.MaxInt32 {
		return
	}
	fd32 := int32(fd) // #nosec G115 -- fd bounded by math.MaxInt32
	_ = unix.IoctlSetInt(int(fd32), unix.TCFLSH, unix.TCIFLUSH)
	fds := []unix.PollFd{{Fd: fd32, Events: unix.POLLIN}}
	for {
		n, err := unix.Poll(fds, 0)
		if err != nil || n <= 0 || fds[0].Revents&unix.POLLIN == 0 {
			break
		}
		var buf [256]byte
		_, _ = unix.Read(int(fd32), buf[:])
	}
}
