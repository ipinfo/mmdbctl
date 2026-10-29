package shrink

import (
	"reflect"
	"testing"
)

// FuzzDecodeMetadata checks that the metadata decoder never panics on
// arbitrary bytes and that a successfully decoded metadata block round-trips
// through the encoder back to the same values.
func FuzzDecodeMetadata(f *testing.F) {
	seeds := [][]byte{
		{0xE1, 0x41, 'a', 0x42, 'b'},
		{0xE2, 0x41, 'a', 0x42, 'b', 0x41, 'c', 0x42, 'd'},
		{0x00},
		{0xFF, 0xFF, 0xFF},
		{},
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		m, err := decodeMetadata(data)
		if err != nil {
			return
		}
		if m == nil {
			t.Fatalf("decodeMetadata returned a nil map with no error")
		}

		// A decoded block must re-encode and decode to the same values.
		enc, err := encodeMetadata(m)
		if err != nil {
			t.Fatalf("encodeMetadata after a successful decode: %v", err)
		}
		m2, err := decodeMetadata(enc)
		if err != nil {
			t.Fatalf("decodeMetadata of the re-encoded block: %v", err)
		}
		if !reflect.DeepEqual(m, m2) {
			t.Fatalf("round trip changed the metadata:\n got %#v\nwant %#v", m2, m)
		}
	})
}
