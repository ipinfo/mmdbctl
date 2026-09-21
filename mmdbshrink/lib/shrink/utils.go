package shrink

import (
	"encoding/binary"
	"fmt"
)

// readSizeAt decodes the count/length prefix of a length-prefixed kind
// following the MMDB spec:
//
//	size 0..28: literal value
//	size 29:    next 1 byte is (length - 29)
//	size 30:    next 2 bytes are (length - 285) big-endian
//	size 31:    next 3 bytes are (length - 65821) big-endian
func readSizeAt(buf []byte, sizeBits int, cur uint32) (uint32, uint32, error) {
	switch {
	case sizeBits <= 28:
		return uint32(sizeBits), cur, nil
	case sizeBits == 29:
		if int(cur)+1 > len(buf) {
			return 0, 0, fmt.Errorf("size29 ext")
		}
		return 29 + uint32(buf[cur]), cur + 1, nil
	case sizeBits == 30:
		if int(cur)+2 > len(buf) {
			return 0, 0, fmt.Errorf("size30 ext")
		}
		return 285 + uint32(binary.BigEndian.Uint16(buf[cur:cur+2])), cur + 2, nil
	case sizeBits == 31:
		if int(cur)+3 > len(buf) {
			return 0, 0, fmt.Errorf("size31 ext")
		}
		v := uint32(buf[cur])<<16 | uint32(buf[cur+1])<<8 | uint32(buf[cur+2])
		return 65821 + v, cur + 3, nil
	}
	return 0, 0, fmt.Errorf("invalid sizeBits %d", sizeBits)
}

// readPointerAt decodes the payload of an MMDB pointer (kind 1) whose control
// byte holds sizeBits and whose payload starts at cur. It returns the target
// offset and the offset just past the pointer's encoding.
//
// Pointer classes (per the MMDB spec, including the per-class additive offsets):
//
//	class 0: 1 payload byte,  value = 11 bits
//	class 1: 2 payload bytes, value = 19 bits + 2048
//	class 2: 3 payload bytes, value = 27 bits + 526336
//	class 3: 4 payload bytes, value = 32 bits
func readPointerAt(buf []byte, sizeBits int, cur uint32) (uint32, uint32, error) {
	cls := (sizeBits >> 3) & 0x3
	topBits := uint32(sizeBits & 0x7)
	n := uint32(cls + 1)
	if uint64(cur)+uint64(n) > uint64(len(buf)) {
		return 0, 0, fmt.Errorf("ptr cls%d payload overruns @%d", cls, cur)
	}
	switch cls {
	case 0:
		return topBits<<8 | uint32(buf[cur]), cur + 1, nil
	case 1:
		return (topBits<<16 | uint32(buf[cur])<<8 | uint32(buf[cur+1])) + 2048, cur + 2, nil
	case 2:
		return (topBits<<24 | uint32(buf[cur])<<16 | uint32(buf[cur+1])<<8 | uint32(buf[cur+2])) + 526336, cur + 3, nil
	default:
		return binary.BigEndian.Uint32(buf[cur : cur+4]), cur + 4, nil
	}
}
