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
