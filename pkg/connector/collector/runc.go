//go:build linux

package collector

import (
	"math"
	"sync/atomic"
	"time"

	"github.com/opencontainers/cgroups"
	"github.com/opencontainers/runc/libcontainer"
	"github.com/opencontainers/runc/types"

	"github.com/edsilegx/ctop/pkg/models"
)

// Runc collector
type Runc struct {
	models.Metrics
	id         string
	libc       *libcontainer.Container
	stream     chan models.Metrics
	done       atomic.Bool
	running    atomic.Bool
	stopCh     chan struct{}
	interval   time.Duration
	lastCpu    float64
	lastSysCpu float64
}

func NewRunc(libc *libcontainer.Container) *Runc {
	id := ""
	if libc != nil {
		id = libc.ID()
	}
	c := &Runc{
		Metrics:  models.NewMetrics(),
		id:       id,
		libc:     libc,
		interval: 1 * time.Second,
		stopCh:   make(chan struct{}),
	}
	return c
}

func (c *Runc) Running() bool {
	return c.running.Load()
}

func (c *Runc) Start() {
	c.done.Store(false)
	c.stopCh = make(chan struct{})
	c.stream = make(chan models.Metrics, 16)
	go c.run()
}

func (c *Runc) Stop() {
	if c.running.Load() && !c.done.Load() {
		c.done.Store(true)
		c.running.Store(false)
		close(c.stopCh)
	}
}

func (c *Runc) Stream() chan models.Metrics {
	return c.stream
}

func (c *Runc) Logs() LogCollector {
	return nil
}

func (c *Runc) run() {
	c.running.Store(true)
	defer close(c.stream)
	log.Debugf("collector started for container: %s", c.id)

	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		if c.libc == nil {
			log.Errorf("failed to collect stats for container %s: nil libc", c.id)
			break
		}
		stats, err := c.libc.Stats()
		if err != nil {
			log.Errorf("failed to collect stats for container %s:\n%s", c.id, err)
			break
		}

		c.ReadCPU(stats.CgroupStats)
		c.ReadMem(stats.CgroupStats)
		c.ReadNet(stats.Interfaces)
		c.ReadIO(stats.CgroupStats)

		select {
		case c.stream <- c.Metrics:
		default:
		}
		if c.done.Load() {
			break
		}

		select {
		case <-c.stopCh:
			c.running.Store(false)
			return
		case <-ticker.C:
		}
	}

	c.running.Store(false)
}

func (c *Runc) ReadCPU(stats *cgroups.Stats) {
	u := stats.CpuStats.CpuUsage
	ncpus := len(u.PercpuUsage)
	total := float64(u.TotalUsage)
	system := float64(getSysCPUUsage())

	cpudiff := total - c.lastCpu
	syscpudiff := system - c.lastSysCpu

	c.NCpus = ncpus
	c.CPUUtil = percent(cpudiff, syscpudiff)
	c.lastCpu = total
	c.lastSysCpu = system
	if stats.PidsStats.Current > math.MaxInt {
		c.Pids = math.MaxInt
	} else {
		c.Pids = int(stats.PidsStats.Current)
	}
}

func (c *Runc) ReadMem(stats *cgroups.Stats) {
	if stats.MemoryStats.Usage.Usage > math.MaxInt64 {
		c.MemUsage = math.MaxInt64
	} else {
		c.MemUsage = int64(stats.MemoryStats.Usage.Usage)
	}
	if stats.MemoryStats.Usage.Limit > math.MaxInt64 {
		c.MemLimit = math.MaxInt64
	} else {
		c.MemLimit = int64(stats.MemoryStats.Usage.Limit)
	}
	if c.MemLimit > sysMemTotal && sysMemTotal > 0 {
		c.MemLimit = sysMemTotal
	}
	c.MemPercent = percent(float64(c.MemUsage), float64(c.MemLimit))
}

func (c *Runc) ReadNet(interfaces []*types.NetworkInterface) {
	var rx, tx int64
	for _, network := range interfaces {
		if network.RxBytes > math.MaxInt64 {
			rx = math.MaxInt64
		} else {
			rx += int64(network.RxBytes)
		}
		if network.TxBytes > math.MaxInt64 {
			tx = math.MaxInt64
		} else {
			tx += int64(network.TxBytes)
		}
	}
	c.NetRx, c.NetTx = rx, tx
}

func (c *Runc) ReadIO(stats *cgroups.Stats) {
	var read, write int64
	for _, blk := range stats.BlkioStats.IoServiceBytesRecursive {
		if blk.Op == "Read" {
			if blk.Value > math.MaxInt64 {
				read = math.MaxInt64
			} else {
				read = int64(blk.Value)
			}
		}
		if blk.Op == "Write" {
			if blk.Value > math.MaxInt64 {
				write = math.MaxInt64
			} else {
				write = int64(blk.Value)
			}
		}
	}
	c.IOBytesRead, c.IOBytesWrite = read, write
}
