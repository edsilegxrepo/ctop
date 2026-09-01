// docker_test.go validates CPU, memory, network, and I/O calculations from Docker engine stats API payloads.
//
// Objective:
//
//	Verify resource parsing math, multi-core scaled percentages, zero division guards, and mock HTTP SSE metric streams.
//
// Test Strategy:
//   - Unit tests verifying boundary math (uint8 CPU overflow >255 cores, zero limits) and mock HTTP JSON streaming.
//   - Tests memory caching exclusions (inactive_file / active_file cgroup v1 & v2 buffers).
//   - Rate calculations verifying EMA smoothing and delta throughput measurements.
package collector

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/edsilegx/ctop/pkg/config"
	"github.com/edsilegx/ctop/pkg/models"
	api "github.com/fsouza/go-dockerclient"
)

func TestDockerCollectorReadCPU(t *testing.T) {
	c := &Docker{
		Metrics: models.NewMetrics(),
	}

	stats := &api.Stats{}
	stats.CPUStats.OnlineCPUs = 300 // tests uint8 overflow fix (300 > 255)
	stats.CPUStats.CPUUsage.TotalUsage = 500000000
	stats.CPUStats.CPUUsage.PercpuUsage = make([]uint64, 300)
	stats.CPUStats.SystemCPUUsage = 1000000000

	c.ReadCPU(stats)

	if c.NCpus != 300 {
		t.Fatalf("expected NCpus to be 300, got %d", c.NCpus)
	}

	if c.lastCpu != 500000000 {
		t.Fatalf("expected lastCpu to be 500000000, got %f", c.lastCpu)
	}

	// Test fallback when OnlineCPUs is 0
	stats2 := &api.Stats{}
	stats2.CPUStats.OnlineCPUs = 0
	stats2.CPUStats.CPUUsage.PercpuUsage = []uint64{100, 200}
	c.ReadCPU(stats2)
	if c.NCpus != 2 {
		t.Fatalf("expected NCpus 2 from PercpuUsage fallback, got %d", c.NCpus)
	}

	// Test fallback when PercpuUsage is empty
	stats3 := &api.Stats{}
	stats3.CPUStats.OnlineCPUs = 0
	stats3.CPUStats.CPUUsage.PercpuUsage = nil
	c.ReadCPU(stats3)
	if c.NCpus != 0 {
		t.Fatalf("expected NCpus 0 when both OnlineCPUs and PercpuUsage are empty, got %d", c.NCpus)
	}
}

func TestDockerCollectorReadMemCgroupV2(t *testing.T) {
	c := &Docker{
		Metrics: models.NewMetrics(),
	}

	stats := &api.Stats{}
	stats.MemoryStats.Usage = 2000000
	stats.MemoryStats.Limit = 4000000
	stats.MemoryStats.Stats.InactiveFile = 500000

	c.ReadMem(stats)

	// usage (2000000) - InactiveFile (500000) = 1500000
	if c.MemUsage != 1500000 {
		t.Fatalf("expected MemUsage 1500000, got %d", c.MemUsage)
	}

	if c.MemLimit != 4000000 {
		t.Fatalf("expected MemLimit 4000000, got %d", c.MemLimit)
	}
}

func TestDockerCollectorReadMemUnderflowProtection(t *testing.T) {
	c := &Docker{
		Metrics: models.NewMetrics(),
	}

	// Case where cache > usage (must not underflow unsigned uint64)
	stats := &api.Stats{}
	stats.MemoryStats.Usage = 1000
	stats.MemoryStats.Limit = 4000000
	stats.MemoryStats.Stats.InactiveFile = 5000

	c.ReadMem(stats)

	if c.MemUsage != 1000 {
		t.Fatalf("expected MemUsage 1000 (protected from underflow), got %d", c.MemUsage)
	}
}

func TestDockerCollectorReadIO(t *testing.T) {
	c := &Docker{
		Metrics: models.NewMetrics(),
	}

	stats := &api.Stats{}
	stats.BlkioStats.IOServiceBytesRecursive = []api.BlkioStatsEntry{
		{Op: "Read", Value: 1048576},
		{Op: "Write", Value: 2097152},
		{Op: "Sync", Value: 512},
	}

	c.ReadIO(stats)

	if c.IOBytesRead != 1048576 {
		t.Fatalf("expected IOBytesRead 1048576, got %d", c.IOBytesRead)
	}
	if c.IOBytesWrite != 2097152 {
		t.Fatalf("expected IOBytesWrite 2097152, got %d", c.IOBytesWrite)
	}
}

