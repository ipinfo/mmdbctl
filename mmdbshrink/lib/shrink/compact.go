package shrink

import (
	"encoding/binary"
	"fmt"
	"github.com/ipinfo/mmdbctl/mmdbshrink/lib/format"
	"log/slog"
	"sort"
	"time"
)

// compactResult is the output of compaction.
type compactResult struct {
	// the new compact data section
	bytes []byte
	// old data-section offset -> new offset (one entry per reachable record start)
	offsetMap map[uint32]uint32
}

// dataRecord is one transitively reachable region of the data section
// in the input, expressed in original-data-section coordinates.
type dataRecord struct {
	// start of the record in the original data section
	offset uint32
	// record length in the original data section
	length uint32
	// pointers contained in this record (offsets are absolute in the original section)
	ptrs []ptrLoc
}

// ptrLoc records one MMDB pointer (kind 1) discovered while walking a
// data-section value: where its encoding starts, what offset it points to,
// and how many bytes the original encoding occupies.
type ptrLoc struct {
	// byte offset in the original data section where the pointer's control byte lives
	offset uint32
	// the data-section offset the pointer references
	target uint32
	// 2..5; bytes consumed by the original encoding
	width uint8
}

// compact rebuilds the data section around what the canonical tree still
// references, and rewrites the tree's leaf pointers to the new offsets.
func compact(logger *slog.Logger, canon canonNodes, dataSection []byte) ([]byte, error) {
	rootSet := make(map[uint32]struct{}, len(canon))
	for _, n := range canon {
		if n.left.kind == dataKind {
			rootSet[n.left.id] = struct{}{}
		}
		if n.right.kind == dataKind {
			rootSet[n.right.id] = struct{}{}
		}
	}
	roots := make([]uint32, 0, len(rootSet))
	for o := range rootSet {
		roots = append(roots, o)
	}
	logger.Debug(fmt.Sprintf("phase1.5: data-section compaction (%s -> ?)", format.DecimalBytes(uint64(len(dataSection)))))

	cmp, err := compactDataSection(logger, dataSection, roots)
	if err != nil {
		return nil, fmt.Errorf("compact: %w", err)
	}
	// Rewrite tree-leaf pointer ids to reference new offsets.
	for i := range canon {
		if canon[i].left.kind == dataKind {
			newOff, ok := cmp.offsetMap[canon[i].left.id]
			if !ok {
				return nil, fmt.Errorf("missing offsetMap entry for tree leaf left id %d", canon[i].left.id)
			}
			canon[i].left.id = newOff
		}
		if canon[i].right.kind == dataKind {
			newOff, ok := cmp.offsetMap[canon[i].right.id]
			if !ok {
				return nil, fmt.Errorf("missing offsetMap entry for tree leaf right id %d", canon[i].right.id)
			}
			canon[i].right.id = newOff
		}
	}
	return cmp.bytes, nil
}

