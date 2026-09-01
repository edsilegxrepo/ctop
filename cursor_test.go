// cursor_test.go validates container list navigation, selection bounds clamping, page calculation, and list resets.
// Test Strategy: Unit tests against mock container fixtures verifying index calculation, boundary checks, and thread-safe refreshes.
package main

import (
	"testing"
	"time"

	"github.com/edsilegx/ctop/internal/cwidgets/compact"
	"github.com/edsilegx/ctop/pkg/connector"
	"github.com/edsilegx/ctop/pkg/connector/collector"
	"github.com/edsilegx/ctop/pkg/container"
	"github.com/edsilegx/ctop/pkg/models"
)

type mockCursorCollector struct{}

func (m *mockCursorCollector) Start()                       {}
func (m *mockCursorCollector) Stop()                        {}
func (m *mockCursorCollector) Running() bool                { return true }
func (m *mockCursorCollector) Stream() chan models.Metrics  { return make(chan models.Metrics) }
func (m *mockCursorCollector) Logs() collector.LogCollector { return nil }

type mockCursorManager struct{}

func (m *mockCursorManager) Start() error            { return nil }
func (m *mockCursorManager) Stop() error             { return nil }
func (m *mockCursorManager) Remove() error           { return nil }
func (m *mockCursorManager) Pause() error            { return nil }
func (m *mockCursorManager) Unpause() error          { return nil }
func (m *mockCursorManager) Restart() error          { return nil }
func (m *mockCursorManager) Exec(cmd []string) error { return nil }
func (m *mockCursorManager) Kill(sig string) error   { return nil }
func (m *mockCursorManager) Top(args string) (models.TopResult, error) {
	return models.TopResult{}, nil
}
func (m *mockCursorManager) Changes() ([]models.Change, error) { return nil, nil }
func (m *mockCursorManager) ReadDir(path string) ([]models.FileInfo, error) {
	return []models.FileInfo{
		{Name: "app", Path: "/app", IsDir: true, Mode: "drwxr-xr-x"},
		{Name: "server", Path: "/app/server", IsDir: false, Size: 1024, Mode: "-rwxr-xr-x"},
	}, nil
}

func (m *mockCursorManager) ReadFile(path string, maxBytes int64) (string, error) {
	return "sample file content", nil
}

func (m *mockCursorManager) SearchFiles(basePath, pattern string, maxResults int) ([]models.FileInfo, error) {
	return []models.FileInfo{
		{Name: "server", Path: "/app/server", IsDir: false, Size: 1024, Mode: "-rwxr-xr-x"},
	}, nil
}

func (m *mockCursorManager) Download(srcPath, dstPath string) (int64, error) {
	return 1024, nil
}

func (m *mockCursorManager) Upload(srcPath, dstPath string) error {
	return nil
}

func (m *mockCursorManager) UpdateResources(memoryMB int64, cpus float64, restartPolicy string) error {
	return nil
}

func createMockContainers(count int) container.Containers {
	var list container.Containers
	for i := 0; i < count; i++ {
		cid := "cid-" + string(rune('A'+i))
		c := container.New(cid, &mockCursorCollector{}, &mockCursorManager{})
		c.SetMeta("name", "container-"+string(rune('A'+i)))
		c.Display = true
		list = append(list, c)
	}
	return list
}

