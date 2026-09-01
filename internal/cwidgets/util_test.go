// util_test.go validates binary byte formatting across full and short precision modes.
//
// Objective:
//
//	Verify exact string outputs for byte boundaries (0B, bytes, KiB, MiB, GiB, TiB).
//
// Test Strategy:
//   - Table-driven unit test verifying byteFormat64 and byteFormat64Short across exact boundaries and fractions.
package cwidgets

import (
	"testing"
)

func TestByteFormat(t *testing.T) {
	cases := []struct {
		bytes    int64
		expected string
		short    string
	}{
		{0, "0B", "0B"},
		{500, "500B", "500B"},
		{1024, "1KiB", "1K"},
		{1536, "1.5KiB", "2K"},
		{1048576, "1MiB", "1M"},
		{1073741824, "1GiB", "1G"},
		{1099511627776, "1TiB", "1T"},
	}

	for _, c := range cases {
		full := ByteFormat64(c.bytes)
		if full != c.expected {
			t.Errorf("ByteFormat64(%d): expected %s, got %s", c.bytes, c.expected, full)
		}
		short := ByteFormat64Short(c.bytes)
		if short != c.short {
			t.Errorf("ByteFormat64Short(%d): expected %s, got %s", c.bytes, c.short, short)
		}
	}
}
