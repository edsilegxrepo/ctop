package compact

import (
	"testing"

	"github.com/bcicen/ctop/config"
	"github.com/bcicen/ctop/models"
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
