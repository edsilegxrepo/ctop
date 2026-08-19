// exit_test.go validates defined exit code constants for correct standard integer mappings.
// Test Strategy: Fast assertions verifying invariant integer values for each exit code constant.
package exit

import (
	"testing"
)

func TestExitCodes(t *testing.T) {
	if ExitSuccess != 0 {
		t.Fatalf("expected ExitSuccess=0, got %d", ExitSuccess)
	}
	if ExitGeneral != 1 {
		t.Fatalf("expected ExitGeneral=1, got %d", ExitGeneral)
	}
	if ExitUsage != 2 {
		t.Fatalf("expected ExitUsage=2, got %d", ExitUsage)
	}
	if ExitConfig != 3 {
		t.Fatalf("expected ExitConfig=3, got %d", ExitConfig)
	}
	if ExitConnector != 4 {
		t.Fatalf("expected ExitConnector=4, got %d", ExitConnector)
	}
	if ExitUI != 5 {
		t.Fatalf("expected ExitUI=5, got %d", ExitUI)
	}
}
