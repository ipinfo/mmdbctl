package shrink

import (
	"bytes"
	"fmt"
	"math/rand/v2"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/maxmind/mmdbwriter"
	"github.com/maxmind/mmdbwriter/mmdbtype"
	"github.com/oschwald/maxminddb-golang/v2"
)

// TestShrinkLookupEquivalence is the end-to-end check for the whole
// pipeline: build an MMDB with the upstream writer, shrink it, and confirm
// the output answers every lookup identically to the input. It runs for every
// supported record size and both IP versions.
func TestShrinkLookupEquivalence(t *testing.T) {
	for _, recordSize := range []int{24, 28, 32} {
		for _, ipVersion := range []int{4, 6} {
			name := fmt.Sprintf("rs%d/v%d", recordSize, ipVersion)
			t.Run(name, func(t *testing.T) {
				dir := t.TempDir()
				in := filepath.Join(dir, "in.mmdb")
				out := filepath.Join(dir, "out.mmdb")
				if err := os.WriteFile(in, buildTestMMDB(t, recordSize, ipVersion), 0o644); err != nil {
					t.Fatal(err)
				}

				res, err := Shrink(nil, in, out, Options{})
				if err != nil {
					t.Fatalf("Shrink: %v", err)
				}
				if res.OutputNodeCount >= res.InputNodeCount {
					t.Errorf("expected dedup: nodes %d -> %d", res.InputNodeCount, res.OutputNodeCount)
				}
				if res.OutputBytes >= res.InputBytes {
					t.Errorf("expected smaller output: %d -> %d bytes", res.InputBytes, res.OutputBytes)
				}

				assertEquivalent(t, in, out)

				// Shrinking the output again must be a fixed point.
				again := filepath.Join(dir, "again.mmdb")
				if _, err := Shrink(nil, out, again, Options{}); err != nil {
					t.Fatalf("Shrink(again): %v", err)
				}
				a, _ := os.ReadFile(out)
				b, _ := os.ReadFile(again)
				if !bytes.Equal(a, b) {
					t.Errorf("second shrink pass is not byte-identical (%d vs %d bytes)", len(a), len(b))
				}

				// The debug-only verbatim data-section path must be equivalent too.
				nc := filepath.Join(dir, "nocompact.mmdb")
				if _, err := Shrink(nil, in, nc, Options{DisableCompact: true}); err != nil {
					t.Fatalf("Shrink(DisableCompact): %v", err)
				}
				assertEquivalent(t, in, nc)
			})
		}
	}
}