func TestGridCursorNavigation(t *testing.T) {
	containers := createMockContainers(5)

	gc := &GridCursor{
		filtered: containers,
	}
	gc.Reset()

	if gc.Len() != 5 {
		t.Fatalf("expected len 5, got %d", gc.Len())
	}

	if gc.Idx() != 0 {
		t.Fatalf("expected initial index 0, got %d", gc.Idx())
	}

	if sel := gc.Selected(); sel == nil || sel.Id != "cid-A" {
		t.Fatalf("expected selected cid-A, got %v", sel)
	}

	// Move down
	gc.Down()
	if gc.Idx() != 1 {
		t.Fatalf("expected index 1 after Down(), got %d", gc.Idx())
	}
	if sel := gc.Selected(); sel == nil || sel.Id != "cid-B" {
		t.Fatalf("expected selected cid-B, got %v", sel)
	}

	// Move down to bottom
	gc.Down()
	gc.Down()
	gc.Down()
	if gc.Idx() != 4 {
		t.Fatalf("expected index 4 at bottom, got %d", gc.Idx())
	}

	// Moving down past bottom should stay at bottom
	gc.Down()
	if gc.Idx() != 4 {
		t.Fatalf("expected index 4 when moving down past bottom, got %d", gc.Idx())
	}

	// Move up
	gc.Up()
	if gc.Idx() != 3 {
		t.Fatalf("expected index 3 after Up(), got %d", gc.Idx())
	}

	// Move up to top
	gc.Up()
	gc.Up()
	gc.Up()
	if gc.Idx() != 0 {
		t.Fatalf("expected index 0 at top, got %d", gc.Idx())
	}

	// Moving up past top should stay at 0
	gc.Up()
	if gc.Idx() != 0 {
		t.Fatalf("expected index 0 when moving up past top, got %d", gc.Idx())
	}
}

func TestGridCursorPgNavigation(t *testing.T) {
	containers := createMockContainers(10)

	gc := &GridCursor{
		filtered: containers,
	}
	gc.Reset()

	gc.PgDown()
	if gc.Idx() < 1 {
		t.Fatalf("expected index to advance on PgDown, got %d", gc.Idx())
	}

	gc.PgUp()
	if gc.Idx() != 0 {
		t.Fatalf("expected index 0 on PgUp, got %d", gc.Idx())
	}
}

func TestGridCursorEmpty(t *testing.T) {
	gc := &GridCursor{
		filtered: container.Containers{},
	}
	gc.Reset()

	if gc.Len() != 0 {
		t.Fatalf("expected len 0, got %d", gc.Len())
	}
	if sel := gc.Selected(); sel != nil {
		t.Fatalf("expected nil selected container on empty cursor, got %v", sel)
	}
	if idx := gc.Idx(); idx != 0 {
		t.Fatalf("expected idx 0 on empty cursor, got %d", idx)
	}

	// Navigation should not panic on empty cursor
	gc.Up()
	gc.Down()
	gc.PgUp()
	gc.PgDown()
}

func TestGridCursorPgCount(t *testing.T) {
	containers := createMockContainers(25)
	gc := &GridCursor{
		filtered: containers,
	}

	// When cGrid is nil, default pgCount is 1
	if count := gc.pgCount(); count != 1 {
		t.Fatalf("expected pgCount 1 when cGrid is nil, got %d", count)
	}
}

func TestGridCursorSelectedNotFound(t *testing.T) {
	containers := createMockContainers(3)
	gc := &GridCursor{
		filtered:   containers,
		selectedID: "non-existent-cid",
	}

	// Idx() should reset and return 0
	idx := gc.Idx()
	if idx != 0 {
		t.Fatalf("expected Idx() to reset to 0 for non-existent ID, got %d", idx)
	}
	if gc.selectedID != "cid-A" {
		t.Fatalf("expected selectedID to reset to 'cid-A', got '%s'", gc.selectedID)
	}
}

func TestGridCursorRefreshAndScroll(t *testing.T) {
	super := connector.NewConnectorSuper(connector.NewMock)
	time.Sleep(50 * time.Millisecond)
	_, _ = super.Get()

	gc := &GridCursor{cSuper: super}
	_, _ = gc.RefreshContainers()

	// Test ScrollPage with mock grid
	cGrid = compact.NewCompactGrid()
	cGrid.SetWidth(100)
	gc.filtered = createMockContainers(30)
	gc.selectedID = "cid-Z"
	gc.ScrollPage()

	count := gc.pgCount()
	if count <= 0 {
		t.Fatalf("expected positive pgCount, got %d", count)
	}

	cGrid = nil // reset
}