// compactDataSection reduces the data section to only bytes reachable from
// the canonicalized tree's leaf pointers, plus any records reachable
// transitively through MMDB pointers within those records.
// Pointers in the output are encoded at the minimum width for their new target.
func compactDataSection(logger *slog.Logger, dataSection []byte, roots []uint32) (compactResult, error) {
	tCompact := time.Now()
	logger.Debug(fmt.Sprintf("compact: starting; %d unique tree-leaf data offsets", len(roots)))

	records, err := collectReachable(dataSection, roots)
	if err != nil {
		return compactResult{}, fmt.Errorf("collectReachable: %w", err)
	}
	rawCount := len(records)

	// Merge nested records into their containers. An MMDB pointer can target
	// the START of any sub-value (e.g., one map entry's value inside a larger
	// map). Our BFS creates a separate dataRecord for that target offset, but
	// its bytes are physically inside the outer record's encoding. Treating
	// them as independent records would duplicate bytes in the compact section
	// and corrupt offset arithmetic. After sorting by offset ascending, scan
	// linearly: a record nested entirely inside the most-recent keeper is
	// dropped (its offset gets a derived offsetMap entry pointing into the
	// keeper); a record partially overlapping a keeper is malformed.
	sort.Slice(records, func(i, j int) bool {
		if records[i].offset != records[j].offset {
			return records[i].offset < records[j].offset
		}
		return records[i].length > records[j].length
	})

	type nestedEntry struct {
		offset       uint32
		outerOffset  uint32
		offsetWithin uint32
	}
	var nested []nestedEntry
	keepers := records[:0]
	var curEnd uint32
	var curOff uint32
	for _, r := range records {
		if len(keepers) == 0 || r.offset >= curEnd {
			keepers = append(keepers, r)
			curOff = r.offset
			curEnd = r.offset + r.length
			continue
		}
		if r.offset+r.length <= curEnd {
			nested = append(nested, nestedEntry{
				offset:       r.offset,
				outerOffset:  curOff,
				offsetWithin: r.offset - curOff,
			})
			continue
		}
		return compactResult{}, fmt.Errorf("records overlap partially: outer [%d,%d) inner [%d,%d) — malformed mmdb",
			curOff, curEnd, r.offset, r.offset+r.length)
	}
	records = keepers
	if len(nested) > 0 {
		logger.Debug(fmt.Sprintf("compact: %d records (%d nested merged into containers, %d kept)",
			rawCount,
			len(nested),
			len(records),
		))
	} else {
		logger.Debug(fmt.Sprintf("compact: %d reachable records", rawCount))
	}

	// Sanity check: keepers should cover the input section once each, with
	// gaps only where bytes are unreachable. Sum of keeper lengths must be
	// <= sectionSize. If >, keepers overlap or valueSpan overshot some
	// record's true length — both indicate a parser bug we want to surface.
	{
		var sum uint64
		for _, r := range records {
			sum += uint64(r.length)
		}
		if sum > uint64(len(dataSection)) {
			return compactResult{}, fmt.Errorf(
				"keeper coverage %d > section size %d (%+d): valueSpan overshot or merge missed an overlap",
				sum, len(dataSection), int64(sum)-int64(len(dataSection)))
		}
	}

	// Per-pointer current width, mutable across the fixed-point loop.
	// widths[i][j] is the current encoding width of records[i].ptrs[j].
	// Initialized to original widths; can only shrink across iterations.
	widths := make([][]uint8, len(records))
	for i, r := range records {
		widths[i] = make([]uint8, len(r.ptrs))
		for j, p := range r.ptrs {
			widths[i][j] = p.width
		}
	}

	// Per-record current new length.
	newLens := make([]uint32, len(records))
	// Per-record current new offset.
	newOffs := make([]uint32, len(records))

	recomputeLensAndOffsets := func() uint32 {
		var total uint32
		for i, r := range records {
			var widthDelta int64
			for j, p := range r.ptrs {
				widthDelta += int64(widths[i][j]) - int64(p.width)
			}
			newLens[i] = uint32(int64(r.length) + widthDelta)
			newOffs[i] = total
			total += newLens[i]
		}
		return total
	}

	// outerNewOffsetByOrig maps each keeper's original offset to its index in
	// records[], used to resolve nested offsets when building offsetMap.
	outerIdx := make(map[uint32]int, len(records))
	for i, r := range records {
		outerIdx[r.offset] = i
	}

	// Build offsetMap (old -> new). Includes both keeper records (newOffs[i])
	// and nested records (resolve into the keeper's POST-REWRITE layout).
	//
	// A nested entry's offsetWithin is the byte distance from the keeper start
	// in ORIGINAL coordinates, frozen during the merge pass. But emitRecord
	// re-encodes each keeper-internal pointer at its (possibly narrower) NEW
	// width, shifting every byte AFTER a shrunk pointer leftward within the
	// emitted record. So the nested value's true new position is its original
	// within-keeper offset plus the signed sum of width deltas for every keeper
	// pointer that physically precedes it (original absolute offset < the nested
	// offset). We read the CURRENT widths each call so the fixed-point loop and
	// the final map both see consistent nested offsets. On the identity fast
	// path every width equals its original, so every delta is zero and the
	// result is unchanged. All physically-preceding keeper pointers are counted
	// regardless of nesting depth, so nested-within-nested is handled too.
	buildOffsetMap := func() map[uint32]uint32 {
		m := make(map[uint32]uint32, len(records)+len(nested))
		for i, r := range records {
			m[r.offset] = newOffs[i]
		}
		for _, n := range nested {
			i, ok := outerIdx[n.outerOffset]
			if !ok {
				continue
			}
			var shift int64
			for j, p := range records[i].ptrs {
				if p.offset < n.offset {
					shift += int64(widths[i][j]) - int64(p.width)
				}
			}
			m[n.offset] = uint32(int64(newOffs[i]+n.offsetWithin) + shift)
		}
		return m
	}

	// Fast-path identity check: if (a) keepers cover the whole input section
	// (no gaps — no unreachable bytes were dropped) and (b) every pointer's
	// original encoding width is already the minimum for its target offset,
	// then re-emitting would produce a section byte-identical to the input.
	// Skip the rewrite entirely and reuse the input bytes. Nested records
	// don't affect this — their bytes are physically inside the keepers
	// they live in, so a verbatim outer-record copy preserves them. The
	// offsetMap built from current newOffs already includes correctly
	// derived entries for nested offsets in this case.
	finalTotal := recomputeLensAndOffsets()
	offsetMap := buildOffsetMap()
	if finalTotal == uint32(len(dataSection)) {
		identical := true
		for i, r := range records {
			for j, p := range r.ptrs {
				newTarget, ok := offsetMap[p.target]
				if !ok || newTarget != p.target {
					identical = false
					break
				}
				if minWidthForOffset(newTarget) != widths[i][j] {
					identical = false
					break
				}
			}
			if !identical {
				break
			}
		}
		if identical {
			logger.Debug(fmt.Sprintf("compact: input section is already minimum-form (no gaps, no over-wide pointers); reusing verbatim (%s)",
				format.DecimalBytes(uint64(len(dataSection)))))
			logger.Debug(fmt.Sprintf("compact: done in %s — %d -> %d bytes (Δ +0, identity)",
				time.Since(tCompact).Round(time.Millisecond),
				len(dataSection),
				len(dataSection),
			))
			return compactResult{bytes: dataSection, offsetMap: offsetMap}, nil
		}
	}

	// Fixed-point loop: adjust widths until each pointer's width matches the
	// minimum required for its current new target. We use `!=` (not `<`) so
	// that the loop converges to the true minimum encoding even at class
	// boundaries — e.g., an original target of 2050 (class 1, width 3) whose
	// new offset becomes 2040 (class 0, width 2) is correctly downgraded.
	// Convergence: shrinking widths decreases record lengths, which shifts
	// later records' offsets only downward, which can only further shrink
	// pointer widths — so the system is monotonically non-increasing and
	// must terminate.
	const maxIter = 32
	for iter := 0; ; iter++ {
		finalTotal = recomputeLensAndOffsets()
		offsetMap = buildOffsetMap()
		changed := false
		for i, r := range records {
			for j, p := range r.ptrs {
				newTarget, ok := offsetMap[p.target]
				if !ok {
					return compactResult{}, fmt.Errorf("pointer target %d (record %d) not in offsetMap", p.target, r.offset)
				}
				w := minWidthForOffset(newTarget)
				if w != widths[i][j] {
					widths[i][j] = w
					changed = true
				}
			}
		}
		if !changed {
			logger.Debug(fmt.Sprintf("compact: pointer widths stabilized after %d iter (final %d bytes)",
				iter+1,
				finalTotal,
			))
			break
		}
		if iter+1 >= maxIter {
			return compactResult{}, fmt.Errorf("compact: pointer widths failed to converge in %d iterations", maxIter)
		}
	}

	finalMap := buildOffsetMap()

	// Emit the compacted section.
	out := make([]byte, finalTotal)
	for i, r := range records {
		dst := out[newOffs[i] : newOffs[i]+newLens[i]]
		if err := emitRecord(dataSection, r, widths[i], dst, finalMap); err != nil {
			return compactResult{}, fmt.Errorf("emit record %d: %w", r.offset, err)
		}
	}

	delta := int64(finalTotal) - int64(len(dataSection))
	pct := 100.0 * float64(delta) / float64(len(dataSection))
	logger.Debug(fmt.Sprintf("compact: done in %s — %d -> %d bytes (Δ %+d, %+.2f%%)",
		time.Since(tCompact).Round(time.Millisecond),
		len(dataSection),
		finalTotal,
		delta,
		pct,
	))

	return compactResult{bytes: out, offsetMap: finalMap}, nil
}