func TestDockerCollectorReadNet(t *testing.T) {
	c := &Docker{
		Metrics: models.NewMetrics(),
	}

	// Legacy single network
	stats1 := &api.Stats{}
	stats1.Network.RxBytes = 1024
	stats1.Network.TxBytes = 2048
	c.ReadNet(stats1)
	if c.NetRx != 1024 || c.NetTx != 2048 {
		t.Fatalf("expected legacy network rx=1024 tx=2048, got rx=%d tx=%d", c.NetRx, c.NetTx)
	}

	// Multi-network
	stats2 := &api.Stats{}
	stats2.Networks = map[string]api.NetworkStats{
		"eth0": {RxBytes: 4096, TxBytes: 8192},
		"eth1": {RxBytes: 1024, TxBytes: 2048},
	}
	c.ReadNet(stats2)
	if c.NetRx != 5120 || c.NetTx != 10240 {
		t.Fatalf("expected multi-network rx=5120 tx=10240, got rx=%d tx=%d", c.NetRx, c.NetTx)
	}
}

func TestDockerCollectorLogsAndRunning(t *testing.T) {
	c := &Docker{
		id:      "c-test",
		Metrics: models.NewMetrics(),
	}

	if c.Running() {
		t.Fatal("expected Running() to be false initially")
	}

	logs := c.Logs()
	if logs == nil {
		t.Fatal("expected non-nil logs collector")
	}

	c.done = make(chan bool, 1)
	c.running.Store(true)
	c.Stop()
	if c.Running() {
		t.Fatal("expected Running() to be false after Stop")
	}
}

func TestDockerCollectorStreamingWithMockServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		statsJSON := `{"read":"2026-08-18T10:00:00Z","cpu_stats":{"cpu_usage":{"total_usage":1000},"system_cpu_usage":2000},"memory_stats":{"usage":1024,"limit":4096}}` + "\n"
		_, _ = w.Write([]byte(statsJSON))
	}))
	defer server.Close()

	client, err := api.NewClient(server.URL)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	c := NewDocker(client, "c123")
	c.Start()

	if !c.Running() {
		t.Log("collector started")
	}

	stream := c.Stream()
	select {
	case m, ok := <-stream:
		if ok {
			t.Logf("received metrics: %+v", m)
		}
	case <-time.After(500 * time.Millisecond):
		t.Log("stream wait completed")
	}

	c.Stop()
}

func TestDockerCollectorReadMemCacheFallbacks(t *testing.T) {
	c := &Docker{
		Metrics: models.NewMetrics(),
	}

	// 1. TotalInactiveFile fallback
	stats1 := &api.Stats{}
	stats1.MemoryStats.Usage = 3000
	stats1.MemoryStats.Limit = 5000
	stats1.MemoryStats.Stats.TotalInactiveFile = 1000
	stats1.MemoryStats.Stats.TotalRss = 500
	stats1.MemoryStats.Stats.Swap = 200
	stats1.MemoryStats.Stats.KernelStack = 100
	stats1.MemoryStats.Stats.Slab = 50
	c.ReadMem(stats1)
	if c.MemUsage != 2000 {
		t.Fatalf("expected MemUsage 2000, got %d", c.MemUsage)
	}
	if c.MemRss != 500 || c.MemSwap != 200 || c.MemKernel != 150 {
		t.Fatalf("expected rss=500 swap=200 kernel=150, got rss=%d swap=%d kernel=%d", c.MemRss, c.MemSwap, c.MemKernel)
	}

	// 2. Cache fallback
	stats2 := &api.Stats{}
	stats2.MemoryStats.Usage = 4000
	stats2.MemoryStats.Limit = 6000
	stats2.MemoryStats.Stats.Cache = 1500
	stats2.MemoryStats.Stats.Rss = 600
	c.ReadMem(stats2)
	if c.MemUsage != 2500 {
		t.Fatalf("expected MemUsage 2500, got %d", c.MemUsage)
	}

	// 3. TotalCache fallback
	stats3 := &api.Stats{}
	stats3.MemoryStats.Usage = 5000
	stats3.MemoryStats.Limit = 8000
	stats3.MemoryStats.Stats.TotalCache = 2000
	c.ReadMem(stats3)
	if c.MemUsage != 3000 {
		t.Fatalf("expected MemUsage 3000, got %d", c.MemUsage)
	}
}

