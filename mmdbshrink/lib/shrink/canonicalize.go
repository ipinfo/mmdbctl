package shrink

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// canonNodes is the bottom-up canonical assignment: a slice of canonical
// nodes, each holding two child pointers in the *output* numbering. The
// input tree's root is pinned at output index 0 unconditionally (see
// canonicalize), so there is no separate root flag or pointer.
type canonNodes = []canonNode

type canonNode struct {
	left, right pointer
}
type pointer struct {
	kind pointerKind
	// output node index (nodeKind) or data section byte offset (dataKind)
	id uint32
}

// pointerKind tags the meaning of a canonical pointer value:
//   - nullKind: empty branch (encoded as `node_count` in the output)
//   - nodeKind: child is another canonical node (id is the output index)
//   - dataKind: child is a record (id is the byte offset into the data section)
type pointerKind uint8

const (
	nullKind pointerKind = iota
	nodeKind
	dataKind
)

type canonicalKey struct {
	left  uint64
	right uint64
}

type frame struct {
	idx  uint32
	post bool
}

// canonicalize traverses treeBuffer and returns a deduplicated node list.
// If a node has an identical child pair as another the previous one is reused, otherwise a new one is created.
// If a non-root node's children are both empty the node itself becomes empty.
// Orphan nodes are dropped.
func canonicalize(logger *slog.Logger, treeBuffer []byte, nodeCount uint32, recordSize uint64) (canonNodes, error) {
	// 0 unvisited, 1 visiting, 2 done
	state := make([]uint8, nodeCount)
	// encoded pointer per input node
	canonicalValue := make([]uint64, nodeCount)
	// canonicalKey -> output node index
	canonical := make(map[canonicalKey]uint32)

	output := []canonNode{}
	// Pin output index 0 for the root. Will fill it in after the post-order
	// pass on root completes; until then leave a placeholder.
	output = append(output, canonNode{})

	// Progress: log every ~5% finalized nodes (or every 10s, whichever first).
	progressEnabled := logger.Enabled(context.Background(), slog.LevelDebug)
	finalized := uint32(0)
	progressEvery := max(nodeCount/20, 100_000)
	tProgress := time.Now()

	stack := []frame{{idx: 0}}
	for len(stack) > 0 {
		next := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if next.idx >= nodeCount {
			return canonNodes{}, fmt.Errorf("internal: idx %d >= node_count %d", next.idx, nodeCount)
		}
		if !next.post {
			switch state[next.idx] {
			case 2:
				continue
			case 1:
				return canonNodes{}, fmt.Errorf("cycle at node %d", next.idx)
			}
			state[next.idx] = 1
			stack = append(stack, frame{idx: next.idx, post: true})
			left, right, err := readNodePair(treeBuffer, next.idx, recordSize)
			if err != nil {
				return canonNodes{}, err
			}
			if right < nodeCount && state[right] == 0 {
				stack = append(stack, frame{idx: right})
			}
			if left < nodeCount && state[left] == 0 {
				stack = append(stack, frame{idx: left})
			}
			continue
		}

		// Post-order visit: compute encoded child pointers + intern.
		left, right, err := readNodePair(treeBuffer, next.idx, recordSize)
		if err != nil {
			return canonNodes{}, err
		}
		leftEnc, err := encodeChild(left, nodeCount, canonicalValue, state)
		if err != nil {
			return canonNodes{}, fmt.Errorf("node %d left: %w", next.idx, err)
		}
		rightEnc, err := encodeChild(right, nodeCount, canonicalValue, state)
		if err != nil {
			return canonNodes{}, fmt.Errorf("node %d right: %w", next.idx, err)
		}
		if leftEnc == 0 && rightEnc == 0 && next.idx != 0 {
			canonicalValue[next.idx] = 0
			state[next.idx] = 2
			continue
		}
		if next.idx == 0 {
			// Pin the root at output index 0. If both children are null we
			// still emit a single root node (a "no records" MMDB has one node
			// with two null pointers — a valid, if useless, mmdb).
			output[0] = canonNode{left: decodePointer(leftEnc), right: decodePointer(rightEnc)}
			canonicalValue[next.idx] = encodePointer(pointer{kind: nodeKind, id: 0})
			state[next.idx] = 2
			continue
		}
		key := canonicalKey{left: leftEnc, right: rightEnc}
		id, ok := canonical[key]
		if !ok {
			id = uint32(len(output))
			output = append(output, canonNode{left: decodePointer(leftEnc), right: decodePointer(rightEnc)})
			canonical[key] = id
		}
		canonicalValue[next.idx] = encodePointer(pointer{kind: nodeKind, id: id})
		state[next.idx] = 2
		finalized++
		if progressEnabled && (finalized%progressEvery == 0 || time.Since(tProgress) > 10*time.Second) {
			logger.Debug(fmt.Sprintf("phase1: finalized %d/%d (%.1f%%) — canonical=%d",
				finalized,
				nodeCount,
				100.0*float64(finalized)/float64(nodeCount),
				len(output),
			))
			tProgress = time.Now()
		}
	}
	return output, nil
}

