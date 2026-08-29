package prober

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestProbeTCP(t *testing.T) {
	// Start local listener for OPEN test
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer func() { _ = ln.Close() }()

	addr := ln.Addr().String()

	// 1. Success probe
	res := ProbeTCP(context.Background(), "Local Server", addr, 1*time.Second)
	if !res.Success || res.Status != "OPEN" {
		t.Errorf("expected OPEN result for live listener, got: %+v", res)
	}

	// 2. Closed port probe
	resClosed := ProbeTCP(context.Background(), "Closed Port", "127.0.0.1:59999", 50*time.Millisecond)
	if resClosed.Success {
		t.Errorf("expected failure for closed port, got success: %+v", resClosed)
	}
}

func TestExtractProbeTargets(t *testing.T) {
	ports := "0.0.0.0:8080->80/tcp\n:::9090->90/tcp"
	networks := "bridge:::172.17.0.2:::172.17.0.1:::02:42:ac:11:00:02:::172.17.0.0/16"

	targets := ExtractProbeTargets(ports, networks)
	if len(targets) == 0 {
		t.Fatal("expected extracted targets")
	}

	foundHostIPv4 := false
	foundHostIPv6 := false
	for _, target := range targets {
		if target.Target == "127.0.0.1:8080" {
			foundHostIPv4 = true
		}
		if target.Target == "[::1]:9090" || target.Target == "::1:9090" {
			foundHostIPv6 = true
		}
	}

	if !foundHostIPv4 {
		t.Errorf("expected 127.0.0.1:8080 in extracted targets")
	}
	if !foundHostIPv6 {
		t.Errorf("expected ::1:9090 in extracted targets")
	}
}
