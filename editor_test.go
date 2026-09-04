package main

import (
	"os"
	"testing"
)

func TestEditContainerFileValidation(t *testing.T) {
	// 1. Nil container
	if _, err := EditContainerFile(nil, "/app/config.json"); err == nil {
		t.Fatal("expected error for nil container")
	}

	// 2. Empty path
	mockContainers := createMockContainers(1)
	c := mockContainers[0]
	if _, err := EditContainerFile(c, ""); err == nil {
		t.Fatal("expected error for empty container path")
	}

	// 3. Simulated non-interactive test editor (using a script that touches/modifies the file)
	tempScript, err := os.CreateTemp("", "test-editor-*.sh")
	if err != nil {
		t.Fatalf("failed to create temp script: %v", err)
	}
	_ = tempScript.Close()
	defer func() { _ = os.Remove(tempScript.Name()) }()
	_ = os.WriteFile(tempScript.Name(), []byte("#!/bin/sh\nfor arg in \"$@\"; do target=\"$arg\"; done\necho 'modified content' >> \"$target\"\n"), 0o755)
	_ = os.Chmod(tempScript.Name(), 0o755)

	oldEditor := os.Getenv("EDITOR")
	_ = os.Setenv("EDITOR", tempScript.Name())
	defer func() { _ = os.Setenv("EDITOR", oldEditor) }()

	mod, err := EditContainerFile(c, "/app/config.json")
	if err != nil {
		t.Fatalf("unexpected edit error: %v", err)
	}
	if !mod {
		t.Fatalf("expected file to be reported as modified")
	}

	// 4. Test editor configured with arguments (e.g. "script.sh --flag")
	_ = os.Setenv("EDITOR", tempScript.Name()+" --flag")
	modArg, err := EditContainerFile(c, "/app/config.json")
	if err != nil {
		t.Fatalf("unexpected edit error with editor arguments: %v", err)
	}
	if !modArg {
		t.Fatalf("expected file to be reported as modified with editor arguments")
	}

	// 5. Unmodified file (editor exits cleanly without modifying file)
	noModScript, err := os.CreateTemp("", "test-no-mod-*.sh")
	if err != nil {
		t.Fatalf("failed to create temp script: %v", err)
	}
	_ = noModScript.Close()
	defer func() { _ = os.Remove(noModScript.Name()) }()
	_ = os.WriteFile(noModScript.Name(), []byte("#!/bin/sh\nexit 0\n"), 0o755)
	_ = os.Chmod(noModScript.Name(), 0o755)

	_ = os.Setenv("EDITOR", noModScript.Name())
	modNone, err := EditContainerFile(c, "/app/config.json")
	if err != nil {
		t.Fatalf("unexpected error on unmodified file: %v", err)
	}
	if modNone {
		t.Fatalf("expected file to be reported as unmodified (false)")
	}

	// 6. Editor failure (non-zero exit status)
	failScript, err := os.CreateTemp("", "test-fail-*.sh")
	if err != nil {
		t.Fatalf("failed to create temp script: %v", err)
	}
	_ = failScript.Close()
	defer func() { _ = os.Remove(failScript.Name()) }()
	_ = os.WriteFile(failScript.Name(), []byte("#!/bin/sh\nexit 2\n"), 0o755)
	_ = os.Chmod(failScript.Name(), 0o755)

	_ = os.Setenv("EDITOR", failScript.Name())
	if _, err := EditContainerFile(c, "/app/config.json"); err == nil {
		t.Fatalf("expected error when editor exits with non-zero status")
	}
}
