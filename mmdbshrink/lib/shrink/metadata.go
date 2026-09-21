// Minimal MMDB metadata codec to decode the metadata map, replace node_count,
// and encode it again.
//
// MMDB values are self-describing: each value starts with a "control byte"
// whose top 3 bits encode the type (kind 1..7) and bottom 5 bits encode
// either the payload size (for length-prefixed kinds: string, bytes, map,
// extended-array) or the integer width (for kinds: uint16, uint32, uint64).
// Type 0 means "extended": the next byte's value + 7 is the actual kind.
//
// Metadata in an MMDB is a map (kind 7) of fixed-name keys to values:
//
//	node_count                      uint32     (kind 6)
//	record_size                     uint16     (kind 5)
//	ip_version                      uint16     (kind 5)
//	database_type                   utf8       (kind 2)
//	languages                       array of utf8 (extended kind 11)
//	binary_format_major_version     uint16     (kind 5)
//	binary_format_minor_version     uint16     (kind 5)
//	build_epoch                     uint64     (kind 9)
//	description                     map<utf8,utf8>  (kind 7 of kind 2 to kind 2)
//
// The metadata section may contain pointers (kind 1); the upstream mmdbwriter
// emits them by default for repeated values such as language codes. Their
// targets are offsets relative to the start of the metadata section (the byte
// after the marker). The decoder resolves them into plain values, so the
// re-encoded metadata carries no pointers.

package shrink

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"sort"
)

var metadataStartMarker = []byte("\xAB\xCD\xEFMaxMind.com")

// findMetadata searches buf for an MMDB metadata and returns its starting index
// and the metadata buffer.
// Returns an error if the metadata marker is not found.
func findMetadata(buf []byte) (start uint64, metaBytes []byte, err error) {
	// Scan from the end (the marker is near EOF).
	limit := len(buf) - len(metadataStartMarker)
	for i := limit; i >= 0; i-- {
		if bytes.Equal(buf[i:i+len(metadataStartMarker)], metadataStartMarker) {
			return uint64(i), buf[i+len(metadataStartMarker):], nil
		}
	}
	return 0, nil, errors.New("metadata marker not found")
}

// decodeMetadata reads the bytes that follow the metadata marker and
// returns the top-level map as a map[string]any.
func decodeMetadata(buf []byte) (map[string]any, error) {
	d := &decoder{buf: buf}
	v, err := d.readValue()
	if err != nil {
		return nil, err
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("metadata block top-level is not a map (%T)", v)
	}
	return m, nil
}

type decoder struct {
	buf []byte
	off int
}

