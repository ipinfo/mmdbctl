package shrink

import (
	"encoding/binary"
	"fmt"
)

// buildSearchTree turns the canonical nodes into raw mmdb tree bytes.
// Returns an error if a pointer doesn't fit in recordSize bits.
func buildSearchTree(nodes canonNodes, recordSize uint64) ([]byte, error) {
	nodesCount := uint32(len(nodes))
	nodeBytes := recordSize / 4
	out := make([]byte, uint64(nodesCount)*nodeBytes)
	for i := range nodesCount {
		left := pointerToTreeValue(nodes[i].left, nodesCount)
		right := pointerToTreeValue(nodes[i].right, nodesCount)
		if err := writeNodePair(out, i, recordSize, left, right); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func pointerToTreeValue(p pointer, outNodeCount uint32) uint32 {
	switch p.kind {
	case nullKind:
		return outNodeCount
	case nodeKind:
		return p.id
	case dataKind:
		return p.id + outNodeCount + dataSectionSeparatorSize
	}
	return outNodeCount
}

func writeNodePair(buf []byte, idx uint32, recordSize uint64, left, right uint32) error {
	nodeBytes := recordSize / 4
	off := uint64(idx) * nodeBytes
	if off+nodeBytes > uint64(len(buf)) {
		return fmt.Errorf("write node %d offset %d > buf %d", idx, off, len(buf))
	}
	b := buf[off:]
	switch recordSize {
	case 24:
		if left>>24 != 0 || right>>24 != 0 {
			return fmt.Errorf("node %d: 24-bit record overflow (l=%d r=%d)", idx, left, right)
		}
		b[0] = byte(left >> 16)
		b[1] = byte(left >> 8)
		b[2] = byte(left)
		b[3] = byte(right >> 16)
		b[4] = byte(right >> 8)
		b[5] = byte(right)
	case 28:
		if left>>28 != 0 || right>>28 != 0 {
			return fmt.Errorf("node %d: 28-bit record overflow (l=%d r=%d)", idx, left, right)
		}
		b[0] = byte(left >> 16)
		b[1] = byte(left >> 8)
		b[2] = byte(left)
		b[3] = byte((left>>20)&0xf0) | byte((right>>24)&0x0f)
		b[4] = byte(right >> 16)
		b[5] = byte(right >> 8)
		b[6] = byte(right)
	case 32:
		binary.BigEndian.PutUint32(b[0:4], left)
		binary.BigEndian.PutUint32(b[4:8], right)
	default:
		return fmt.Errorf("unsupported record_size %d", recordSize)
	}
	return nil
}
