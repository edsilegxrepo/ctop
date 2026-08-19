package collector

import (
	"math"
	"strings"
	"sync/atomic"

	"github.com/edsilegx/ctop/models"
	api "github.com/fsouza/go-dockerclient"
)

// Docker collector
type Docker struct {
	models.Metrics
	id         string
	client     *api.Client
	running    atomic.Bool
	stream     chan models.Metrics
	done       chan bool
	lastCpu    float64
	lastSysCpu float64
}

func NewDocker(client *api.Client, id string) *Docker {
	return &Docker{
		Metrics: models.Metrics{},
		id:      id,
		client:  client,
	}
}

func (c *Docker) Start() {
	c.done = make(chan bool, 1)
	stream := make(chan models.Metrics)
	c.stream = stream
	stats := make(chan *api.Stats)

	go func() {
		opts := api.StatsOptions{
			ID:     c.id,
			Stats:  stats,
			Stream: true,
			Done:   c.done,
		}
		if err := c.client.Stats(opts); err != nil {
			log.Errorf("collector failed for container %s: %s", c.id, err)
		}
		c.running.Store(false)
	}()

	go func() {
		defer close(stream)
		for s := range stats {
			c.ReadCPU(s)
			c.ReadMem(s)
			c.ReadNet(s)
			c.ReadIO(s)
			stream <- c.Metrics
		}
		log.Infof("collector stopped for container: %s", c.id)
	}()

	c.running.Store(true)
	log.Infof("collector started for container: %s", c.id)
}

func (c *Docker) Running() bool {
	return c.running.Load()
}

func (c *Docker) Stream() chan models.Metrics {
	return c.stream
}

func (c *Docker) Logs() LogCollector {
	return NewDockerLogs(c.id, c.client)
}

// Stop collector
func (c *Docker) Stop() {
	c.running.Store(false)
	select {
	case c.done <- true:
	default:
	}
}

func (c *Docker) ReadCPU(stats *api.Stats) {
	var ncpus int
	if stats.CPUStats.OnlineCPUs > math.MaxInt {
		ncpus = math.MaxInt
	} else {
		ncpus = int(stats.CPUStats.OnlineCPUs)
	}
	if ncpus == 0 {
		ncpus = len(stats.CPUStats.CPUUsage.PercpuUsage)
	}
	total := float64(stats.CPUStats.CPUUsage.TotalUsage)
	system := float64(stats.CPUStats.SystemCPUUsage)

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

func (c *Docker) ReadMem(stats *api.Stats) {
	cache := stats.MemoryStats.Stats.InactiveFile
	if cache == 0 {
		cache = stats.MemoryStats.Stats.TotalInactiveFile
	}
	if cache == 0 {
		cache = stats.MemoryStats.Stats.Cache
	}
	if cache == 0 {
		cache = stats.MemoryStats.Stats.TotalCache
	}

	usage := stats.MemoryStats.Usage
	if usage > cache {
		usage -= cache
	}

	if usage > math.MaxInt64 {
		c.MemUsage = math.MaxInt64
	} else {
		c.MemUsage = int64(usage)
	}
	if stats.MemoryStats.Limit > math.MaxInt64 {
		c.MemLimit = math.MaxInt64
	} else {
		c.MemLimit = int64(stats.MemoryStats.Limit)
	}
	c.MemPercent = percent(float64(c.MemUsage), float64(c.MemLimit))
}

func (c *Docker) ReadNet(stats *api.Stats) {
	var rx, tx int64
	if len(stats.Networks) > 0 {
		for _, network := range stats.Networks {
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
	} else {
		if stats.Network.RxBytes > math.MaxInt64 {
			rx = math.MaxInt64
		} else {
			rx = int64(stats.Network.RxBytes)
		}
		if stats.Network.TxBytes > math.MaxInt64 {
			tx = math.MaxInt64
		} else {
			tx = int64(stats.Network.TxBytes)
		}
	}
	c.NetRx, c.NetTx = rx, tx
}

func (c *Docker) ReadIO(stats *api.Stats) {
	var read, write int64
	for _, blk := range stats.BlkioStats.IOServiceBytesRecursive {
		if strings.EqualFold(blk.Op, "read") {
			if blk.Value > math.MaxInt64 {
				read = math.MaxInt64
			} else {
				read += int64(blk.Value)
			}
		}
		if strings.EqualFold(blk.Op, "write") {
			if blk.Value > math.MaxInt64 {
				write = math.MaxInt64
			} else {
				write += int64(blk.Value)
			}
		}
	}
	c.IOBytesRead, c.IOBytesWrite = read, write
}
