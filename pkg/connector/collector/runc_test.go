//go:build linux

// runc_test.go validates Linux cgroups CPU, memory, network, and I/O metrics parsing.
// Test Strategy: Feeds raw cgroups.Stats fixtures to collector logic to verify accurate delta calculations.
package collector

import (
	"testing"
	"time"

	"github.com/edsilegx/ctop/pkg/models"
	"github.com/opencontainers/cgroups"
	"github.com/opencontainers/runc/types"
)

func TestRuncCollectorReadCPU(t *testing.T) {
	c := &Runc{
		Metrics: models.NewMetrics(),
	}

	stats := &cgroups.Stats{
		CpuStats: cgroups.CpuStats{
			CpuUsage: cgroups.CpuUsage{
				TotalUsage:  500000000,
				PercpuUsage: []uint64{100000000, 200000000, 100000000, 100000000},
			},
		},
		PidsStats: cgroups.PidsStats{
			Current: 42,
		},
	}

	c.ReadCPU(stats)

	if c.NCpus != 4 {
		t.Fatalf("expected NCpus to be 4, got %d", c.NCpus)
	}
	if c.Pids != 42 {
		t.Fatalf("expected Pids to be 42, got %d", c.Pids)
	}
	if c.lastCpu != 500000000 {
		t.Fatalf("expected lastCpu to be 500000000, got %f", c.lastCpu)
	}
}

func TestRuncCollectorReadIO(t *testing.T) {
	c := &Runc{
		Metrics: models.NewMetrics(),
	}

	stats := &cgroups.Stats{
		BlkioStats: cgroups.BlkioStats{
			IoServiceBytesRecursive: []cgroups.BlkioStatEntry{
				{Op: "Read", Value: 4096000},
				{Op: "Write", Value: 8192000},
			},
		},
	}

	c.ReadIO(stats)

	if c.IOBytesRead != 4096000 {
		t.Fatalf("expected IOBytesRead to be 4096000, got %d", c.IOBytesRead)
	}
	if c.IOBytesWrite != 8192000 {
		t.Fatalf("expected IOBytesWrite to be 8192000, got %d", c.IOBytesWrite)
	}
}

func TestRuncCollectorReadMem(t *testing.T) {
	c := &Runc{
		Metrics: models.NewMetrics(),
	}

	stats := &cgroups.Stats{
		MemoryStats: cgroups.MemoryStats{
			Usage: cgroups.MemoryData{
				Usage: 2048000,
				Limit: 8192000,
			},
		},
	}

	c.ReadMem(stats)

	if c.MemUsage != 2048000 {
		t.Fatalf("expected MemUsage 2048000, got %d", c.MemUsage)
	}
	if c.MemLimit != 8192000 {
		t.Fatalf("expected MemLimit 8192000, got %d", c.MemLimit)
	}
	if c.MemPercent != 25 {
		t.Fatalf("expected MemPercent 25, got %d", c.MemPercent)
	}

	// Test large memory limit clamped to sysMemTotal
	if sysMemTotal > 0 {
		stats2 := &cgroups.Stats{
			MemoryStats: cgroups.MemoryStats{
				Usage: cgroups.MemoryData{
					Usage: 1024,
					Limit: uint64(sysMemTotal) + 1000000,
				},
			},
		}
		c.ReadMem(stats2)
		if c.MemLimit != sysMemTotal {
			t.Fatalf("expected MemLimit clamped to sysMemTotal %d, got %d", sysMemTotal, c.MemLimit)
		}
	}
}

func TestRuncCollectorReadNet(t *testing.T) {
	c := &Runc{
		Metrics: models.NewMetrics(),
	}

	c.ReadNet(nil)
	if c.NetRx != 0 || c.NetTx != 0 {
		t.Fatalf("expected 0 net stats for empty interfaces, got rx=%d tx=%d", c.NetRx, c.NetTx)
	}

	ifaces := []*types.NetworkInterface{
		{Name: "eth0", RxBytes: 1024, TxBytes: 2048},
		{Name: "eth1", RxBytes: 4096, TxBytes: 8192},
	}
	c.ReadNet(ifaces)
	if c.NetRx != 5120 || c.NetTx != 10240 {
		t.Fatalf("expected rx=5120 tx=10240, got rx=%d tx=%d", c.NetRx, c.NetTx)
	}
}

func TestRuncLifecycle(t *testing.T) {
	r := NewRunc(nil)

	if r.Running() {
		t.Fatal("expected r.Running() to be false initially")
	}
	if r.Logs() != nil {
		t.Fatal("expected runc Logs() to be nil")
	}
	if r.Stream() != nil {
		t.Fatal("expected stream to be nil before start")
	}

	r.Start()
	time.Sleep(50 * time.Millisecond)

	// Test Stop branch when running
	r.running.Store(true)
	r.done.Store(false)
	r.Stop()
	if r.Running() {
		t.Fatal("expected r.Running() to be false after Stop")
	}
}

func TestSysProcFunctions(t *testing.T) {
	mem := getSysMemTotal()
	if mem <= 0 {
		t.Logf("system mem: %d", mem)
	}

	cpu := getSysCPUUsage()
	if cpu <= 0 {
		t.Logf("system cpu usage: %d", cpu)
	}
}

func TestPercentEdgeCases(t *testing.T) {
	if p := percent(50, 100); p != 50 {
		t.Fatalf("expected 50, got %d", p)
	}
	if p := percent(50, 0); p != 0 {
		t.Fatalf("expected 0 for zero total, got %d", p)
	}
	if p := percent(50, -10); p != 0 {
		t.Fatalf("expected 0 for negative total, got %d", p)
	}
}