// collectReachable walks every root and follows MMDB pointers transitively,
// returning one dataRecord per unique reachable offset.
func collectReachable(buf []byte, roots []uint32) ([]dataRecord, error) {
	seen := make(map[uint32]struct{}, len(roots))
	var records []dataRecord
	queue := make([]uint32, 0, len(roots))
	for _, r := range roots {
		if _, ok := seen[r]; !ok {
			seen[r] = struct{}{}
			queue = append(queue, r)
		}
	}
	for len(queue) > 0 {
		off := queue[0]
		queue = queue[1:]
		end, ptrs, err := valueSpan(buf, off)
		if err != nil {
			return nil, fmt.Errorf("valueSpan @%d: %w", off, err)
		}
		records = append(records, dataRecord{offset: off, length: end - off, ptrs: ptrs})
		for _, p := range ptrs {
			if _, ok := seen[p.target]; !ok {
				seen[p.target] = struct{}{}
				queue = append(queue, p.target)
			}
		}
	}
	return records, nil
}

// emitRecord copies the record's bytes into dst, replacing each pointer
// encoding with one of the chosen `widths` and the new target offset from
// offsetMap. Verifies the dst length matches the projected new length.
func emitRecord(src []byte, rec dataRecord, widths []uint8, dst []byte, offsetMap map[uint32]uint32) error {
	// Sort pointers by absolute offset (within original data section).
	idx := make([]int, len(rec.ptrs))
	for i := range idx {
		idx[i] = i
	}
	sort.Slice(idx, func(a, b int) bool { return rec.ptrs[idx[a]].offset < rec.ptrs[idx[b]].offset })

	srcCur := rec.offset
	srcEnd := rec.offset + rec.length
	dstCur := uint32(0)

	for _, k := range idx {
		p := rec.ptrs[k]
		if p.offset < srcCur || p.offset+uint32(p.width) > srcEnd {
			return fmt.Errorf("pointer at %d (width %d) outside record [%d,%d)", p.offset, p.width, rec.offset, srcEnd)
		}
		preLen := p.offset - srcCur
		copy(dst[dstCur:dstCur+preLen], src[srcCur:srcCur+preLen])
		dstCur += preLen
		srcCur += preLen
		newTarget, ok := offsetMap[p.target]
		if !ok {
			return fmt.Errorf("pointer target %d not in offsetMap", p.target)
		}
		w := widths[k]
		if dstCur+uint32(w) > uint32(len(dst)) {
			return fmt.Errorf("dst overflow at ptr write (record %d, ptr %d)", rec.offset, p.offset)
		}
		if err := writePointer(dst[dstCur:dstCur+uint32(w)], newTarget, w); err != nil {
			return err
		}
		dstCur += uint32(w)
		srcCur += uint32(p.width)
	}
	tailLen := srcEnd - srcCur
	if dstCur+tailLen != uint32(len(dst)) {
		return fmt.Errorf("emit length mismatch: dstCur=%d tail=%d dstLen=%d", dstCur, tailLen, len(dst))
	}
	copy(dst[dstCur:], src[srcCur:srcEnd])
	return nil
}

