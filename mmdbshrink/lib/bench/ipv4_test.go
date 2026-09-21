package bench

import (
	"encoding/binary"
	"testing"
)

// TestMakeIPv4Deterministic pins the property every bench mode relies on: the
// same seed yields the same address sequence, so two runs see identical
// workloads, and different seeds do not.
func TestMakeIPv4Deterministic(t *testing.T) {
	const n = 10_000
	a, b, c := makeIPv4(42), makeIPv4(42), makeIPv4(43)

	differs := false
	for i := 0; i < n; i++ {
		x, y, z := a(), b(), c()
		if x != y {
			t.Fatalf("lookup %d: same seed diverged: %s vs %s", i, x, y)
		}
		if x != z {
			differs = true
		}
	}
	if !differs {
		t.Fatal("seeds 42 and 43 produced identical streams")
	}
}

// TestMakeIPv4Range checks every generated address is a plain IPv4 inside the
// documented half-open range, and that the generator actually reaches both
// ends of it rather than clustering.
func TestMakeIPv4Range(t *testing.T) {
	const n = 1_000_000
	mk := makeIPv4(1)

	minSeen, maxSeen := uint32(ipv4RangeEnd), uint32(0)
	for i := 0; i < n; i++ {
		ip := mk()
		if !ip.Is4() {
			t.Fatalf("lookup %d: not IPv4: %s", i, ip)
		}
		v := binary.BigEndian.Uint32(ip.AsSlice())
		if v < ipv4RangeStart || v >= ipv4RangeEnd {
			t.Fatalf("lookup %d: %s outside [1.0.0.0, 224.0.0.0)", i, ip)
		}
		minSeen = min(minSeen, v)
		maxSeen = max(maxSeen, v)
	}

	// With a million uniform draws over ~3.7 billion values, the extremes
	// should land within the first and last 1% of the range.
	span := uint32(ipv4RangeEnd - ipv4RangeStart)
	if minSeen > ipv4RangeStart+span/100 {
		t.Errorf("min seen %d never approached range start %d", minSeen, ipv4RangeStart)
	}
	if maxSeen < ipv4RangeEnd-span/100 {
		t.Errorf("max seen %d never approached range end %d", maxSeen, ipv4RangeEnd)
	}
}
