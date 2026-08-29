//go:build !release

package collector

import (
	// nosemgrep: go.lang.security.audit.crypto.math_random.math-random-used - pseudo-random generator is intended for non-security mock metrics simulation
	"math/rand"
	"sync/atomic"
	"time"

	"github.com/edsilegx/ctop/pkg/models"
)

// Mock collector
type Mock struct {
	models.Metrics
	stream     chan models.Metrics
	done       atomic.Bool
	running    atomic.Bool
	aggression int64
}

func NewMock(a int64) *Mock {
	c := &Mock{
		Metrics:    models.Metrics{},
		aggression: a,
	}
	c.MemLimit = 2147483648
	return c
}

func (c *Mock) Running() bool {
	return c.running.Load()
}

func (c *Mock) Start() {
	c.done.Store(false)
	c.running.Store(true)
	c.stream = make(chan models.Metrics)
	go c.run()
}

func (c *Mock) Stop() {
	c.running.Store(false)
	c.done.Store(true)
}

func (c *Mock) Stream() chan models.Metrics {
	return c.stream
}

func (c *Mock) Logs() LogCollector {
	return &MockLogs{make(chan bool)}
}

func (c *Mock) run() {
	c.running.Store(true)
	// #nosec G404 - weak pseudo-random generator is sufficient for non-security mock metrics simulation
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	defer func() {
		c.running.Store(false)
		close(c.stream)
	}()

	// set to random static value, once
	c.Pids = r.Intn(12)
	c.IOBytesRead = r.Int63n(8098) * c.aggression
	c.IOBytesWrite = r.Int63n(8098) * c.aggression

	for {
		c.CPUUtil += r.Intn(2) * int(c.aggression)
		if c.CPUUtil >= 100 {
			c.CPUUtil = 0
		}

		c.NetTx += r.Int63n(60) * c.aggression
		c.NetRx += r.Int63n(60) * c.aggression
		c.MemUsage += r.Int63n(c.MemLimit/512) * c.aggression
		if c.MemUsage > c.MemLimit {
			c.MemUsage = 0
		}
		c.MemPercent = percent(float64(c.MemUsage), float64(c.MemLimit))
		c.stream <- c.Metrics
		if c.done.Load() {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
}
