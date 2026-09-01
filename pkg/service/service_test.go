package service

import (
	"strings"
	"testing"
)

func TestGenerateSystemdUnit(t *testing.T) {
	unit := GenerateSystemdUnit("/usr/bin/ctop")
	if !strings.Contains(unit, "ExecStart=/usr/bin/ctop --headless --web :9090 --web-auth-token auto") {
		t.Errorf("expected ExecStart in unit, got:\n%s", unit)
	}
	if !strings.Contains(unit, "Requires=docker.service") {
		t.Errorf("expected Requires=docker.service, got:\n%s", unit)
	}

	// Default fallback
	unitDefault := GenerateSystemdUnit("")
	if !strings.Contains(unitDefault, "ExecStart=/usr/local/bin/ctop --headless --web :9090 --web-auth-token auto") {
		t.Errorf("expected default ExecStart in unit, got:\n%s", unitDefault)
	}
}

func TestRunServiceCommands(t *testing.T) {
	if err := Run([]string{}); err == nil {
		t.Error("expected error for empty args")
	}

	if err := Run([]string{"unknown_cmd"}); err == nil {
		t.Error("expected error for unknown subcommand")
	}

	if err := Run([]string{"generate"}); err != nil {
		t.Errorf("unexpected error on generate: %v", err)
	}

	if err := Run([]string{"status"}); err != nil {
		t.Errorf("unexpected error on status: %v", err)
	}
}
