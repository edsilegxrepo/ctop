package models

import (
	"testing"
)

func TestNewMeta(t *testing.T) {
	m := NewMeta("name", "ctop", "state", "running")
	if m.Get("name") != "ctop" {
		t.Errorf("expected name='ctop', got '%s'", m.Get("name"))
	}
	if m.Get("state") != "running" {
		t.Errorf("expected state='running', got '%s'", m.Get("state"))
	}
	if m.Get("nonexistent") != "" {
		t.Errorf("expected empty string for nonexistent key, got '%s'", m.Get("nonexistent"))
	}
}

func TestNewMetrics(t *testing.T) {
	m := NewMetrics()
	if m.CPUUtil != -1 {
		t.Errorf("expected default CPUUtil=-1, got %d", m.CPUUtil)
	}
	if m.MemUsage != -1 {
		t.Errorf("expected default MemUsage=-1, got %d", m.MemUsage)
	}
	if m.NetTx != -1 || m.NetRx != -1 {
		t.Errorf("expected default NetTx/NetRx=-1, got %d/%d", m.NetTx, m.NetRx)
	}
}

func TestEMA(t *testing.T) {
	ema := NewEMA(0.5)
	v1 := ema.Update(100.0)
	if v1 != 100.0 {
		t.Errorf("expected initial sample 100.0, got %f", v1)
	}
	v2 := ema.Update(50.0)
	if v2 != 75.0 {
		t.Errorf("expected 0.5*50 + 0.5*100 = 75.0, got %f", v2)
	}
	v3 := ema.Update(50.0)
	if v3 != 62.5 {
		t.Errorf("expected 0.5*50 + 0.5*75 = 62.5, got %f", v3)
	}
}
