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
		if left > 0xFFFFFFFF || right > 0xFFFFFFFF {
			return nil, fmt.Errorf("node %d: tree value exceeds 32-bit record size (l=%d r=%d)", i, left, right)
		}
		if err := writeNodePair(out, i, recordSize, uint32(left), uint32(right)); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func smallestRecordSize(nodes canonNodes, nodeCount uint32) (uint64, error) {
	maxValue := uint64(nodeCount)
	for _, node := range nodes {
		for _, p := range [...]pointer{node.left, node.right} {
			if value := pointerToTreeValue(p, nodeCount); value > maxValue {
				maxValue = value
			}
		}
	}
	switch {
	case maxValue <= 0xFFFFFF:
		return 24, nil
	case maxValue <= 0x0FFFFFFF:
		return 28, nil
	case maxValue <= 0xFFFFFFFF:
		return 32, nil
	default:
		return 0, fmt.Errorf("tree value %d exceeds maximum 32-bit record value", maxValue)
	}
}

func pointerToTreeValue(p pointer, outNodeCount uint32) uint64 {
	switch p.kind {
	case nullKind:
		return uint64(outNodeCount)
	case nodeKind:
		return uint64(p.id)
	case dataKind:
		return uint64(p.id) + uint64(outNodeCount) + dataSectionSeparatorSize
	}
	return uint64(outNodeCount)
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
