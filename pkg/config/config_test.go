// config_test.go tests parameter mutation, switch toggling, TOML serialization, path resolution, and column management.
//
// Objective:
//
//	Verify configuration lifecycle, TOML round-trip persistence, default value fallback, and column ordering.
//
// Test Strategy:
//   - Isolated filesystem tests using t.TempDir() to verify TOML encoding, path fallback rules, and column reordering.
//   - Tests parameter updates and boolean switch toggles with concurrency protection.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
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

	Update("downloadDir", "/custom/downloads")
	if val := GetVal("downloadDir"); val != "/custom/downloads" {
		t.Errorf("expected downloadDir='/custom/downloads', got '%s'", val)
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

	// Test logWrap defaults to false (truncate by default)
	if GetSwitchVal("logWrap") != false {
		t.Errorf("expected logWrap to default to false, got true")
	}
	Toggle("logWrap")
	if !GetSwitchVal("logWrap") {
		t.Errorf("expected logWrap to toggle to true")
	}
	Toggle("logWrap")
	if GetSwitchVal("logWrap") {
		t.Errorf("expected logWrap to toggle back to false")
	}
}

func TestConfigPathResolution(t *testing.T) {
	// 1. With XDG_CONFIG_HOME
	tempDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tempDir)

	path, err := getConfigPath()
	if err != nil {
		t.Fatalf("unexpected error getting config path with XDG_CONFIG_HOME: %v", err)
	}
	expected := filepath.Join(tempDir, "ctop", "config")
	if path != expected {
		t.Fatalf("expected path '%s', got '%s'", expected, path)
	}

	// 2. Clear XDG_CONFIG_HOME and test standard resolution
	t.Setenv("XDG_CONFIG_HOME", "")
	path2, err := getConfigPath()
	if err != nil {
		t.Fatalf("unexpected error with standard resolution: %v", err)
	}
	if path2 == "" {
		t.Fatal("expected non-empty config path")
	}
}

func TestConfigWriteAndRead(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tempDir)

	Init()
	Update("filterStr", "write-test-filter")
	UpdateSwitch("sortReversed", true)

	writtenPath, err := Write()
	if err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	if writtenPath == "" {
		t.Fatal("expected non-empty written path")
	}

	// Reset in-memory values
	Update("filterStr", "")
	UpdateSwitch("sortReversed", false)

	// Read from written config file
	err = Read()
	if err != nil {
		t.Fatalf("failed to read written config: %v", err)
	}

	if val := GetVal("filterStr"); val != "write-test-filter" {
		t.Fatalf("expected filterStr='write-test-filter', got '%s'", val)
	}
	if val := GetSwitchVal("sortReversed"); !val {
		t.Fatalf("expected sortReversed=true, got %v", val)
	}
}

func TestConfigLegacyPath(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", tempHome)
	t.Setenv("USERPROFILE", tempHome)

	legacyFile := filepath.Join(tempHome, ".ctop")
	if err := os.WriteFile(legacyFile, []byte(""), 0o644); err != nil {
		t.Fatalf("failed to create legacy .ctop file: %v", err)
	}

	path, err := getConfigPath()
	if err != nil {
		t.Fatalf("unexpected error resolving legacy path: %v", err)
	}
	if path != legacyFile {
		t.Fatalf("expected legacy path '%s', got '%s'", legacyFile, path)
	}
}

func TestConfigColumnsRead(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tempDir)

	Init()
	SetColumns([]string{"name", "cpu", "mem"})
	_, err := Write()
	if err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	// Reset columns to different configuration
	SetColumns([]string{"name"})

	err = Read()
	if err != nil {
		t.Fatalf("failed to read config: %v", err)
	}

	cols := ColumnsString()
	if cols != "name,cpu,mem" {
		t.Fatalf("expected columns 'name,cpu,mem', got '%s'", cols)
	}
}

func TestColumnToggleAndShift(t *testing.T) {
	Init()
	SetColumns([]string{"name", "id", "cpu", "mem"})

	// Test ColumnToggle
	ColumnToggle("cpu")
	enabled := EnabledColumns()
	found := false
	for _, c := range enabled {
		if c == "cpu" {
			found = true
		}
	}
	if found {
		t.Fatalf("expected 'cpu' to be disabled after toggle")
	}

	// Toggle back
	ColumnToggle("cpu")

	// Test ColumnLeft and ColumnRight
	ColumnLeft("id") // index 1 -> 0
	if GlobalColumns[0].Name != "id" {
		t.Fatalf("expected 'id' at index 0 after ColumnLeft, got '%s'", GlobalColumns[0].Name)
	}

	// ColumnLeft at index 0 does nothing
	ColumnLeft("id")
	if GlobalColumns[0].Name != "id" {
		t.Fatalf("expected 'id' to remain at index 0")
	}

	// ColumnRight
	ColumnRight("id") // index 0 -> 1
	if GlobalColumns[1].Name != "id" {
		t.Fatalf("expected 'id' at index 1 after ColumnRight, got '%s'", GlobalColumns[1].Name)
	}

	// ColumnRight at end of slice does nothing
	lastIdx := len(GlobalColumns) - 1
	lastName := GlobalColumns[lastIdx].Name
	ColumnRight(lastName)
	if GlobalColumns[lastIdx].Name != lastName {
		t.Fatalf("expected '%s' to remain at last index", lastName)
	}

	// Test popColumn panic
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("expected panic on popColumn with invalid column name")
		}
	}()
	popColumn("non-existent-col-xyz")
}

func TestDefaultDownloadDir(t *testing.T) {
	t.Setenv("CTOP_DOWNLOAD_DIR", "")
	Init()

	if DefaultDownloadDir != "/tmp" {
		t.Fatalf("expected DefaultDownloadDir constant to be '/tmp', got '%s'", DefaultDownloadDir)
	}

	if val := GetDownloadDir(); val != "/tmp" {
		t.Fatalf("expected GetDownloadDir() to return '/tmp' by default, got '%s'", val)
	}

	// When explicitly set to empty or whitespace, should fall back to DefaultDownloadDir
	Update("downloadDir", "")
	if val := GetDownloadDir(); val != "/tmp" {
		t.Fatalf("expected GetDownloadDir() on empty string to fall back to '/tmp', got '%s'", val)
	}

	SetDownloadDir("   ")
	if val := GetDownloadDir(); val != "/tmp" {
		t.Fatalf("expected SetDownloadDir('   ') to fall back to '/tmp', got '%s'", val)
	}

	// Custom valid directory
	SetDownloadDir("/var/custom-ctop")
	if val := GetDownloadDir(); val != "/var/custom-ctop" {
		t.Fatalf("expected GetDownloadDir() to return '/var/custom-ctop', got '%s'", val)
	}
}

func TestConfigUpdate_ConcurrentRegistration(t *testing.T) {
	Init()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			key := fmt.Sprintf("dynamicParam_%d", idx%5)
			Update(key, fmt.Sprintf("val_%d", idx))
		}(i)
	}
	wg.Wait()

	// Verify each key was set and exists uniquely
	for i := 0; i < 5; i++ {
		key := fmt.Sprintf("dynamicParam_%d", i)
		count := 0
		lock.RLock()
		for _, p := range GlobalParams {
			if p.Key == key {
				count++
			}
		}
		lock.RUnlock()
		if count != 1 {
			t.Fatalf("expected key %s to appear exactly once in GlobalParams, appeared %d times", key, count)
		}
		if val := GetVal(key); val == "" {
			t.Fatalf("expected non-empty value for key %s", key)
		}
	}
}
