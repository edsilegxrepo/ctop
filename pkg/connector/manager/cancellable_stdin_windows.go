//go:build windows

package manager

import (
	"io"
	"os"
	"sync/atomic"

	"golang.org/x/sys/windows"
)

type cancellableStdin struct {
	io.Reader
	closed uint32
}

func newCancellableStdin(f *os.File) io.ReadCloser {
	return &cancellableStdin{Reader: f}
}

func (c *cancellableStdin) Read(p []byte) (int, error) {
	if atomic.LoadUint32(&c.closed) == 1 {
		return 0, io.EOF
	}
	return c.Reader.Read(p)
}

func (c *cancellableStdin) Close() error {
	atomic.StoreUint32(&c.closed, 1)
	return nil
}

// flushTerminalInput discards any pending or unread console input records on Windows.
func flushTerminalInput(fd uintptr) {
	_ = windows.FlushConsoleInputBuffer(windows.Handle(fd))
}
