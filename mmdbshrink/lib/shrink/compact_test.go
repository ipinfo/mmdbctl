package shrink

import (
	"bytes"
	"log/slog"
	"testing"
)

var testLogger = slog.New(slog.DiscardHandler)

// TestPointerClassBoundaries pins the MMDB pointer (kind 1) class boundaries
// per the spec, so a future edit can't reintroduce the off-by-1024 we just
// fixed in the class-2 ceiling (134744063, NOT 134743039).
func TestPointerClassBoundaries(t *testing.T) {
	cases := []struct {
		name      string
		target    uint32
		wantWidth uint8
	}{
		{"class0 min", 0, 2},
		{"class0 max", 2047, 2},
		{"class1 min", 2048, 3},
		{"class1 max", 526335, 3},
		{"class2 min", 526336, 4},
		// 134744063 = (2^27 - 1) + 526336 — the boundary we got wrong before.
		{"class2 max (boundary)", 134744063, 4},
		{"class3 min above class2", 134744064, 5},
		{"class3 high", 0xFFFFFFFF, 5},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := minWidthForOffset(c.target)
			if got != c.wantWidth {
				t.Errorf("minWidthForOffset(%d) = %d, want %d", c.target, got, c.wantWidth)
			}
		})
	}
}

// TestPointerRoundTrip writes a pointer at the chosen width, then parses it
// back via valueSpan and confirms the decoded target matches. Class boundaries
// are exercised end-to-end.
func TestPointerRoundTrip(t *testing.T) {
	targets := []uint32{
		0, 1, 2047,
		2048, 2049, 526335,
		526336, 526337, 134744063,
		134744064, 0xFFFFFFFF,
	}
	for _, target := range targets {
		w := minWidthForOffset(target)
		buf := make([]byte, w)
		if err := writePointer(buf, target, w); err != nil {
			t.Fatalf("writePointer(%d, w=%d): %v", target, w, err)
		}
		end, ptrs, err := valueSpan(buf, 0)
		if err != nil {
			t.Fatalf("valueSpan(target=%d w=%d): %v", target, w, err)
		}
		if end != uint32(w) {
			t.Errorf("valueSpan(target=%d w=%d): end=%d want %d", target, w, end, w)
		}
		if len(ptrs) != 1 {
			t.Fatalf("valueSpan(target=%d): got %d ptrs, want 1", target, len(ptrs))
		}
		if ptrs[0].target != target {
			t.Errorf("decoded target %d, want %d (width=%d)", ptrs[0].target, target, w)
		}
		if ptrs[0].width != w {
			t.Errorf("decoded width %d, want %d (target=%d)", ptrs[0].width, w, target)
		}
	}
}

// TestCompactNestedAndIdentity sanity-checks the BFS+merge+identity-fast-path
// pipeline on a tiny synthetic data section: an outer map containing one
// pointer that targets a value nested inside a separate top-level value.
// (Gap removal and pointer-width rewriting are exercised by the dedicated
// tests below.)
func TestCompactNestedAndIdentity(t *testing.T) {
	// Build a tiny data section:
	//   offset 0: utf8 string "hi" (3 bytes: ctrl 0x42, 'h', 'i')
	//   offset 3: utf8 string "world" (6 bytes: ctrl 0x45, 'w','o','r','l','d')
	// Roots = [0, 3]. No pointers, no nested. Identity fast path should hit.
	section := []byte{
		0x42, 'h', 'i',
		0x45, 'w', 'o', 'r', 'l', 'd',
	}
	res, err := compactDataSection(testLogger, section, []uint32{0, 3})
	if err != nil {
		t.Fatalf("compactDataSection: %v", err)
	}
	if !bytes.Equal(res.bytes, section) {
		t.Errorf("identity fast path didn't produce section bytes; got len=%d want len=%d",
			len(res.bytes), len(section))
	}
	if got, want := res.offsetMap[0], uint32(0); got != want {
		t.Errorf("offsetMap[0] = %d, want %d", got, want)
	}
	if got, want := res.offsetMap[3], uint32(3); got != want {
		t.Errorf("offsetMap[3] = %d, want %d", got, want)
	}
}

