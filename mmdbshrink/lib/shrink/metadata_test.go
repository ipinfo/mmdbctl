package shrink

import (
	"strings"
	"testing"
)

// TestMetadataSizeEncodingBoundaries is the regression test for the writeCtrl
// off-by-one at the size-29/size-30 boundary. The size-29 encoding covers
// payload lengths 29..284 (29 + one byte 0..255). The boundary constant was
// `28+255` (=283), which mis-routed a length of exactly 284 into the size-30
// branch where it encoded as uint16(284-285) = 65535 — declaring a 65820-byte
// value that every reader rejects. The fix is the boundary constant `29+255`
// (=284).
//
// We round-trip a map of string values whose lengths straddle every size-class
// boundary and assert exact equality. Length 284 fails to round-trip on the
// unfixed bound (decode reads a 65820-byte string and overruns the buffer) and
// passes after the fix.
func TestMetadataSizeEncodingBoundaries(t *testing.T) {
	lengths := []int{
		28,    // last literal (size 0..28)
		29,    // first size-29
		283,   // size-29 (the old, wrong boundary)
		284,   // size-29 boundary (the bug: must NOT spill into size-30)
		285,   // first size-30
		65820, // last size-30 (285 + 65535)
		65821, // first size-31
	}

	m := make(map[string]any, len(lengths))
	for _, n := range lengths {
		key := "k" + itoa(n)
		m[key] = strings.Repeat("a", n)
	}

	enc, err := encodeMetadata(m)
	if err != nil {
		t.Fatalf("encodeMetadata: %v", err)
	}
	dec, err := decodeMetadata(enc)
	if err != nil {
		t.Fatalf("decodeMetadata: %v", err)
	}

	if len(dec) != len(m) {
		t.Fatalf("decoded map has %d entries, want %d", len(dec), len(m))
	}
	for _, n := range lengths {
		key := "k" + itoa(n)
		got, ok := dec[key].(string)
		if !ok {
			t.Errorf("key %q missing or wrong type (%T)", key, dec[key])
			continue
		}
		if len(got) != n {
			t.Errorf("value for %q: decoded length %d, want %d", key, len(got), n)
			continue
		}
		if got != m[key].(string) {
			t.Errorf("value for %q did not round-trip exactly", key)
		}
	}
}

// TestMetadataMapAndArrayCountBoundary round-trips a map and an array whose
// ENTRY/ELEMENT counts sit on the size-29 boundary (284), since writeCtrl is
// shared by string length and map/array count encoding.
func TestMetadataMapAndArrayCountBoundary(t *testing.T) {
	for _, count := range []int{283, 284, 285} {
		inner := make(map[string]any, count)
		arr := make([]any, count)
		for i := 0; i < count; i++ {
			k := "e" + itoa(i)
			inner[k] = uint16(i % 65536)
			arr[i] = "x" + itoa(i)
		}
		top := map[string]any{
			"the_map":   inner,
			"the_array": arr,
		}
		enc, err := encodeMetadata(top)
		if err != nil {
			t.Fatalf("count=%d encodeMetadata: %v", count, err)
		}
		dec, err := decodeMetadata(enc)
		if err != nil {
			t.Fatalf("count=%d decodeMetadata: %v", count, err)
		}
		gotMap, ok := dec["the_map"].(map[string]any)
		if !ok {
			t.Fatalf("count=%d: the_map missing or wrong type (%T)", count, dec["the_map"])
		}
		if len(gotMap) != count {
			t.Errorf("count=%d: decoded map has %d entries, want %d", count, len(gotMap), count)
		}
		gotArr, ok := dec["the_array"].([]any)
		if !ok {
			t.Fatalf("count=%d: the_array missing or wrong type (%T)", count, dec["the_array"])
		}
		if len(gotArr) != count {
			t.Errorf("count=%d: decoded array has %d elements, want %d", count, len(gotArr), count)
		}
		for i := 0; i < count; i++ {
			if got := gotArr[i].(string); got != "x"+itoa(i) {
				t.Errorf("count=%d: array[%d] = %q, want %q", count, i, got, "x"+itoa(i))
			}
		}
	}
}

// itoa is a tiny base-10 formatter to avoid importing strconv just for tests.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// TestMetadataDecodesPointers covers pointers (kind 1) inside the metadata
// section. The upstream mmdbwriter emits them by default whenever a value is
// repeated, for example a language code that appears both as a description
// key and a languages element, so rejecting them makes shrink fail on any
// database it or `mmdbctl import` produced. Pointer targets are relative to
// the start of the metadata section.
func TestMetadataDecodesPointers(t *testing.T) {
	// map(2) { "a": "en", "b": ptr -> the "en" string }
	//
	// offset 0: 0xE2      map, 2 entries
	// offset 1: 0x41 'a'  key "a"
	// offset 3: 0x42 'e' 'n'   value "en"   <- pointer target (offset 3)
	// offset 6: 0x41 'b'  key "b"
	// offset 8: 0x20 0x03 class-0 pointer to offset 3
	buf := []byte{
		0xE2,
		0x41, 'a',
		0x42, 'e', 'n',
		0x41, 'b',
		0x20, 0x03,
	}
	m, err := decodeMetadata(buf)
	if err != nil {
		t.Fatalf("decodeMetadata: %v", err)
	}
	if got := m["a"]; got != "en" {
		t.Errorf(`m["a"] = %#v, want "en"`, got)
	}
	if got := m["b"]; got != "en" {
		t.Errorf(`m["b"] = %#v, want "en" (via pointer)`, got)
	}

	// Re-encoding resolves the pointer into a plain value and round-trips.
	enc, err := encodeMetadata(m)
	if err != nil {
		t.Fatalf("encodeMetadata: %v", err)
	}
	dec, err := decodeMetadata(enc)
	if err != nil {
		t.Fatalf("decodeMetadata(re-encoded): %v", err)
	}
	if dec["a"] != "en" || dec["b"] != "en" {
		t.Errorf("re-encoded metadata = %v, want a=en b=en", dec)
	}

	// A pointer to a pointer is invalid per the spec and must be rejected,
	// not followed.
	bad := []byte{
		0xE1,
		0x41, 'a',
		0x20, 0x05, // offset 3: pointer -> offset 5
		0x20, 0x03, // offset 5: pointer -> offset 3
	}
	if _, err := decodeMetadata(bad); err == nil {
		t.Error("expected error for pointer-to-pointer metadata, got nil")
	}
}
