package logging

import (
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
)

const (
	defaultSocketAddr = "127.0.0.1:9000"
)

var server struct {
	wg     sync.WaitGroup
	ln     net.Listener
	conns  map[net.Conn]struct{}
	stopCh chan struct{}
	mu     sync.Mutex
	closed bool
	sock   string
}

func getSocketPath() string {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		cacheDir = os.TempDir()
	}
	sockDir := filepath.Join(cacheDir, "ctop")
	_ = os.MkdirAll(sockDir, 0o700)
	return filepath.Join(sockDir, "ctop.sock")
}

func getListener() (net.Listener, string) {
	var ln net.Listener
	var err error
	var sockFile string

	if debugModeTCP() {
		addr := os.Getenv("CTOP_DEBUG_ADDR")
		if addr == "" {
			addr = defaultSocketAddr
		}
		ln, err = net.Listen("tcp", addr)
	} else {
		sockFile = getSocketPath()
		_ = os.Remove(sockFile) // Clean up stale socket from previous crash
		ln, err = net.Listen("unix", sockFile)
		if err == nil {
			_ = os.Chmod(sockFile, 0o600)
		}
	}
	if err != nil {
		if Log != nil {
			Log.Errorf("failed to start log server: %s", err)
		}
		return nil, ""
	}
	return ln, sockFile
}

func StartServer() {
	server.mu.Lock()
	defer server.mu.Unlock()

	if server.ln != nil {
		return
	}

	ln, sock := getListener()
	if ln == nil {
		return
	}
	server.ln = ln
	server.sock = sock
	server.closed = false
	server.conns = make(map[net.Conn]struct{})
	server.stopCh = make(chan struct{})

	server.wg.Add(1)
	go func() {
		defer server.wg.Done()
		for {
			conn, err := ln.Accept()
			if err != nil {
				server.mu.Lock()
				isClosed := server.closed
				server.mu.Unlock()
				if isClosed {
					return
				}
				if ne, ok := err.(net.Error); ok && ne.Timeout() {
					continue
				}
				return
			}
			server.mu.Lock()
			if server.closed {
				server.mu.Unlock()
				_ = conn.Close()
				return
			}
			server.conns[conn] = struct{}{}
			server.mu.Unlock()

			go handler(conn)
		}
	}()

	if Log != nil {
		Log.Notice("logging server started")
	}
}

func StopServer() {
	server.mu.Lock()
	if server.ln == nil && server.stopCh == nil {
		server.mu.Unlock()
		return
	}
	server.closed = true
	ln := server.ln
	sock := server.sock
	server.ln = nil
	server.sock = ""
	if server.stopCh != nil {
		close(server.stopCh)
		server.stopCh = nil
	}
	for conn := range server.conns {
		_ = conn.Close()
	}
	server.conns = nil
	server.mu.Unlock()

	if ln != nil {
		if err := ln.Close(); err != nil && Log != nil {
			Log.Errorf("failed to close log server listener: %s", err)
		}
	}

	if sock != "" {
		_ = os.Remove(sock)
	}

	server.wg.Wait()
}

func handler(wc io.WriteCloser) {
	server.wg.Add(1)
	defer server.wg.Done()
	defer func() {
		if conn, ok := wc.(net.Conn); ok {
			server.mu.Lock()
			if server.conns != nil {
				delete(server.conns, conn)
			}
			server.mu.Unlock()
		}
		if err := wc.Close(); err != nil && Log != nil {
			Log.Errorf("failed to close log handler: %s", err)
		}
	}()
	if Log == nil {
		return
	}

	server.mu.Lock()
	serverStop := server.stopCh
	server.mu.Unlock()

	stopCh := make(chan struct{})
	defer close(stopCh)

	clientDone := make(chan struct{})
	if conn, ok := wc.(net.Conn); ok {
		go func() {
			buf := make([]byte, 1)
			for {
				_, err := conn.Read(buf)
				if err != nil {
					close(clientDone)
					return
				}
			}
		}()
	}

	tailStream := Log.tail(stopCh)
	for {
		select {
		case <-serverStop:
			_, _ = wc.Write([]byte("bye\n"))
			return
		case <-clientDone:
			return
		case msg, ok := <-tailStream:
			if !ok {
				_, _ = wc.Write([]byte("bye\n"))
				return
			}
			msg = fmt.Sprintf("%s\n", msg)
			if _, err := wc.Write([]byte(msg)); err != nil {
				return
			}
		}
	}
}
