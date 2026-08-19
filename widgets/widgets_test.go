// widgets_test.go validates CTopHeader, StatusLine, ErrorView, InputWidget, and TextView presentation controls.
// Test Strategy: Verifies text formatting, cursor typing, backspace editing, and buffer drawing without terminal deadlocks.
package widgets

import (
	"image"
	"testing"
	"time"

	ui "github.com/gizak/termui/v3"
)

func TestCTopHeader(t *testing.T) {
	h := NewCTopHeader()
	if h == nil {
		t.Fatal("expected non-nil header")
	}

	h.SetCount(15)
	if h.CountText != "15 containers" {
		t.Fatalf("expected '15 containers', got '%s'", h.CountText)
	}

	h.SetFilter("web")
	if h.FilterText != "filter: web" {
		t.Fatalf("expected 'filter: web', got '%s'", h.FilterText)
	}

	h.SetFilter("")
	if h.FilterText != "" {
		t.Fatalf("expected empty FilterText, got '%s'", h.FilterText)
	}

	if height := h.Height(); height != 1 {
		t.Fatalf("expected Height 1, got %d", height)
	}

	buf := ui.NewBuffer(image.Rect(0, 0, 100, 5))
	h.SetRect(0, 0, 80, 1)
	h.Draw(buf)
}

func TestStatusLine(t *testing.T) {
	sl := NewStatusLine()
	if sl == nil {
		t.Fatal("expected non-nil status line")
	}

	sl.Show("Connected to docker")
	if sl.Message != "Connected to docker" {
		t.Fatalf("expected 'Connected to docker', got '%s'", sl.Message)
	}
	if sl.isErr {
		t.Fatal("expected isErr to be false for Show()")
	}

	sl.ShowErr("Connection failed")
	if sl.Message != "Connection failed" {
		t.Fatalf("expected 'Connection failed', got '%s'", sl.Message)
	}
	if !sl.isErr {
		t.Fatal("expected isErr to be true for ShowErr()")
	}

	buf := ui.NewBuffer(image.Rect(0, 0, 100, 5))
	sl.SetRect(0, 0, 80, 1)
	sl.Draw(buf)
}

func TestErrorView(t *testing.T) {
	ev := NewErrorView()
	if ev == nil {
		t.Fatal("expected non-nil error view")
	}

	ev.Append("First error message")
	ev.Append("Second error message")

	if len(ev.lines) != 4 { // 2 messages + 2 empty spacer lines
		t.Fatalf("expected 4 lines in error view, got %d", len(ev.lines))
	}

	buf := ui.NewBuffer(image.Rect(0, 0, 80, 20))
	ev.SetRect(0, 0, 80, 20)
	ev.Draw(buf)
}

func TestInputWidget(t *testing.T) {
	input := NewInput()
	if input == nil {
		t.Fatal("expected non-nil input")
	}

	ch := input.Stream()
	done := make(chan bool)
	go func() {
		for range ch {
		}
		done <- true
	}()

	// Type characters
	input.KeyPress("a")
	input.KeyPress("b")
	input.KeyPress("c")

	if input.Data != "abc" {
		t.Fatalf("expected Data 'abc', got '%s'", input.Data)
	}

	input.KeyPress("<Backspace>")
	if input.Data != "ab" {
		t.Fatalf("expected Data 'ab' after backspace, got '%s'", input.Data)
	}

	// Backspace on empty data
	input.KeyPress("<Backspace>")
	input.KeyPress("<Backspace>")
	input.KeyPress("<Backspace>")
	if input.Data != "" {
		t.Fatalf("expected empty data, got '%s'", input.Data)
	}

	// Invalid characters (ignored)
	input.KeyPress("<Enter>")
	input.KeyPress("<Space>")
	if input.Data != "" {
		t.Fatalf("expected unchanged data, got '%s'", input.Data)
	}

	buf := ui.NewBuffer(image.Rect(0, 0, 50, 5))
	input.SetRect(0, 0, 40, 3)
	input.Draw(buf)
}

func TestTextViewControls(t *testing.T) {
	linesCh := make(chan ToggleText, 10)
	tv := NewTextView(linesCh)

	linesCh <- &testToggleText{text: "line 1"}
	linesCh <- &testToggleText{text: "line 2"}
	time.Sleep(50 * time.Millisecond)

	// Pause and resume
	tv.Pause()
	if !tv.IsPaused() {
		t.Fatal("expected IsPaused() to be true")
	}
	tv.Resume()
	if tv.IsPaused() {
		t.Fatal("expected IsPaused() to be false")
	}

	// Filter
	tv.SetFilter("line 1")
	if filter := tv.Filter(); filter != "line 1" {
		t.Fatalf("expected filter 'line 1', got '%s'", filter)
	}

	// Toggle
	tv.Toggle()
	tv.Resize()

	buf := ui.NewBuffer(image.Rect(0, 0, 100, 30))
	tv.SetRect(0, 0, 80, 20)
	tv.Draw(buf)

	// Empty text view draw
	emptyCh := make(chan ToggleText)
	emptyTv := NewTextView(emptyCh)
	emptyTv.SetFilter("non-matching")
	emptyTv.SetRect(0, 0, 80, 20)
	emptyTv.Draw(buf)
	close(linesCh)
	close(emptyCh)
}