// TestCompactRemovesDeadGap exercises the non-identity path: a data section
// with unreachable bytes between two reachable records. Compaction must drop
// the gap, pack the records contiguously, and remap offsets. This path does
// not trigger on writer-produced files (their data sections are gap-free), so
// it gets no real-artifact coverage and is pinned here.
func TestCompactRemovesDeadGap(t *testing.T) {
	// offset 0: utf8 "hi"    -> ctrl 0x42, 'h', 'i'   (3 bytes, reachable)
	// offset 3: 5 dead bytes -> unreachable, never parsed
	// offset 8: utf8 "world" -> ctrl 0x45, 'w'..'d'   (6 bytes, reachable)
	section := []byte{
		0x42, 'h', 'i',
		0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
		0x45, 'w', 'o', 'r', 'l', 'd',
	}
	res, err := compactDataSection(testLogger, section, []uint32{0, 8})
	if err != nil {
		t.Fatalf("compactDataSection: %v", err)
	}
	want := []byte{0x42, 'h', 'i', 0x45, 'w', 'o', 'r', 'l', 'd'}
	if !bytes.Equal(res.bytes, want) {
		t.Errorf("compacted bytes = %x, want %x", res.bytes, want)
	}
	if got, want := res.offsetMap[0], uint32(0); got != want {
		t.Errorf("offsetMap[0] = %d, want %d", got, want)
	}
	if got, want := res.offsetMap[8], uint32(3); got != want {
		t.Errorf("offsetMap[8] = %d, want %d", got, want)
	}
}

// TestCompactRewritesPointerWidthAcrossClassBoundary exercises the fixed-point
// width loop. A record holds a class-1 (3-byte) pointer to a far-away target;
// removing the dead gap before that target drops its new offset below 2048, so
// the pointer must be re-encoded as a class-0 (2-byte) pointer. This is the
// part of compaction real writer-produced files never trigger.
func TestCompactRewritesPointerWidthAcrossClassBoundary(t *testing.T) {
	const target = 2048 // class-0/class-1 boundary: 2048 needs 3 bytes here
	section := make([]byte, target+2)
	// offset 0: bare class-1 pointer to `target` (encoded value target-2048=0).
	section[0], section[1], section[2] = 0x28, 0x00, 0x00
	// offsets 3..target-1: dead, unreachable filler.
	for i := 3; i < target; i++ {
		section[i] = 0xFF
	}
	// offset target: utf8 "z".
	section[target], section[target+1] = 0x41, 'z'

	res, err := compactDataSection(testLogger, section, []uint32{0})
	if err != nil {
		t.Fatalf("compactDataSection: %v", err)
	}
	// Output: 2-byte pointer + 2-byte string = 4 bytes (was target+2).
	if len(res.bytes) != 4 {
		t.Fatalf("compacted length = %d, want 4", len(res.bytes))
	}
	if got, want := res.offsetMap[0], uint32(0); got != want {
		t.Errorf("offsetMap[0] = %d, want %d", got, want)
	}
	if got, want := res.offsetMap[target], uint32(2); got != want {
		t.Errorf("offsetMap[%d] = %d, want 2", target, got)
	}
	// The rewritten pointer must be class-0 (2 bytes) and resolve to the
	// string's new offset.
	end, ptrs, err := valueSpan(res.bytes, 0)
	if err != nil {
		t.Fatalf("valueSpan(ptr): %v", err)
	}
	if end != 2 {
		t.Errorf("pointer end = %d, want 2 (class-0 width)", end)
	}
	if len(ptrs) != 1 || ptrs[0].target != 2 || ptrs[0].width != 2 {
		t.Fatalf("decoded pointer = %+v, want target=2 width=2", ptrs)
	}
	// And the target offset really holds the string "z".
	sEnd, _, err := valueSpan(res.bytes, 2)
	if err != nil {
		t.Fatalf("valueSpan(string): %v", err)
	}
	if sEnd != 4 || res.bytes[2] != 0x41 || res.bytes[3] != 'z' {
		t.Errorf("target value malformed: end=%d bytes=%x", sEnd, res.bytes[2:])
	}
}