func readNodePair(buf []byte, idx uint32, recordSize uint64) (uint32, uint32, error) {
	nodeBytes := recordSize / 4
	off := uint64(idx) * nodeBytes
	if off+nodeBytes > uint64(len(buf)) {
		return 0, 0, fmt.Errorf("node %d offset %d > buf %d", idx, off, len(buf))
	}
	b := buf[off:]
	switch recordSize {
	case 24:
		l := uint32(b[0])<<16 | uint32(b[1])<<8 | uint32(b[2])
		r := uint32(b[3])<<16 | uint32(b[4])<<8 | uint32(b[5])
		return l, r, nil
	case 28:
		l := (uint32(b[3])&0xf0)<<20 | uint32(b[0])<<16 | uint32(b[1])<<8 | uint32(b[2])
		r := (uint32(b[3])&0x0f)<<24 | uint32(b[4])<<16 | uint32(b[5])<<8 | uint32(b[6])
		return l, r, nil
	case 32:
		l := uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
		r := uint32(b[4])<<24 | uint32(b[5])<<16 | uint32(b[6])<<8 | uint32(b[7])
		return l, r, nil
	}
	return 0, 0, fmt.Errorf("unsupported record_size %d", recordSize)
}

func encodeChild(value uint32, nodeCount uint32, canonicalValue []uint64, state []uint8) (uint64, error) {
	switch {
	case value < nodeCount:
		if state[value] != 2 {
			return 0, fmt.Errorf("referenced node %d not finalized", value)
		}
		return canonicalValue[value], nil
	case value == nodeCount:
		return 0, nil
	case uint64(value) < uint64(nodeCount)+dataSectionSeparatorSize:
		return 0, fmt.Errorf("data pointer %d falls inside the 16-byte separator", value)
	default:
		// Data section offset (in bytes) — store this as the canonical id.
		off := uint32(uint64(value) - uint64(nodeCount) - dataSectionSeparatorSize)
		return encodePointer(pointer{kind: dataKind, id: off}), nil
	}
}

func encodePointer(p pointer) uint64 {
	switch p.kind {
	case nullKind:
		return 0
	case nodeKind:
		return uint64(0x1)<<60 | uint64(p.id)
	case dataKind:
		return uint64(0x2)<<60 | uint64(p.id)
	}
	return 0
}

func decodePointer(v uint64) pointer {
	if v == 0 {
		return pointer{kind: nullKind}
	}
	tag := v >> 60
	id := uint32(v & ((1 << 60) - 1))
	switch tag {
	case 0x1:
		return pointer{kind: nodeKind, id: id}
	case 0x2:
		return pointer{kind: dataKind, id: id}
	}
	return pointer{kind: nullKind}
}
