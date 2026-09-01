// util.go provides binary byte formatting utilities converting raw byte counts into human-readable strings (KiB, MiB, GiB).
//
// Objective:
//
//	Deliver high-performance, allocation-efficient byte string formatting for terminal gauges, columns, and logs.
//
// Functionality:
//   - Formats 32-bit and 64-bit byte counters into standard (1.5MiB) or compact/short (2M) notation.
//   - Supports byte scales from Bytes (B) through Pebibytes (PiB).
package cwidgets

import (
	"strconv"
)

const (
	// byte ratio constants
	_           = iota
	kib float64 = 1 << (10 * iota)
	mib
	gib
	tib
	pib
)

var (
	units = []float64{
		1,
		kib,
		mib,
		gib,
		tib,
		pib,
	}

	// short, full unit labels
	labels = [][2]string{
		{"B", "B"},
		{"K", "KiB"},
		{"M", "MiB"},
		{"G", "GiB"},
		{"T", "TiB"},
		{"P", "PiB"},
	}
)

// convenience methods
func ByteFormat(n int) string          { return byteFormat(float64(n), false) }
func ByteFormatShort(n int) string     { return byteFormat(float64(n), true) }
func ByteFormat64(n int64) string      { return byteFormat(float64(n), false) }
func ByteFormat64Short(n int64) string { return byteFormat(float64(n), true) }

func byteFormat(n float64, short bool) string {
	i := len(units) - 1

	for i > 0 {
		if n >= units[i] {
			n /= units[i]
			break
		}
		i--
	}

	if short {
		return unpadFloat(n, 0) + labels[i][0]
	}
	return unpadFloat(n, 2) + labels[i][1]
}

func unpadFloat(f float64, maxp int) string {
	return strconv.FormatFloat(f, 'f', getPrecision(f, maxp), 64)
}

func getPrecision(f float64, maxp int) int {
	frac := int((f - float64(int(f))) * 100)
	if frac == 0 || maxp == 0 {
		return 0
	}
	if frac%10 == 0 || maxp < 2 {
		return 1
	}
	return maxp
}
