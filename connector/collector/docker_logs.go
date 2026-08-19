package collector

import (
	"bufio"
	"context"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/edsilegx/ctop/models"
	api "github.com/fsouza/go-dockerclient"
)

const (
	maxLogLineSize = 1024 * 1024 // 1MB buffer for long log lines
)

type DockerLogs struct {
	id       string
	client   *api.Client
	cancel   context.CancelFunc
	stopOnce sync.Once
}

func NewDockerLogs(id string, client *api.Client) *DockerLogs {
	return &DockerLogs{
		id:     id,
		client: client,
	}
}

func (l *DockerLogs) Stream() chan models.Log {
	r, w := io.Pipe()
	logCh := make(chan models.Log, 100)
	ctx, cancel := context.WithCancel(context.Background())
	l.cancel = cancel

	opts := api.LogsOptions{
		Context:      ctx,
		Container:    l.id,
		OutputStream: w,
		ErrorStream:  w,
		Stdout:       true,
		Stderr:       true,
		Tail:         "100",
		Follow:       true,
		Timestamps:   true,
		RawTerminal:  false,
	}

	// read io pipe into channel
	go func() {
		defer close(logCh)
		defer func() { _ = r.Close() }()

		scanner := bufio.NewScanner(r)
		buf := make([]byte, 64*1024)
		scanner.Buffer(buf, maxLogLineSize)

		for scanner.Scan() {
			text := l.stripPfx(scanner.Text())
			parts := strings.SplitN(text, " ", 2)
			if len(parts) == 0 {
				continue
			}
			if len(parts) < 2 {
				logCh <- models.Log{Timestamp: l.parseTime(""), Message: parts[0]}
			} else {
				logCh <- models.Log{Timestamp: l.parseTime(parts[0]), Message: parts[1]}
			}
		}
		if err := scanner.Err(); err != nil && err != io.EOF && err != io.ErrClosedPipe {
			log.Debugf("scanner finished for %s: %s", l.id, err)
		}
	}()

	// connect to container log stream
	go func() {
		defer func() { _ = w.Close() }()
		err := l.client.Logs(opts)
		if err != nil && ctx.Err() == nil {
			log.Errorf("error reading container logs: %s", err)
		}
		log.Infof("log reader stopped for container: %s", l.id)
	}()

	log.Infof("log reader started for container: %s", l.id)
	return logCh
}

func (l *DockerLogs) Stop() {
	l.stopOnce.Do(func() {
		if l.cancel != nil {
			l.cancel()
		}
	})
}

func (l *DockerLogs) parseTime(s string) time.Time {
	ts, err := time.Parse(time.RFC3339Nano, s)
	if err == nil {
		return ts
	}

	ts, err2 := time.Parse(time.RFC3339Nano, l.stripPfx(s))
	if err2 == nil {
		return ts
	}

	return time.Now()
}

// attempt to strip message header prefix from a given raw docker log string
func (l *DockerLogs) stripPfx(s string) string {
	b := []byte(s)
	if len(b) > 8 && (b[0] == 1 || b[0] == 2) && b[1] == 0 && b[2] == 0 && b[3] == 0 {
		return string(b[8:])
	}
	return s
}
