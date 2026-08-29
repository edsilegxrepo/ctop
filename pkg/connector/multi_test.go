package connector

import (
	"testing"
)

func TestParseHostSpec(t *testing.T) {
	tests := []struct {
		input       string
		expectedID  string
		expectedEnd string
	}{
		{"local", "local", ""},
		{"ssh://user@prod-server1:2222", "user@prod-server1", "ssh://user@prod-server1:2222"},
		{"tcp://192.168.1.50:2375", "192.168.1.50:2375", "tcp://192.168.1.50:2375"},
		{"unix:///var/run/docker.sock", "/var/run/docker.sock", "unix:///var/run/docker.sock"},
	}

	for _, tt := range tests {
		_, hostID := ParseHostSpec(tt.input)
		if hostID != tt.expectedID {
			t.Errorf("input '%s': expected hostID '%s', got '%s'", tt.input, tt.expectedID, hostID)
		}
	}
}

func TestMultiConnectorAggregation(t *testing.T) {
	mc := NewMultiConnector()

	// Mock connector 1
	mock1, err := NewMock()
	if err != nil {
		t.Fatalf("failed to create mock1: %v", err)
	}
	for _, c := range mock1.All() {
		c.SetHost("host1")
	}

	// Mock connector 2
	mock2, err := NewMock()
	if err != nil {
		t.Fatalf("failed to create mock2: %v", err)
	}
	for _, c := range mock2.All() {
		c.SetHost("host2")
	}

	mc.AddConnector("host1", mock1)
	mc.AddConnector("host2", mock2)

	all := mc.All()
	if len(all) != len(mock1.All())+len(mock2.All()) {
		t.Fatalf("expected combined length %d, got %d", len(mock1.All())+len(mock2.All()), len(all))
	}
}