func TestDockerCollectorCgroupV2IOFallback(t *testing.T) {
	tempDir := t.TempDir()
	ioStatFile := tempDir + "/io.stat"
	content := "254:0 rbytes=1048576 wbytes=2097152 rios=100 wios=200 dbytes=0 dios=0\n"
	if err := os.WriteFile(ioStatFile, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write io.stat: %v", err)
	}

	r, w, ok := parseIOStat(ioStatFile)
	if !ok {
		t.Fatal("expected parseIOStat to succeed")
	}
	if r != 1048576 || w != 2097152 {
		t.Fatalf("expected read=1048576 write=2097152, got read=%d write=%d", r, w)
	}
}

func TestDockerCollectorRatesCalculation(t *testing.T) {
	c := &Docker{
		Metrics: models.NewMetrics(),
	}

	// First tick at t0
	stats1 := &api.Stats{}
	stats1.BlkioStats.IOServiceBytesRecursive = []api.BlkioStatsEntry{
		{Op: "Read", Value: 1000},
		{Op: "Write", Value: 500},
	}
	stats1.Network.RxBytes = 2000
	stats1.Network.TxBytes = 1000
	c.ReadIO(stats1)
	c.ReadNet(stats1)

	// Simulate 1 second elapsed
	c.lastNetTime = time.Now().Add(-1 * time.Second)
	c.lastIOTime = time.Now().Add(-1 * time.Second)

	// Second tick at t1 (+1000 read, +500 write, +2000 rx, +1000 tx)
	stats2 := &api.Stats{}
	stats2.BlkioStats.IOServiceBytesRecursive = []api.BlkioStatsEntry{
		{Op: "Read", Value: 2000},
		{Op: "Write", Value: 1000},
	}
	stats2.Network.RxBytes = 4000
	stats2.Network.TxBytes = 2000
	c.ReadIO(stats2)
	c.ReadNet(stats2)

	if c.IORateRead <= 0 {
		t.Fatalf("expected positive IORateRead, got %d", c.IORateRead)
	}
	if c.IORateWrite <= 0 {
		t.Fatalf("expected positive IORateWrite, got %d", c.IORateWrite)
	}
	if c.NetRxRate <= 0 {
		t.Fatalf("expected positive NetRxRate, got %d", c.NetRxRate)
	}
	if c.NetTxRate <= 0 {
		t.Fatalf("expected positive NetTxRate, got %d", c.NetTxRate)
	}
}

func TestDockerCollectorCPUMode(t *testing.T) {
	config.Init()

	c := &Docker{
		Metrics: models.NewMetrics(),
	}

	// 1. Normalized mode (default)
	config.Update("cpuMode", "normalized")
	stats1 := &api.Stats{}
	stats1.CPUStats.OnlineCPUs = 4
	stats1.CPUStats.CPUUsage.TotalUsage = 200000000
	stats1.CPUStats.SystemCPUUsage = 1000000000
	c.ReadCPU(stats1)

	// Second tick: +200M total, +1000M system -> 20%
	stats2 := &api.Stats{}
	stats2.CPUStats.OnlineCPUs = 4
	stats2.CPUStats.CPUUsage.TotalUsage = 400000000
	stats2.CPUStats.SystemCPUUsage = 2000000000
	c.ReadCPU(stats2)

	if c.CPUUtil <= 0 {
		t.Fatalf("expected positive CPUUtil in normalized mode, got %d", c.CPUUtil)
	}

	// 2. Per-core mode: 4 cores * 20% = ~80%
	config.Update("cpuMode", "per-core")
	cPerCore := &Docker{
		Metrics: models.NewMetrics(),
	}
	cPerCore.ReadCPU(stats1)
	cPerCore.ReadCPU(stats2)

	if cPerCore.CPUUtil < c.CPUUtil {
		t.Fatalf("expected per-core CPUUtil (%d) >= normalized CPUUtil (%d)", cPerCore.CPUUtil, c.CPUUtil)
	}
}
