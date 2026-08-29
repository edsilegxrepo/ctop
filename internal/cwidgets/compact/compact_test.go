// compact_test.go validates compact grid row creation, column rendering, and cell value formatting.
// Test Strategy: Verifies text and gauge column models with synthetic metadata and metrics.
package compact

import (
	"image"
	"strings"
	"testing"

	"github.com/edsilegx/ctop/pkg/config"
	"github.com/edsilegx/ctop/pkg/models"
	ui "github.com/gizak/termui/v3"
)

func TestNewRowWidgets(t *testing.T) {
	config.Init()
	cols := newRowWidgets()
	if len(cols) == 0 {
		t.Fatalf("expected enabled columns, got 0")
	}

	foundName := false
	for _, c := range cols {
		if c.Header() == "NAME" {
			foundName = true
			if c.FixedWidth() != 0 {
				t.Errorf("expected NAME col fixed width 0 (auto), got %d", c.FixedWidth())
			}
		}
	}
	if !foundName {
		t.Errorf("expected NAME column to be present in default row widgets")
	}
}

func TestTextColSetMeta(t *testing.T) {
	col := NewNameCol()
	m := models.NewMeta("name", "my-test-container")
	col.SetMeta(m)

	textCol, ok := col.(*MetaCol)
	if !ok {
		t.Fatalf("expected *MetaCol type")
	}
	if textCol.Text != "my-test-container" {
		t.Errorf("expected Text='my-test-container', got '%s'", textCol.Text)
	}
}

func TestGaugeColSetMetrics(t *testing.T) {
	col := NewCPUCol()
	m := models.NewMetrics()
	m.CPUUtil = 42
	m.NCpus = 1
	col.SetMetrics(m)

	cpuCol, ok := col.(*CPUCol)
	if !ok {
		t.Fatalf("expected *CPUCol type")
	}
	if cpuCol.Percent != 42 {
		t.Errorf("expected Percent=42, got %d", cpuCol.Percent)
	}
	if cpuCol.Label != "42%" {
		t.Errorf("expected Label='42%%', got '%s'", cpuCol.Label)
	}
}

func TestAllColumnTypes(t *testing.T) {
	meta := models.NewMeta("name", "web-server")
	meta["id"] = "c12345678901"
	meta["state"] = "running"
	meta["ports"] = "80/tcp"
	meta["created"] = "10 minutes ago"

	metrics := models.NewMetrics()
	metrics.CPUUtil = 75
	metrics.NCpus = 4
	metrics.MemUsage = 512 * 1024 * 1024
	metrics.MemLimit = 1024 * 1024 * 1024
	metrics.MemPercent = 50
	metrics.NetRx = 1024
	metrics.NetTx = 2048
	metrics.IOBytesRead = 4096
	metrics.IOBytesWrite = 8192
	metrics.Pids = 12

	cols := []CompactCol{
		NewStatus(),
		NewNameCol(),
		NewCIDCol(),
		NewImageCol(),
		NewPortsCol(),
		NewIpsCol(),
		NewCreatedCol(),
		NewCPUCol(),
		NewCpuScaledCol(),
		NewMemCol(),
		NewNetCol(),
		NewIOCol(),
		NewPIDCol(),
		NewUptimeCol(),
	}

	buf := ui.NewBuffer(image.Rect(0, 0, 200, 10))

	for _, col := range cols {
		if _, ok := col.(*Status); ok {
			if col.Header() != "" {
				t.Errorf("expected empty header for Status column, got '%s'", col.Header())
			}
		}
		if _, isStatus := col.(*Status); !isStatus {
			if header := col.Header(); header == "" {
				t.Errorf("expected non-empty header for %T", col)
			}
		}
		col.SetMeta(meta)
		col.SetMetrics(metrics)
		col.SetRect(0, 0, 20, 1)
		col.Draw(buf)
		col.Reset()
		col.Draw(buf)
	}
}

func TestCompactRowAndGrid(t *testing.T) {
	config.Init()
	grid := NewCompactGrid()
	if grid == nil {
		t.Fatal("expected non-nil grid")
	}

	row1 := NewCompactRow()
	row2 := NewCompactRow()

	meta := models.NewMeta("name", "test-row")
	row1.SetMeta(meta)
	row1.Highlight()
	row1.UnHighlight()

	grid.AddRows(row1, row2)
	grid.SetY(2)
	grid.SetWidth(120)
	grid.Align()

	if height := grid.GetHeight(); height <= 0 {
		t.Fatalf("expected positive height, got %d", height)
	}

	if maxRows := grid.MaxRows(); maxRows < 0 {
		t.Fatalf("expected non-negative maxRows, got %d", maxRows)
	}

	buf := ui.NewBuffer(image.Rect(0, 0, 120, 40))
	grid.Draw(buf)

	// Offset paging
	grid.Offset = 1
	grid.Align()
	grid.Draw(buf)

	// Clear
	grid.Clear()
	if len(grid.Rows) != 0 {
		t.Fatalf("expected 0 rows after clear, got %d", len(grid.Rows))
	}
	grid.Draw(buf)
}

func TestRateModeAndCumulativeToggle(t *testing.T) {
	config.Init()

	netCol := NewNetCol()
	ioCol := NewIOCol()

	m := models.NewMetrics()
	m.NetRx = 10485760         // 10M
	m.NetTx = 5242880          // 5M
	m.NetRxRate = 102400       // 100K/s
	m.NetTxRate = 51200        // 50K/s
	m.IOBytesRead = 209715200  // 200M
	m.IOBytesWrite = 104857600 // 100M
	m.IORateRead = 204800      // 200K/s
	m.IORateWrite = 102400     // 100K/s

	// 1. Test Rate Mode (default: rateMode=true)
	config.UpdateSwitch("rateMode", true)
	if netCol.Header() != "NET (Rx / Tx)" {
		t.Errorf("expected NET (Rx / Tx) header in rate mode, got %s", netCol.Header())
	}
	if ioCol.Header() != "IO (Reads / Writes)" {
		t.Errorf("expected IO (Reads / Writes) header in rate mode, got %s", ioCol.Header())
	}
	netCol.SetMetrics(m)
	ioCol.SetMetrics(m)
	if netText := netCol.(*NetCol).Text; !strings.Contains(netText, "/s") {
		t.Errorf("expected net rate with /s in rate mode, got %s", netText)
	}
	if ioText := ioCol.(*IOCol).Text; !strings.Contains(ioText, "/s") {
		t.Errorf("expected io rate with /s in rate mode, got %s", ioText)
	}

	// 2. Test Cumulative Mode (rateMode=false)
	config.UpdateSwitch("rateMode", false)
	if netCol.Header() != "NET (Rx / Tx)" {
		t.Errorf("expected NET (Rx / Tx) header in cumulative mode, got %s", netCol.Header())
	}
	if ioCol.Header() != "IO (Reads / Writes)" {
		t.Errorf("expected IO (Reads / Writes) header in cumulative mode, got %s", ioCol.Header())
	}
	netCol.SetMetrics(m)
	ioCol.SetMetrics(m)
	if netText := netCol.(*NetCol).Text; strings.Contains(netText, "/s") {
		t.Errorf("expected cumulative net without /s, got %s", netText)
	}
	if ioText := ioCol.(*IOCol).Text; strings.Contains(ioText, "/s") {
		t.Errorf("expected cumulative io without /s, got %s", ioText)
	}
}
