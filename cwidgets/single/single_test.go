package single

import (
	"testing"
)

func TestMkInfoRows(t *testing.T) {
	rows := mkInfoRows("ports", "80/tcp\n443/tcp")
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows for multiline ports, got %d", len(rows))
	}
	if rows[0][0] != "ports" || rows[0][1] != "80/tcp" {
		t.Errorf("unexpected first row: %v", rows[0])
	}
	if rows[1][0] != "" || rows[1][1] != "443/tcp" {
		t.Errorf("unexpected second row: %v", rows[1])
	}
}

func TestToFloat64Slice(t *testing.T) {
	input := []int{10, -5, 20, 0}
	res := toFloat64Slice(input)
	if len(res) != 4 {
		t.Fatalf("expected length 4, got %d", len(res))
	}
	if res[0] != 10.0 || res[1] != 0.0 || res[2] != 20.0 || res[3] != 0.0 {
		t.Errorf("unexpected result: %v", res)
	}
}

func TestNetUpdate(t *testing.T) {
	net := NewNet()
	net.Update(1000, 2000)
	net.Update(3000, 5000)

	if len(net.rxHist.Data) != 60 {
		t.Errorf("expected 60 points in rxHist, got %d", len(net.rxHist.Data))
	}
	if net.rxHist.Val != 2000 {
		t.Errorf("expected rx rate 2000, got %d", net.rxHist.Val)
	}
	if net.txHist.Val != 3000 {
		t.Errorf("expected tx rate 3000, got %d", net.txHist.Val)
	}
	if net.rxTitle != "RX [1.95kib/s]" {
		t.Errorf("unexpected rxTitle: %s", net.rxTitle)
	}
}

func TestIOUpdate(t *testing.T) {
	io := NewIO()
	io.Update(500, 1000)
	io.Update(1500, 3000)

	if len(io.readHist.Data) != 60 {
		t.Errorf("expected 60 points in readHist, got %d", len(io.readHist.Data))
	}
	if io.readHist.Val != 1000 {
		t.Errorf("expected read rate 1000, got %d", io.readHist.Val)
	}
	if io.writeHist.Val != 2000 {
		t.Errorf("expected write rate 2000, got %d", io.writeHist.Val)
	}
	if io.readTitle != "READ [1000b/s]" {
		t.Errorf("unexpected readTitle: %s", io.readTitle)
	}
}
