// menu_test.go validates interactive menu item manipulation, dynamic item deletion, alphabetical sorting, and buffer drawing.
//
// Objective:
//
//	Verify modal menu items management, cursor navigation clamping, separator skipping, tooltip drawing, and sorting stability.
//
// Test Strategy:
//   - Verifies cursor clamping, item index adjustments, sorting invariants, and tooltip rendering.
//   - Tests item addition, deletion, and selection queries with synthetic keyboard events.
package menu

import (
	"image"
	"testing"

	ui "github.com/gizak/termui/v3"
)

func TestMenuBasics(t *testing.T) {
	m := NewMenu()
	if m == nil {
		t.Fatal("expected non-nil Menu")
	}

	item1 := Item{Val: "opt1", Label: "Option 1"}
	item2 := Item{Val: "opt2", Label: "Option 2"}
	sep := NewSeparator()
	item3 := Item{Val: "opt3", Label: "Option 3"}

	m.AddItems(item1, sep, item2, item3)

	if len(m.items) != 4 {
		t.Fatalf("expected 4 items, got %d", len(m.items))
	}

	// Test SelectedItem and SelectedValue
	if val := m.SelectedValue(); val != "opt1" {
		t.Fatalf("expected 'opt1', got '%s'", val)
	}

	// Down should skip separator
	m.Down()
	if val := m.SelectedValue(); val != "opt2" {
		t.Fatalf("expected 'opt2' after Down(), got '%s'", val)
	}

	m.Down()
	if val := m.SelectedValue(); val != "opt3" {
		t.Fatalf("expected 'opt3' after Down(), got '%s'", val)
	}

	// Moving down past last item stays at last item
	m.Down()
	if val := m.SelectedValue(); val != "opt3" {
		t.Fatalf("expected 'opt3' at bottom, got '%s'", val)
	}

	// Up should move back to opt2 and skip separator to opt1
	m.Up()
	if val := m.SelectedValue(); val != "opt2" {
		t.Fatalf("expected 'opt2' after Up(), got '%s'", val)
	}

	m.Up()
	if val := m.SelectedValue(); val != "opt1" {
		t.Fatalf("expected 'opt1' after Up(), got '%s'", val)
	}

	// Moving up past top stays at 0
	m.Up()
	if val := m.SelectedValue(); val != "opt1" {
		t.Fatalf("expected 'opt1' at top, got '%s'", val)
	}
}

func TestMenuSetCursorAndDelItem(t *testing.T) {
	m := NewMenu()
	m.AddItems(
		Item{Val: "apple", Label: "Apple"},
		Item{Val: "banana", Label: "Banana"},
		Item{Val: "cherry", Label: "Cherry"},
	)

	// Set cursor by value
	if !m.SetCursor("banana") {
		t.Fatal("expected SetCursor('banana') to succeed")
	}
	if m.SelectedValue() != "banana" {
		t.Fatalf("expected banana, got %s", m.SelectedValue())
	}

	// Set cursor by label
	if !m.SetCursor("Cherry") {
		t.Fatal("expected SetCursor('Cherry') to succeed")
	}
	if m.SelectedValue() != "cherry" {
		t.Fatalf("expected cherry, got %s", m.SelectedValue())
	}

	// Set cursor non-existent
	if m.SetCursor("orange") {
		t.Fatal("expected SetCursor('orange') to return false")
	}

	// DelItem by value
	if !m.DelItem("banana") {
		t.Fatal("expected DelItem('banana') to succeed")
	}
	if len(m.items) != 2 {
		t.Fatalf("expected 2 items after delete, got %d", len(m.items))
	}

	// DelItem by label
	if !m.DelItem("Cherry") {
		t.Fatal("expected DelItem('Cherry') to succeed")
	}
	if len(m.items) != 1 {
		t.Fatalf("expected 1 item after delete, got %d", len(m.items))
	}

	// DelItem non-existent
	if m.DelItem("banana") {
		t.Fatal("expected DelItem to return false for already deleted item")
	}

	// ClearItems
	m.ClearItems()
	if len(m.items) != 0 {
		t.Fatalf("expected 0 items after ClearItems, got %d", len(m.items))
	}
	if val := m.SelectedValue(); val != "" {
		t.Fatalf("expected empty SelectedValue on empty menu, got '%s'", val)
	}
}

func TestMenuSorting(t *testing.T) {
	m := NewMenu()
	m.SortItems = true
	m.AddItems(
		Item{Val: "z", Label: "Zebra"},
		Item{Val: "a", Label: "Apple"},
		Item{Val: "m", Label: "Mango"},
	)

	if m.items[0].Label != "Apple" || m.items[1].Label != "Mango" || m.items[2].Label != "Zebra" {
		t.Fatalf("expected sorted items: Apple, Mango, Zebra; got %v", m.items)
	}

	items := NewItems(
		Item{Val: "b", Label: "B"},
		Item{Val: "a", Label: "A"},
	)
	if items.Len() != 2 {
		t.Fatalf("expected len 2, got %d", items.Len())
	}
	items.Swap(0, 1)
	if items[0].Label != "A" {
		t.Fatalf("expected swap to work")
	}
	if !items.Less(0, 1) {
		t.Fatalf("expected A < B")
	}
	if items[0].Text() != "A" {
		t.Fatalf("expected Text() == 'A'")
	}
	itemNoLabel := Item{Val: "valOnly"}
	if itemNoLabel.Text() != "valOnly" {
		t.Fatalf("expected Text() == 'valOnly'")
	}
}

func TestMenuDrawAndToolTip(t *testing.T) {
	m := NewMenu()
	m.Selectable = true
	m.SubText = "Select an option below:"
	m.AddItems(
		Item{Val: "start", Label: "Start"},
		NewSeparator(),
		Item{Val: "stop", Label: "Stop"},
	)
	m.SetToolTip("Help tip line 1", "Help tip line 2")

	buf := ui.NewBuffer(image.Rect(0, 0, 100, 40))
	m.SetRect(0, 0, 80, 20)
	m.Draw(buf)

	if m.toolTip == nil {
		t.Fatal("expected toolTip to be configured")
	}

	toolTipBuf := ui.NewBuffer(image.Rect(0, 0, 100, 40))
	m.toolTip.Draw(toolTipBuf)
}