func (d *decoder) readValue() (any, error) {
	if d.off >= len(d.buf) {
		return nil, errors.New("decoder: unexpected EOF")
	}
	ctrl := d.buf[d.off]
	d.off++
	kind := int(ctrl >> 5)
	size := int(ctrl & 0x1f)
	if kind == 0 {
		// Extended type
		if d.off >= len(d.buf) {
			return nil, errors.New("decoder: extended type byte missing")
		}
		ext := d.buf[d.off]
		d.off++
		kind = int(ext) + 7
	}

	// For kinds 1..7, size 29 means "1 extra byte for length", 30 = 2, 31 = 3.
	// For kinds with payload-as-int-width (5,6,9,8,10), size is the byte width
	// directly (no extension).
	switch kind {
	case 1: // pointer
		target, next, err := readPointerAt(d.buf, size, uint32(d.off))
		if err != nil {
			return nil, fmt.Errorf("decoder: %w", err)
		}
		d.off = int(next)
		if int(target) >= len(d.buf) {
			return nil, fmt.Errorf("decoder: pointer target %d past metadata (%d)", target, len(d.buf))
		}
		if d.buf[target]>>5 == 1 {
			return nil, fmt.Errorf("decoder: pointer @%d targets another pointer", target)
		}
		sub := &decoder{buf: d.buf, off: int(target)}
		return sub.readValue()
	case 2, 4: // utf8 string, bytes
		n, err := d.readSize(size)
		if err != nil {
			return nil, err
		}
		if d.off+n > len(d.buf) {
			return nil, fmt.Errorf("decoder: string/bytes len %d exceeds buf", n)
		}
		s := string(d.buf[d.off : d.off+n])
		d.off += n
		if kind == 4 {
			return []byte(s), nil
		}
		return s, nil
	case 5: // uint16
		v, err := d.readUintN(size, 2)
		if err != nil {
			return nil, err
		}
		return uint16(v), nil
	case 6: // uint32
		v, err := d.readUintN(size, 4)
		if err != nil {
			return nil, err
		}
		return uint32(v), nil
	case 7: // map
		n, err := d.readSize(size)
		if err != nil {
			return nil, err
		}
		m := make(map[string]any, n)
		for i := 0; i < n; i++ {
			kv, err := d.readValue()
			if err != nil {
				return nil, fmt.Errorf("map key %d: %w", i, err)
			}
			ks, ok := kv.(string)
			if !ok {
				return nil, fmt.Errorf("map key %d not a string (%T)", i, kv)
			}
			vv, err := d.readValue()
			if err != nil {
				return nil, fmt.Errorf("map value for %q: %w", ks, err)
			}
			m[ks] = vv
		}
		return m, nil
	case 9: // uint64
		v, err := d.readUintN(size, 8)
		if err != nil {
			return nil, err
		}
		return v, nil
	case 11: // array (extended)
		n, err := d.readSize(size)
		if err != nil {
			return nil, err
		}
		a := make([]any, n)
		for i := 0; i < n; i++ {
			v, err := d.readValue()
			if err != nil {
				return nil, fmt.Errorf("array elem %d: %w", i, err)
			}
			a[i] = v
		}
		return a, nil
	case 14: // boolean (extended) — size is 0 or 1
		return size != 0, nil
	default:
		return nil, fmt.Errorf("decoder: unsupported kind %d", kind)
	}
}

// readSize decodes a length/count prefix
func (d *decoder) readSize(size int) (int, error) {
	n, next, err := readSizeAt(d.buf, size, uint32(d.off))
	if err != nil {
		return 0, err
	}
	d.off = int(next)
	return int(n), nil
}

// readUintN decodes an N-byte big-endian unsigned int into a uint64. The
// MMDB encoding uses size byte == byte length (0..maxBytes).
func (d *decoder) readUintN(size, maxBytes int) (uint64, error) {
	if size > maxBytes {
		return 0, fmt.Errorf("readUintN: size %d > max %d", size, maxBytes)
	}
	if d.off+size > len(d.buf) {
		return 0, fmt.Errorf("readUintN: need %d bytes have %d", size, len(d.buf)-d.off)
	}
	var v uint64
	for i := 0; i < size; i++ {
		v = (v << 8) | uint64(d.buf[d.off+i])
	}
	d.off += size
	return v, nil
}

// encodeMetadata serializes the given map back to MMDB metadata bytes.
// Output is a sequence: [outer-map control byte+ext if needed][entries...].
func encodeMetadata(m map[string]any) ([]byte, error) {
	e := &encoder{}
	if err := e.writeMap(m); err != nil {
		return nil, err
	}
	return e.buf, nil
}

type encoder struct {
	buf []byte
}

func (e *encoder) writeValue(v any) error {
	switch x := v.(type) {
	case string:
		return e.writeString(x)
	case []byte:
		return e.writeBytes(x)
	case uint64:
		return e.writeUintN(x, 9, 8)
	case uint32:
		return e.writeUintN(uint64(x), 6, 4)
	case uint16:
		return e.writeUintN(uint64(x), 5, 2)
	case bool:
		// boolean (extended kind 14): size 0 = false, 1 = true
		var b byte
		if x {
			b = 0x01
		}
		// type byte: kind=0 (extended), size = b
		e.buf = append(e.buf, byte(b))
		// extended type byte: 14 - 7 = 7
		e.buf = append(e.buf, 0x07)
		return nil
	case map[string]any:
		return e.writeMap(x)
	case []any:
		return e.writeArray(x)
	case []string:
		// Convenience: same as []any of strings.
		a := make([]any, len(x))
		for i, s := range x {
			a[i] = s
		}
		return e.writeArray(a)
	default:
		return fmt.Errorf("encoder: unsupported type %T", v)
	}
}

