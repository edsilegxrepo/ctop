package collector

import (
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/edsilegx/ctop/pkg/config"
	"github.com/edsilegx/ctop/pkg/models"
	api "github.com/fsouza/go-dockerclient"
)

// Docker collector
type Docker struct {
	models.Metrics
	id           string
	client       *api.Client
	running      atomic.Bool
	stream       chan models.Metrics
	done         chan bool
	lastCpu      float64
	lastSysCpu   float64
	cpuEMA       *models.EMA
	lastNetTx    int64
	lastNetRx    int64
	netTxEMA     *models.EMA
	netRxEMA     *models.EMA
	lastIORead   int64
	lastIOWrite  int64
	ioReadEMA    *models.EMA
	ioWriteEMA   *models.EMA
	lastNetTime  time.Time
	lastIOTime   time.Time
	cgroupIOPath string
	mu           sync.Mutex
}

func NewDocker(client *api.Client, id string) *Docker {
	return &Docker{
		Metrics: models.Metrics{},
		id:      id,
		client:  client,
	}
}

func (c *Docker) Start() {
	if !c.running.CompareAndSwap(false, true) {
		return
	}
	c.done = make(chan bool, 1)
	stream := make(chan models.Metrics, 16)
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
			c.mu.Lock()
			c.ReadCPU(s)
			c.ReadMem(s)
			c.ReadNet(s)
			c.ReadIO(s)
			metrics := c.Metrics
			c.mu.Unlock()
			select {
			case stream <- metrics:
			default:
			}
		}
		log.Infof("collector stopped for container: %s", c.id)
	}()

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
	if c.running.CompareAndSwap(true, false) {
		select {
		case c.done <- true:
		default:
		}
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
	rawCPU := percent(cpudiff, syscpudiff)
	if config.GetVal("cpuMode") == "per-core" && ncpus > 1 {
		rawCPU = rawCPU * ncpus
	}
	if c.cpuEMA == nil {
		c.cpuEMA = models.NewEMA(0.3)
	}
	c.CPUUtil = int(math.Round(c.cpuEMA.Update(float64(rawCPU))))
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

	rss := stats.MemoryStats.Stats.TotalRss
	if rss == 0 {
		rss = stats.MemoryStats.Stats.Rss
	}
	if rss > math.MaxInt64 {
		c.MemRss = math.MaxInt64
	} else {
		c.MemRss = int64(rss)
	}
	if cache > math.MaxInt64 {
		c.MemCache = math.MaxInt64
	} else {
		c.MemCache = int64(cache)
	}
	if stats.MemoryStats.Stats.Swap > math.MaxInt64 {
		c.MemSwap = math.MaxInt64
	} else {
		c.MemSwap = int64(stats.MemoryStats.Stats.Swap)
	}
	kMem := stats.MemoryStats.Stats.KernelStack + stats.MemoryStats.Stats.Slab
	if kMem > math.MaxInt64 {
		c.MemKernel = math.MaxInt64
	} else {
		c.MemKernel = int64(kMem)
	}
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

	now := time.Now()
	if !c.lastNetTime.IsZero() {
		elapsed := now.Sub(c.lastNetTime).Seconds()
		if elapsed > 0 {
			if c.lastNetTx > 0 && tx >= c.lastNetTx {
				txRate := float64(tx-c.lastNetTx) / elapsed
				if c.netTxEMA == nil {
					c.netTxEMA = models.NewEMA(0.3)
				}
				c.NetTxRate = int64(math.Round(c.netTxEMA.Update(txRate)))
			}
			if c.lastNetRx > 0 && rx >= c.lastNetRx {
				rxRate := float64(rx-c.lastNetRx) / elapsed
				if c.netRxEMA == nil {
					c.netRxEMA = models.NewEMA(0.3)
				}
				c.NetRxRate = int64(math.Round(c.netRxEMA.Update(rxRate)))
			}
		}
	}
	c.lastNetTx = tx
	c.lastNetRx = rx
	c.lastNetTime = now
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

	// Fallback to cgroups v2 io.stat on Linux if Docker API returns zero (moby/moby#35352)
	if read == 0 && write == 0 {
		r, w := c.readCgroupV2IO()
		if r > 0 || w > 0 {
			read, write = r, w
		}
	}

	now := time.Now()
	if !c.lastIOTime.IsZero() {
		elapsed := now.Sub(c.lastIOTime).Seconds()
		if elapsed > 0 {
			if c.lastIORead > 0 && read >= c.lastIORead {
				readRate := float64(read-c.lastIORead) / elapsed
				if c.ioReadEMA == nil {
					c.ioReadEMA = models.NewEMA(0.3)
				}
				c.IORateRead = int64(math.Round(c.ioReadEMA.Update(readRate)))
			}
			if c.lastIOWrite > 0 && write >= c.lastIOWrite {
				writeRate := float64(write-c.lastIOWrite) / elapsed
				if c.ioWriteEMA == nil {
					c.ioWriteEMA = models.NewEMA(0.3)
				}
				c.IORateWrite = int64(math.Round(c.ioWriteEMA.Update(writeRate)))
			}
		}
	}
	c.lastIORead = read
	c.lastIOWrite = write
	c.lastIOTime = now

	c.IOBytesRead, c.IOBytesWrite = read, write
}

func (c *Docker) readCgroupV2IO() (int64, int64) {
	if runtime.GOOS != "linux" {
		return 0, 0
	}

	// 1. Try cached path first
	if c.cgroupIOPath != "" {
		if r, w, ok := parseIOStat(c.cgroupIOPath); ok {
			return r, w
		}
	}

	// 2. Direct docker cgroup path
	directPath := filepath.Join("/sys/fs/cgroup/docker", c.id, "io.stat")
	if r, w, ok := parseIOStat(directPath); ok {
		c.cgroupIOPath = directPath
		return r, w
	}

	// 3. Scan system.slice for docker-<cid>*.scope
	entries, err := os.ReadDir("/sys/fs/cgroup/system.slice")
	if err == nil {
		for _, entry := range entries {
			name := entry.Name()
			if strings.HasPrefix(name, "docker-") && strings.Contains(name, c.id) && strings.HasSuffix(name, ".scope") {
				scopePath := filepath.Join("/sys/fs/cgroup/system.slice", name, "io.stat")
				if r, w, ok := parseIOStat(scopePath); ok {
					c.cgroupIOPath = scopePath
					return r, w
				}
			}
		}
	}

	return 0, 0
}

func parseIOStat(path string) (int64, int64, bool) {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return 0, 0, false
	}
	var totalRead, totalWrite int64
	var found bool
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		for _, field := range fields {
			if strings.HasPrefix(field, "rbytes=") {
				valStr := strings.TrimPrefix(field, "rbytes=")
				if val, err := strconv.ParseInt(valStr, 10, 64); err == nil {
					totalRead += val
					found = true
				}
			} else if strings.HasPrefix(field, "wbytes=") {
				valStr := strings.TrimPrefix(field, "wbytes=")
				if val, err := strconv.ParseInt(valStr, 10, 64); err == nil {
					totalWrite += val
					found = true
				}
			}
		}
	}
	return totalRead, totalWrite, found
}