// TestCompactNestedOffsetAfterPointerWidthShrink is the regression test for the
// data-corruption bug where a nested entry's offsetMap value was computed from
// the keeper's ORIGINAL layout (newOffs[i] + offsetWithin) and never adjusted
// for keeper-internal pointers that shrink during the width fixed-point loop.
//
// The section exercises the NON-identity path: an outer array containing BOTH a
// class-1 (3-byte) pointer to a far target AND a nested sub-value located AFTER
// that pointer; a second record that points directly at the nested sub-value;
// and a dead gap so the far target's new offset drops below 2048, forcing the
// outer pointer to shrink 3->2 bytes. When the outer pointer shrinks, the nested
// sub-value shifts one byte earlier within the emitted record. The fix must make
// offsetMap[nested] track that shift; the unfixed code leaves it equal to the
// original within-keeper offset, pointing one byte too far right (into the
// payload), and a reader then decodes garbage.
func TestCompactNestedOffsetAfterPointerWidthShrink(t *testing.T) {
	const farTarget = 3000 // class-1 (width 3) in the original section

	section := make([]byte, farTarget+2)
	// Outer array (extended kind 11), 2 elements, at offset 0 (length 7):
	//   [0] 0x02  array ctrl, size=2
	//   [1] 0x04  ext byte (11-7=4)
	//   [2..4]    class-1 pointer -> farTarget (encodes farTarget-2048=952)
	//   [5..6]    nested sub-value: utf8 string "z" (ctrl 0x41, 'z')
	section[0] = 0x02
	section[1] = 0x04
	if err := writePointer(section[2:5], farTarget, 3); err != nil {
		t.Fatalf("writePointer(outer): %v", err)
	}
	const nestedOrig = 5
	section[5] = 0x41
	section[6] = 'z'
	// Second record at offset 7 (length 2): a class-0 pointer -> the nested
	// sub-value at offset 5. This is the referrer that must survive the shift.
	const secondPtr = 7
	if err := writePointer(section[secondPtr:secondPtr+2], nestedOrig, 2); err != nil {
		t.Fatalf("writePointer(second): %v", err)
	}
	// offsets 9..farTarget-1: dead, unreachable filler.
	for i := secondPtr + 2; i < farTarget; i++ {
		section[i] = 0xFF
	}
	// Far target at offset farTarget (length 2): utf8 string "z".
	section[farTarget] = 0x41
	section[farTarget+1] = 'z'

	res, err := compactDataSection(testLogger, section, []uint32{0, secondPtr})
	if err != nil {
		t.Fatalf("compactDataSection: %v", err)
	}

	// The outer pointer must have shrunk 3->2, so the nested value shifts one
	// byte earlier: original within-keeper offset 5 - 1 = new offset 4.
	const wantNested = 4
	if got := res.offsetMap[nestedOrig]; got != wantNested {
		t.Fatalf("offsetMap[%d] = %d, want %d (must track the shrink-induced shift)",
			nestedOrig, got, wantNested)
	}
	// The byte at the remapped nested offset must be the string control byte
	// 0x41, not the 'z' payload byte (which is what the buggy offset pointed at).
	if res.bytes[wantNested] != 0x41 {
		t.Fatalf("byte at remapped nested offset %d = 0x%02x, want 0x41 (string ctrl)",
			wantNested, res.bytes[wantNested])
	}

	// Walking the emitted section from EVERY offsetMap target must succeed and,
	// for the records whose original value we know, yield the original value.
	for orig, neu := range res.offsetMap {
		end, _, err := valueSpan(res.bytes, neu)
		if err != nil {
			t.Fatalf("valueSpan from offsetMap[%d]=%d failed: %v", orig, neu, err)
		}
		if end > uint32(len(res.bytes)) {
			t.Fatalf("valueSpan from offsetMap[%d]=%d overran section (end=%d, len=%d)",
				orig, neu, end, len(res.bytes))
		}
	}

	// The nested sub-value decodes as the original string "z".
	if got := decodeStringAt(t, res.bytes, res.offsetMap[nestedOrig]); got != "z" {
		t.Errorf("nested value = %q, want %q", got, "z")
	}
	// The far target also decodes as "z" at its remapped offset.
	if got := decodeStringAt(t, res.bytes, res.offsetMap[farTarget]); got != "z" {
		t.Errorf("far target value = %q, want %q", got, "z")
	}
	// The second record's pointer must now resolve to the (shifted) nested
	// offset, proving the referrer reaches the correct value.
	_, ptrs, err := valueSpan(res.bytes, res.offsetMap[secondPtr])
	if err != nil {
		t.Fatalf("valueSpan(second ptr): %v", err)
	}
	if len(ptrs) != 1 {
		t.Fatalf("second record: got %d ptrs, want 1", len(ptrs))
	}
	if ptrs[0].target != wantNested {
		t.Errorf("second pointer resolves to %d, want %d (the shifted nested offset)",
			ptrs[0].target, wantNested)
	}
}

// decodeStringAt walks a utf8-string (kind 2) value at off in buf and returns
// its contents, failing the test on any decode error or wrong kind.
func decodeStringAt(t *testing.T, buf []byte, off uint32) string {
	t.Helper()
	if int(off) >= len(buf) {
		t.Fatalf("decodeStringAt: offset %d past section (%d)", off, len(buf))
	}
	ctrl := buf[off]
	if kind := ctrl >> 5; kind != 2 {
		t.Fatalf("decodeStringAt: offset %d kind = %d, want 2 (utf8 string)", off, kind)
	}
	n, after, err := readSizeAt(buf, int(ctrl&0x1f), off+1)
	if err != nil {
		t.Fatalf("decodeStringAt: readSizeAt @%d: %v", off, err)
	}
	if int(after)+int(n) > len(buf) {
		t.Fatalf("decodeStringAt: string @%d overruns section", off)
	}
	return string(buf[after : after+n])
}