func (e *encoder) writeString(s string) error {
	e.writeCtrl(2 /*string*/, len(s), false)
	e.buf = append(e.buf, []byte(s)...)
	return nil
}

func (e *encoder) writeBytes(b []byte) error {
	e.writeCtrl(4 /*bytes*/, len(b), false)
	e.buf = append(e.buf, b...)
	return nil
}

func (e *encoder) writeMap(m map[string]any) error {
	// Sort keys for stable output.
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	e.writeCtrl(7 /*map*/, len(m), false)
	for _, k := range keys {
		if err := e.writeString(k); err != nil {
			return err
		}
		if err := e.writeValue(m[k]); err != nil {
			return fmt.Errorf("encode map[%q]: %w", k, err)
		}
	}
	return nil
}

func (e *encoder) writeArray(a []any) error {
	e.writeCtrl(11 /*array, extended*/, len(a), true)
	for i, v := range a {
		if err := e.writeValue(v); err != nil {
			return fmt.Errorf("encode array[%d]: %w", i, err)
		}
	}
	return nil
}

// writeUintN writes an unsigned integer using the MMDB encoding for the
// given kind (5/6/9/etc). Strips leading zero bytes per spec.
func (e *encoder) writeUintN(v uint64, kind int, maxBytes int) error {
	// Find minimum byte width.
	var tmp [8]byte
	binary.BigEndian.PutUint64(tmp[:], v)
	start := 8 - maxBytes
	for start < 8 && tmp[start] == 0 {
		start++
	}
	width := 8 - start
	if width > maxBytes {
		return fmt.Errorf("uint%d overflow: %d", maxBytes*8, v)
	}
	// kind 5/6: regular kind, width fits in size bits (0..maxBytes).
	// kind 9: "extended" — type byte = 0x00, ext byte = kind - 7 = 2
	if kind == 9 {
		e.buf = append(e.buf, byte(width)) // type 0 (extended), size = width
		e.buf = append(e.buf, 0x02)        // ext = kind 9 - 7 = 2
		e.buf = append(e.buf, tmp[start:]...)
		return nil
	}
	// kinds 5, 6 are non-extended:
	if width > 28 {
		return fmt.Errorf("integer width %d > 28 (would need extended size)", width)
	}
	e.buf = append(e.buf, byte(kind<<5)|byte(width))
	e.buf = append(e.buf, tmp[start:]...)
	return nil
}

// writeCtrl writes the control byte (and extended size encoding if needed)
// for a length/count-prefixed kind (string, bytes, map, array).
func (e *encoder) writeCtrl(kind int, size int, extended bool) {
	var sizeByte byte
	switch {
	case size <= 28:
		sizeByte = byte(size)
	case size <= 29+255:
		sizeByte = 29
	case size <= 285+65535:
		sizeByte = 30
	default:
		sizeByte = 31
	}
	if extended {
		// type byte: kind=0, size = our size byte
		e.buf = append(e.buf, sizeByte)
		// ext byte: actual kind - 7
		e.buf = append(e.buf, byte(kind-7))
	} else {
		e.buf = append(e.buf, byte(kind<<5)|sizeByte)
	}
	// Size payload
	switch sizeByte {
	case 29:
		e.buf = append(e.buf, byte(size-29))
	case 30:
		v := uint16(size - 285)
		e.buf = append(e.buf, byte(v>>8), byte(v))
	case 31:
		v := uint32(size - 65821)
		e.buf = append(e.buf, byte(v>>16), byte(v>>8), byte(v))
	}
}
