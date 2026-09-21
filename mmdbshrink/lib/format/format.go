// Package format renders numbers for human-readable output.
//
// It exists so the lib packages and the CLI produce identical strings without
// either depending on the other.
package format

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Int formats an integer with comma thousands separators: "1,234,567".
func Int(v uint64) string {
	s := strconv.FormatUint(v, 10)
	if len(s) <= 3 {
		return s
	}

	var b strings.Builder
	pre := len(s) % 3
	if pre > 0 {
		b.WriteString(s[:pre])
		b.WriteByte(',')
	}
	for i := pre; i < len(s); i += 3 {
		b.WriteString(s[i : i+3])
		if i+3 < len(s) {
			b.WriteByte(',')
		}
	}
	return b.String()
}

// Bytes renders a byte count with IEC (1024-based) units: "1.21 MiB".
func Bytes(v uint64) string {
	const (
		KiB = 1024
		MiB = 1024 * KiB
		GiB = 1024 * MiB
	)
	switch {
	case v >= GiB:
		return fmt.Sprintf("%.2f GiB", float64(v)/float64(GiB))
	case v >= MiB:
		return fmt.Sprintf("%.2f MiB", float64(v)/float64(MiB))
	case v >= KiB:
		return fmt.Sprintf("%.2f KiB", float64(v)/float64(KiB))
	}
	return fmt.Sprintf("%d B", v)
}

// DecimalBytes renders a byte count with SI (1000-based) units: "1.23 MB".
func DecimalBytes(v uint64) string {
	const unit = 1000
	if v < unit {
		return fmt.Sprintf("%d B", v)
	}

	div, exp := uint64(unit), 0
	for n := v / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %cB", float64(v)/float64(div), "KMGTPE"[exp])
}

// PctDelta returns "(-65.78%)", "(+5.40%)", or "(unchanged)".
func PctDelta(diff, base int64) string {
	if diff == 0 || base == 0 {
		return "(unchanged)"
	}
	pct := 100.0 * float64(diff) / float64(base)
	if diff < 0 {
		return fmt.Sprintf("(%.2f%%)", pct)
	}
	return fmt.Sprintf("(+%.2f%%)", pct)
}

// SignedPct formats an already-computed percentage with an explicit sign
// and one decimal, "+5.4%" or "-65.8%", and "—" for NaN.
func SignedPct(v float64) string {
	if math.IsNaN(v) {
		return "—"
	}
	if v >= 0 {
		return fmt.Sprintf("+%.1f%%", v)
	}
	return fmt.Sprintf("%.1f%%", v)
}

// SizeDelta returns "(saved 19.12 MB, -65.76%)" when output is
// smaller, "(grew 2.13 MB, +5.40%)" when larger, or "(unchanged)".
func SizeDelta(in, out int64) string {
	diff := out - in
	if diff == 0 {
		return "(unchanged)"
	}
	pct := 100.0 * float64(diff) / float64(in)
	if diff < 0 {
		return fmt.Sprintf("(saved %s, %.2f%%)", DecimalBytes(uint64(-diff)), pct)
	}
	return fmt.Sprintf("(grew %s, +%.2f%%)", DecimalBytes(uint64(diff)), pct)
}
