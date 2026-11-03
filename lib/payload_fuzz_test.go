package lib

import (
	"bytes"
	"testing"
)

// FuzzPayloadCalculation checks the MMDB size-prefix decoding against the
// formulas in the spec, for the three extended sizes.
func FuzzPayloadCalculation(f *testing.F) {
	f.Add([]byte{0x01}, 29)
	f.Add([]byte{0x01, 0x02}, 30)
	f.Add([]byte{0x01, 0x02, 0x03}, 31)
	f.Add([]byte{}, 0)

	f.Fuzz(func(t *testing.T, data []byte, payloadSize int) {
		r := bytes.NewReader(data)
		got, offset, err := payloadCalculation(r, payloadSize, 0)
		if err != nil {
			return
		}

		switch payloadSize {
		case 29:
			if len(data) < 1 {
				t.Fatalf("size 29 succeeded with %d bytes available", len(data))
			}
			if want := 29 + int(data[0]); got != want {
				t.Fatalf("size 29: got %d, want %d", got, want)
			}
			if offset != 1 {
				t.Fatalf("size 29: offset %d, want 1", offset)
			}
		case 30:
			if len(data) < 2 {
				t.Fatalf("size 30 succeeded with %d bytes available", len(data))
			}
			if want := 285 + int(data[0])<<8 + int(data[1]); got != want {
				t.Fatalf("size 30: got %d, want %d", got, want)
			}
			if offset != 2 {
				t.Fatalf("size 30: offset %d, want 2", offset)
			}
		case 31:
			if len(data) < 3 {
				t.Fatalf("size 31 succeeded with %d bytes available", len(data))
			}
			if want := 65821 + int(data[0])<<16 + int(data[1])<<8 + int(data[2]); got != want {
				t.Fatalf("size 31: got %d, want %d", got, want)
			}
			if offset != 3 {
				t.Fatalf("size 31: offset %d, want 3", offset)
			}
		default:
			if got != payloadSize {
				t.Fatalf("size %d: got %d, want it unchanged", payloadSize, got)
			}
			if offset != 0 {
				t.Fatalf("size %d: offset %d, want 0", payloadSize, offset)
			}
		}
	})
}
