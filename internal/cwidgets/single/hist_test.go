package single

import (
	"testing"
)

func TestIntHist(t *testing.T) {
	h := NewIntHist(3)
	if len(h.Data) != 3 {
		t.Fatalf("expected initial data length 3, got %d", len(h.Data))
	}

	h.Append(10)
	h.Append(20)
	h.Append(30)
	if h.Val != 30 {
		t.Errorf("expected Val=30, got %d", h.Val)
	}

	h.Append(40)
	if h.Val != 40 {
		t.Errorf("expected Val=40, got %d", h.Val)
	}
	if len(h.Data) != 3 {
		t.Fatalf("expected capped length 3, got %d", len(h.Data))
	}
	if h.Data[0] != 20 || h.Data[1] != 30 || h.Data[2] != 40 {
		t.Errorf("unexpected sliding window data: %v", h.Data)
	}
}

func TestFloatHist(t *testing.T) {
	h := NewFloatHist(3)
	h.Append(1.5)
	h.Append(2.5)
	h.Append(3.5)
	h.Append(4.5)

	if h.Val != 4.5 {
		t.Errorf("expected Val=4.5, got %f", h.Val)
	}
	if len(h.Data) != 3 {
		t.Fatalf("expected capped length 3, got %d", len(h.Data))
	}
	if h.Data[0] != 2.5 || h.Data[1] != 3.5 || h.Data[2] != 4.5 {
		t.Errorf("unexpected sliding window data: %v", h.Data)
	}
}

func TestDiffHist(t *testing.T) {
	h := NewDiffHist(3)
	// Initial update sets lastVal without appending
	h.Append(100)
	if h.lastVal != 100 {
		t.Errorf("expected lastVal=100, got %d", h.lastVal)
	}

	h.Append(150)
	if h.Val != 50 {
		t.Errorf("expected diff=50, got %d", h.Val)
	}
}