// minWidthForOffset returns the minimum number of bytes required to encode
// `target` as an MMDB pointer (kind 1), including the control byte.
//
// Class boundaries (per MMDB spec, including the per-class additive offsets):
//   - class 0 (2 bytes): 0 .. 2047                            (11 raw bits)
//   - class 1 (3 bytes): 2048 .. 526335                       (19 raw bits + 2048)
//   - class 2 (4 bytes): 526336 .. 134744063                  (27 raw bits + 526336)
//   - class 3 (5 bytes): 0 .. 2^32-1                          (32 raw bits, no offset)
//
// The class-2 ceiling 134744063 = (2^27 - 1) + 526336 must be exact —
// off-by-one here makes the shrinker bump valid class-2 pointers to
// class 3 unnecessarily.
func minWidthForOffset(target uint32) uint8 {
	switch {
	case target <= 2047:
		return 2
	case target <= 526335:
		return 3
	case target <= 134744063:
		return 4
	default:
		return 5
	}
}

// writePointer encodes target into the MMDB pointer (kind 1) format using
// `width` bytes total (2..5). The width must be the minimum-or-larger
// width that can hold the value; behavior is well-defined for any width
// >= the minimum.
func writePointer(buf []byte, target uint32, width uint8) error {
	switch width {
	case 2:
		if target > 2047 {
			return fmt.Errorf("target %d > 2047 for class 0", target)
		}
		// kind=1 (001), class=0 (00), top 3 bits of value
		buf[0] = 0x20 | byte((target>>8)&0x07)
		buf[1] = byte(target)
	case 3:
		// class 1 encodes target-2048; for targets < 2048 this would underflow,
		// so prefer class 0 (handled above by minWidthForOffset). Keep an
		// explicit check in case a wider width was forced for some reason.
		if target < 2048 || target > 526335 {
			return fmt.Errorf("target %d out of class 1 range", target)
		}
		v := target - 2048
		buf[0] = 0x28 | byte((v>>16)&0x07)
		buf[1] = byte(v >> 8)
		buf[2] = byte(v)
	case 4:
		if target < 526336 || target > 134744063 {
			return fmt.Errorf("target %d out of class 2 range", target)
		}
		v := target - 526336
		buf[0] = 0x30 | byte((v>>24)&0x07)
		buf[1] = byte(v >> 16)
		buf[2] = byte(v >> 8)
		buf[3] = byte(v)
	case 5:
		buf[0] = 0x38
		binary.BigEndian.PutUint32(buf[1:5], target)
	default:
		return fmt.Errorf("invalid pointer width %d", width)
	}
	return nil
}

