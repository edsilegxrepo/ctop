// single_test.go validates detailed single-container inspection widgets (CPU/Mem sparklines, environment tables, network/IO diffs).
// Test Strategy: Tests history ring buffers, multiline table generators, and metric delta calculations.
package single

import (
	"image"
	"testing"
	"time"

	"github.com/edsilegx/ctop/models"
	ui "github.com/gizak/termui/v3"
)

func TestMkInfoRows(t *testing.T) {
	rows := mkInfoRows("ports", "80/tcp\n443/tcp")
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows for multiline ports, got %d", len(rows))
	}
	if rows[0][0] != "ports" || rows[0][1] != "80/tcp" {
		t.Errorf("unexpected first row: %v", rows[0])
	}
	if rows[1][0] != "" || rows[1][1] != "443/tcp" {
		t.Errorf("unexpected second row: %v", rows[1])
	}
}

func TestToFloat64Slice(t *testing.T) {
	input := []int{10, -5, 20, 0}
	res := toFloat64Slice(input)
	if len(res) != 4 {
		t.Fatalf("expected length 4, got %d", len(res))
	}
	if res[0] != 10.0 || res[1] != 0.0 || res[2] != 20.0 || res[3] != 0.0 {
		t.Errorf("unexpected result: %v", res)
	}
}

func TestNetUpdate(t *testing.T) {
	net := NewNet()
	net.Update(1000, 2000)
	net.Update(3000, 5000)

	if len(net.rxHist.Data) != 60 {
		t.Errorf("expected 60 points in rxHist, got %d", len(net.rxHist.Data))
	}
	if net.rxHist.Val != 2000 {
		t.Errorf("expected rx rate 2000, got %d", net.rxHist.Val)
	}
	if net.txHist.Val != 3000 {
		t.Errorf("expected tx rate 3000, got %d", net.txHist.Val)
	}
	if net.rxTitle != "RX [1.95kib/s]" {
		t.Errorf("unexpected rxTitle: %s", net.rxTitle)
	}

	buf := ui.NewBuffer(image.Rect(0, 0, 100, 20))
	net.SetRect(0, 0, 80, 12)
	net.Draw(buf)
}

func TestIOUpdate(t *testing.T) {
	io := NewIO()
	io.Update(500, 1000)
	io.Update(1500, 3000)

	if len(io.readHist.Data) != 60 {
		t.Errorf("expected 60 points in readHist, got %d", len(io.readHist.Data))
	}
	if io.readHist.Val != 1000 {
		t.Errorf("expected read rate 1000, got %d", io.readHist.Val)
	}

	buf := ui.NewBuffer(image.Rect(0, 0, 100, 20))
	io.SetRect(0, 0, 80, 12)
	io.Draw(buf)
}

func TestCpuAndMemWidgets(t *testing.T) {
	cpu := NewCpu()
	cpu.Update(85)
	if len(cpu.hist.Data) == 0 || cpu.hist.Data[len(cpu.hist.Data)-1] != 85.0 {
		t.Errorf("expected CPU data ending in 85.0")
	}

	buf := ui.NewBuffer(image.Rect(0, 0, 100, 20))
	cpu.SetRect(0, 0, 80, 12)
	cpu.Draw(buf)

	mem := NewMem()
	mem.Update(512*1024*1024, 1024*1024*1024)
	if mem.val != 512*1024*1024 || mem.limit != 1024*1024*1024 {
		t.Errorf("expected mem val and limit to be updated")
	}
	mem.SetRect(0, 0, 80, 12)
	mem.Draw(buf)
}

func TestEnvAndInfoWidgets(t *testing.T) {
	env := NewEnv()
	env.Set("FOO=BAR;BAZ=QUX")
	if env.GetHeight() != 4 { // 2 lines + 2 borders
		t.Errorf("expected env height 4, got %d", env.GetHeight())
	}
	buf := ui.NewBuffer(image.Rect(0, 0, 100, 20))
	env.SetRect(0, 0, 80, 10)
	env.Draw(buf)

	info := NewInfo()
	info.Set("name", "test-server")
	info.Set("state", "running")
	if info.GetHeight() <= 0 {
		t.Errorf("expected positive info height, got %d", info.GetHeight())
	}
	info.SetRect(0, 0, 80, 10)
	info.Draw(buf)
}

func TestSingleView(t *testing.T) {
	s := NewSingle()
	if s == nil {
		t.Fatal("expected non-nil Single")
	}

	meta := models.NewMeta("name", "nginx-ingress")
	meta["[ENV-VAR]"] = "PORT=80"
	s.SetMeta(meta)

	metrics := models.NewMetrics()
	metrics.CPUUtil = 45
	metrics.NetRx = 2048
	metrics.NetTx = 4096
	metrics.MemUsage = 256 * 1024 * 1024
	metrics.MemLimit = 512 * 1024 * 1024
	metrics.IOBytesRead = 1024
	metrics.IOBytesWrite = 2048
	s.SetMetrics(metrics)

	s.SetWidth(120)
	s.Up()
	s.Down()

	if height := s.GetHeight(); height <= 0 {
		t.Fatalf("expected positive height, got %d", height)
	}

	s.Align()
	buf := ui.NewBuffer(image.Rect(0, 0, 120, 80))
	s.Draw(buf)
}

func TestLogsWidget(t *testing.T) {
	stream := make(chan models.Log, 5)
	logsWidget := NewLogs(stream)

	stream <- models.Log{Timestamp: time.Now(), Message: "starting service..."}
	stream <- models.Log{Timestamp: time.Now(), Message: "service ready"}
	time.Sleep(50 * time.Millisecond)

	buf := ui.NewBuffer(image.Rect(0, 0, 80, 20))
	logsWidget.SetRect(0, 0, 80, 20)
	logsWidget.Draw(buf)
	close(stream)
}
