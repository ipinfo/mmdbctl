package shrink

import "testing"

// TestNodePairRoundTrip round-trips writeNodePair -> readNodePair for every
// supported record_size across boundary values, with particular attention to
// the 28-bit nibble packing in byte b[3] (the high nibble of left and the low
// nibble of right share that byte). It also asserts that out-of-range values
// are rejected as overflow.
func TestNodePairRoundTrip(t *testing.T) {
	cases := []struct {
		recordSize uint64
		max        uint32 // max representable value per field
	}{
		{24, 0xFFFFFF},
		{28, 0x0FFFFFFF},
		{32, 0xFFFFFFFF},
	}

	for _, c := range cases {
		// Values to round-trip. Include 0, 1, the field max, and (for 28-bit)
		// values that set the high nibble routed through b[3].
		vals := []uint32{0, 1, c.max, c.max - 1}
		if c.recordSize == 28 {
			// High nibble of the 28-bit field (bits 24..27) lives in b[3].
			vals = append(vals, 0x0F000000, 0x01000000, 0x0FFFFFFF, 0x00FFFFFF)
		}
		if c.recordSize == 32 {
			vals = append(vals, 0x80000000, 0x7FFFFFFF)
		}

		nodeBytes := uint64(c.recordSize) / 4
		for _, left := range vals {
			for _, right := range vals {
				buf := make([]byte, nodeBytes)
				if err := writeNodePair(buf, 0, c.recordSize, left, right); err != nil {
					t.Fatalf("rs=%d writeNodePair(l=%#x r=%#x): %v", c.recordSize, left, right, err)
				}
				gotL, gotR, err := readNodePair(buf, 0, c.recordSize)
				if err != nil {
					t.Fatalf("rs=%d readNodePair: %v", c.recordSize, err)
				}
				if gotL != left || gotR != right {
					t.Errorf("rs=%d round-trip: got (l=%#x r=%#x), want (l=%#x r=%#x)",
						c.recordSize, gotL, gotR, left, right)
				}
			}
		}
	}
}

// TestNodePairOverflowRejected confirms writeNodePair rejects values that don't
// fit the field width for 24- and 28-bit records (32-bit has no overflow).
func TestNodePairOverflowRejected(t *testing.T) {
	cases := []struct {
		recordSize uint64
		overflow   uint32 // one bit past the field max
	}{
		{24, 0x1000000},  // 2^24
		{28, 0x10000000}, // 2^28
	}
	for _, c := range cases {
		nodeBytes := uint64(c.recordSize) / 4
		// Overflow in left.
		buf := make([]byte, nodeBytes)
		if err := writeNodePair(buf, 0, c.recordSize, c.overflow, 0); err == nil {
			t.Errorf("rs=%d: expected overflow error for left=%#x, got nil", c.recordSize, c.overflow)
		}
		// Overflow in right.
		buf = make([]byte, nodeBytes)
		if err := writeNodePair(buf, 0, c.recordSize, 0, c.overflow); err == nil {
			t.Errorf("rs=%d: expected overflow error for right=%#x, got nil", c.recordSize, c.overflow)
		}
	}
}