// valueSpan walks one MMDB value at the given offset. It returns:
//   - end: the byte offset just past the value's encoding
//   - ptrs: pointer encodings (kind 1) found INSIDE this value
//
// For pointer values themselves, the pointer is reported and end stops
// after the pointer's encoded bytes. The value at the pointer's target is
// reached separately via collectReachable.
func valueSpan(buf []byte, off uint32) (end uint32, ptrs []ptrLoc, err error) {
	if int(off) >= len(buf) {
		return 0, nil, fmt.Errorf("offset %d past data section (%d)", off, len(buf))
	}
	ctrl := buf[off]
	kind := int(ctrl >> 5)
	sizeBits := int(ctrl & 0x1f)
	cur := off + 1
	if kind == 0 {
		if int(cur) >= len(buf) {
			return 0, nil, fmt.Errorf("ext byte missing @%d", off)
		}
		kind = int(buf[cur]) + 7
		cur++
	}

	switch kind {
	case 1: // pointer
		target, end, err := readPointerAt(buf, sizeBits, cur)
		if err != nil {
			return 0, nil, fmt.Errorf("%w (ptr @%d)", err, off)
		}
		return end, []ptrLoc{{offset: off, target: target, width: uint8(end - off)}}, nil

	case 2, 4: // utf8 string, bytes
		n, after, err := readSizeAt(buf, sizeBits, cur)
		if err != nil {
			return 0, nil, err
		}
		if int(after)+int(n) > len(buf) {
			return 0, nil, fmt.Errorf("string/bytes length %d overruns @%d", n, off)
		}
		return after + n, nil, nil

	case 3, 5, 6, 8, 9, 10, 15:
		// Numeric kinds: payload = sizeBits bytes (BE).
		// 3=double, 5=u16, 6=u32, 8=i32, 9=u64, 10=u128, 15=float.
		if sizeBits > 16 {
			return 0, nil, fmt.Errorf("kind %d bad sizeBits %d @%d", kind, sizeBits, off)
		}
		if int(cur)+sizeBits > len(buf) {
			return 0, nil, fmt.Errorf("kind %d overrun @%d", kind, off)
		}
		return cur + uint32(sizeBits), nil, nil

	case 14: // bool (extended). No payload; sizeBits is the value (0/1).
		return cur, nil, nil

	case 7: // map: sizeBits = entry count
		n, after, err := readSizeAt(buf, sizeBits, cur)
		if err != nil {
			return 0, nil, err
		}
		c := after
		for i := uint32(0); i < n; i++ {
			ke, kp, err := valueSpan(buf, c)
			if err != nil {
				return 0, nil, fmt.Errorf("map[%d] key @%d: %w", i, off, err)
			}
			ptrs = append(ptrs, kp...)
			c = ke
			ve, vp, err := valueSpan(buf, c)
			if err != nil {
				return 0, nil, fmt.Errorf("map[%d] val @%d: %w", i, off, err)
			}
			ptrs = append(ptrs, vp...)
			c = ve
		}
		return c, ptrs, nil

	case 11: // array (extended): sizeBits = element count
		n, after, err := readSizeAt(buf, sizeBits, cur)
		if err != nil {
			return 0, nil, err
		}
		c := after
		for i := uint32(0); i < n; i++ {
			e, p, err := valueSpan(buf, c)
			if err != nil {
				return 0, nil, fmt.Errorf("array[%d] @%d: %w", i, off, err)
			}
			ptrs = append(ptrs, p...)
			c = e
		}
		return c, ptrs, nil
	}
	return 0, nil, fmt.Errorf("unsupported kind %d @%d", kind, off)
}
