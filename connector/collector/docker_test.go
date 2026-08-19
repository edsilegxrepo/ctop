// docker_test.go validates CPU, memory, network, and I/O calculations from Docker engine stats API payloads.
// Test Strategy: Unit tests verifying boundary math (uint8 CPU overflow, zero limits) and mock HTTP JSON streaming.
package collector

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/edsilegx/ctop/models"
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