// buildTestMMDB writes a small database whose trie has plenty of identical
// subtrees: many networks share a handful of records, and records carry
// nested maps and arrays so the data section contains internal pointers.
func buildTestMMDB(t *testing.T, recordSize, ipVersion int) []byte {
	t.Helper()
	w, err := mmdbwriter.New(mmdbwriter.Options{
		DatabaseType:            "mmdbshrink-test",
		Description:             map[string]string{"en": "mmdbshrink equivalence fixture", "fr": "fixture"},
		Languages:               []string{"en", "fr"},
		IPVersion:               ipVersion,
		RecordSize:              recordSize,
		IncludeReservedNetworks: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	records := make([]mmdbtype.Map, 5)
	for i := range records {
		records[i] = mmdbtype.Map{
			"country": mmdbtype.String(fmt.Sprintf("C%d", i)),
			"asn":     mmdbtype.Uint32(64500 + uint32(i)),
			"geo": mmdbtype.Map{
				"city":  mmdbtype.String(fmt.Sprintf("city-%d", i)),
				"names": mmdbtype.Map{"en": mmdbtype.String("shared-name"), "fr": mmdbtype.String("shared-name")},
			},
			"tags":  mmdbtype.Slice{mmdbtype.String("a"), mmdbtype.String("b"), mmdbtype.Uint16(uint16(i))},
			"proxy": mmdbtype.Bool(i%2 == 0),
		}
	}
	insert := func(cidr string, rec mmdbtype.Map) {
		_, n, err := net.ParseCIDR(cidr)
		if err != nil {
			t.Fatal(err)
		}
		if err := w.Insert(n, rec); err != nil {
			t.Fatalf("insert %s: %v", cidr, err)
		}
	}
	for i := 0; i < 256; i++ {
		rec := records[i%len(records)]
		insert(fmt.Sprintf("10.%d.0.0/16", i), rec)
		// A repeating pattern of /24s inside every other /16 gives deep,
		// identical subtrees for the hash-consing pass to merge.
		if i%2 == 0 {
			for j := 0; j < 8; j++ {
				insert(fmt.Sprintf("10.%d.%d.0/24", i, j*32), records[(i+j)%len(records)])
			}
		}
		if ipVersion == 6 {
			insert(fmt.Sprintf("2001:db8:%x::/48", i), rec)
			insert(fmt.Sprintf("2001:db8:%x:8000::/52", i), records[(i+1)%len(records)])
		}
	}
	// A couple of odd-sized networks at the extremes of the address space.
	insert("0.0.0.0/8", records[0])
	insert("255.0.0.0/8", records[1])
	insert("192.0.2.128/25", records[2])
	if ipVersion == 6 {
		insert("2c0f::/16", records[3])
		insert("ff00::/8", records[4])
	}

	var buf bytes.Buffer
	if _, err := w.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// assertEquivalent checks metadata, every network with its record, and a
// battery of individual lookups agree between two MMDB files.
func assertEquivalent(t *testing.T, basePath, otherPath string) {
	t.Helper()
	base, err := maxminddb.Open(basePath)
	if err != nil {
		t.Fatal(err)
	}
	defer base.Close()
	other, err := maxminddb.Open(otherPath)
	if err != nil {
		t.Fatalf("open shrunk output: %v", err)
	}
	defer other.Close()

	if err := other.Verify(); err != nil {
		t.Errorf("reader Verify() on shrunk output: %v", err)
	}

	// Metadata: identical except node_count.
	bm, om := base.Metadata, other.Metadata
	if om.NodeCount >= bm.NodeCount {
		t.Errorf("node_count %d -> %d, expected a decrease", bm.NodeCount, om.NodeCount)
	}
	om.NodeCount = bm.NodeCount
	if !reflect.DeepEqual(bm, om) {
		t.Errorf("metadata differs:\n base: %+v\nother: %+v", bm, om)
	}

	// Full network enumeration: same prefixes, same records, same order.
	type entry struct {
		prefix netip.Prefix
		record any
	}
	collect := func(r *maxminddb.Reader) []entry {
		var out []entry
		for res := range r.Networks() {
			var rec any
			if err := res.Decode(&rec); err != nil {
				t.Fatalf("decode %s: %v", res.Prefix(), err)
			}
			out = append(out, entry{res.Prefix(), rec})
		}
		return out
	}
	be, oe := collect(base), collect(other)
	if len(be) == 0 {
		t.Fatal("baseline enumerated no networks")
	}
	if len(be) != len(oe) {
		t.Fatalf("network count differs: %d vs %d", len(be), len(oe))
	}
	for i := range be {
		if be[i].prefix != oe[i].prefix || !reflect.DeepEqual(be[i].record, oe[i].record) {
			t.Fatalf("network %d differs:\n base: %s %v\nother: %s %v",
				i, be[i].prefix, be[i].record, oe[i].prefix, oe[i].record)
		}
	}

	// Point lookups: first, middle and last address of every enumerated
	// prefix, a fixed set of edge addresses, and a seeded random sample.
	var probes []netip.Addr
	for _, e := range be {
		first := e.prefix.Addr()
		last := lastAddr(e.prefix)
		probes = append(probes, first, midAddr(first, last), last)
	}
	for _, s := range []string{
		"0.0.0.0", "255.255.255.255", "127.0.0.1", "10.0.0.0", "10.255.255.255",
		"::", "::1", "::ffff:10.1.2.3", "2001:db8::", "2002:a01:203::", "ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff",
	} {
		probes = append(probes, netip.MustParseAddr(s))
	}
	rng := rand.New(rand.NewPCG(1, 2))
	for i := 0; i < 2000; i++ {
		var b4 [4]byte
		var b16 [16]byte
		for j := range b4 {
			b4[j] = byte(rng.UintN(256))
		}
		for j := range b16 {
			b16[j] = byte(rng.UintN(256))
		}
		probes = append(probes, netip.AddrFrom4(b4), netip.AddrFrom16(b16))
	}
	for _, ip := range probes {
		br, or := base.Lookup(ip), other.Lookup(ip)
		if (br.Err() == nil) != (or.Err() == nil) {
			t.Fatalf("lookup %s: error mismatch: base=%v other=%v", ip, br.Err(), or.Err())
		}
		if br.Err() != nil {
			continue // both fail (e.g. IPv6 address in an IPv4-only database)
		}
		if br.Found() != or.Found() || br.Prefix() != or.Prefix() {
			t.Fatalf("lookup %s: found=%v/%v prefix=%s/%s", ip, br.Found(), or.Found(), br.Prefix(), or.Prefix())
		}
		if !br.Found() {
			continue
		}
		var brec, orec any
		if err := br.Decode(&brec); err != nil {
			t.Fatalf("decode base %s: %v", ip, err)
		}
		if err := or.Decode(&orec); err != nil {
			t.Fatalf("decode other %s: %v", ip, err)
		}
		if !reflect.DeepEqual(brec, orec) {
			t.Fatalf("lookup %s: record differs:\n base: %v\nother: %v", ip, brec, orec)
		}
	}
}

func lastAddr(p netip.Prefix) netip.Addr {
	b := p.Addr().AsSlice()
	for bit := p.Bits(); bit < len(b)*8; bit++ {
		b[bit/8] |= 0x80 >> (bit % 8)
	}
	a, _ := netip.AddrFromSlice(b)
	return a
}

func midAddr(first, last netip.Addr) netip.Addr {
	f, l := first.AsSlice(), last.AsSlice()
	// Average byte-wise from the most significant end with carry; good
	// enough to land strictly inside the prefix for anything wider than /31.
	out := make([]byte, len(f))
	var carry uint16
	for i := len(f) - 1; i >= 0; i-- {
		sum := uint16(f[i]) + uint16(l[i]) + carry
		out[i] = byte(sum)
		carry = sum >> 8
	}
	// Divide by two.
	var rem uint16
	for i := 0; i < len(out); i++ {
		cur := rem<<8 | uint16(out[i])
		out[i] = byte(cur >> 1)
		rem = cur & 1
	}
	if carry != 0 {
		out[0] |= 0x80
	}
	a, _ := netip.AddrFromSlice(out)
	return a
}
