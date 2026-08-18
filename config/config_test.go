package config

import (
	"testing"
)

func TestConfigInit(t *testing.T) {
	Init()

	if len(GlobalParams) == 0 {
		t.Fatalf("expected GlobalParams to be populated, got 0")
	}
	if len(GlobalSwitches) == 0 {
		t.Fatalf("expected GlobalSwitches to be populated, got 0")
	}
	if len(GlobalColumns) == 0 {
		t.Fatalf("expected GlobalColumns to be populated, got 0")
	}
}

func TestConfigParams(t *testing.T) {
	Init()

	Update("filterStr", "test-filter")
	if val := GetVal("filterStr"); val != "test-filter" {
		t.Errorf("expected filterStr='test-filter', got '%s'", val)
	}

	Update("sortField", "cpu")
	if val := GetVal("sortField"); val != "cpu" {
		t.Errorf("expected sortField='cpu', got '%s'", val)
	}

	// Test non-existent param returns empty
	if val := GetVal("nonExistentKey"); val != "" {
		t.Errorf("expected empty string for nonExistentKey, got '%s'", val)
	}
}

func TestConfigSwitches(t *testing.T) {
	Init()

	initialVal := GetSwitchVal("sortReversed")
	Toggle("sortReversed")
	if GetSwitchVal("sortReversed") != !initialVal {
		t.Errorf("expected sortReversed to toggle from %v to %v", initialVal, !initialVal)
	}

	UpdateSwitch("sortReversed", true)
	if !GetSwitchVal("sortReversed") {
		t.Errorf("expected sortReversed=true")
	}

	UpdateSwitch("sortReversed", false)
	if GetSwitchVal("sortReversed") {
		t.Errorf("expected sortReversed=false")
	}
}
