package verify

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/oschwald/maxminddb-golang/v2"
	"github.com/oschwald/maxminddb-golang/v2/mmdbdata"
)

// countRecords walks every prefix without decoding any record, so it costs
// only the trie traversal.
func countRecords(db *maxminddb.Reader, opts []maxminddb.NetworksOption) (uint64, error) {
	var n uint64
	for result := range db.NetworksWithin(rootPrefix(db), opts...) {
		if err := result.Err(); err != nil {
			return 0, err
		}
		n++
	}
	return n, nil
}

func TestMetadataEquivalentChecksReleaseMetadataByDefault(t *testing.T) {
	base := maxminddb.Metadata{
		Description:              map[string]string{"en": "Smoke test"},
		DatabaseType:             "Smoke-Test",
		Languages:                []string{"en"},
		BinaryFormatMajorVersion: 2,
		BinaryFormatMinorVersion: 0,
		BuildEpoch:               42,
		IPVersion:                4,
		NodeCount:                100,
		RecordSize:               24,
	}

	sameExceptNodeCount := base
	sameExceptNodeCount.NodeCount = 10
	if ok, msg := metadataEquivalent(base, sameExceptNodeCount); !ok {
		t.Fatalf("metadataEquivalent should allow node_count changes, got %q", msg)
	}

	tests := []struct {
		name string
		edit func(*maxminddb.Metadata)
		want string
	}{
		{
			name: "database_type",
			edit: func(m *maxminddb.Metadata) {
				m.DatabaseType = "Different-Test"
			},
			want: "database_type differs",
		},
		{
			name: "description",
			edit: func(m *maxminddb.Metadata) {
				m.Description = map[string]string{"en": "Different description"}
			},
			want: "description differs",
		},
		{
			name: "build_epoch",
			edit: func(m *maxminddb.Metadata) {
				m.BuildEpoch = 43
			},
			want: "build_epoch differs",
		},
		{
			name: "ip_version",
			edit: func(m *maxminddb.Metadata) {
				m.IPVersion = 6
			},
			want: "ip_version differs",
		},
		{
			name: "record_size",
			edit: func(m *maxminddb.Metadata) {
				m.RecordSize = 28
			},
			want: "record_size differs",
		},
		{
			name: "languages",
			edit: func(m *maxminddb.Metadata) {
				m.Languages = []string{"en", "pt"}
			},
			want: "languages differs",
		},
		{
			name: "binary_format_major_version",
			edit: func(m *maxminddb.Metadata) {
				m.BinaryFormatMajorVersion = 3
			},
			want: "binary_format_major_version differs",
		},
		{
			name: "binary_format_minor_version",
			edit: func(m *maxminddb.Metadata) {
				m.BinaryFormatMinorVersion = 1
			},
			want: "binary_format_minor_version differs",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			other := base
			other.Description = map[string]string{"en": "Smoke test"}
			tt.edit(&other)

			ok, msg := metadataEquivalent(base, other)
			if ok {
				t.Fatalf("metadataEquivalent returned ok for %s change", tt.name)
			}
			if !strings.Contains(msg, tt.want) {
				t.Fatalf("metadataEquivalent msg = %q, want to contain %q", msg, tt.want)
			}
		})
	}
}

func TestMetadataEquivalentTreatsNilAndEmptyAlike(t *testing.T) {
	a := maxminddb.Metadata{DatabaseType: "T", IPVersion: 4, RecordSize: 24}
	b := a
	b.Languages = []string{}
	b.Description = map[string]string{}
	if ok, msg := metadataEquivalent(a, b); !ok {
		t.Fatalf("nil and empty collections should compare equal, got %q", msg)
	}
	b.Languages = []string{"en"}
	if ok, msg := metadataEquivalent(a, b); ok || !strings.Contains(msg, "languages differs") {
		t.Fatalf("got ok=%v msg=%q, want a languages mismatch", ok, msg)
	}
}

func TestCompareKeySetsNamesTheFirstMissingKeyInOrder(t *testing.T) {
	a := map[string]int{"$.b=string": 1, "$.a=string": 1, "$.c=string": 1}
	b := map[string]int{"$.c=string": 1}
	for range 20 {
		if _, msg := compareKeySets("schema entry", a, b); msg != "schema entry $.a=string present only in baseline" {
			t.Fatalf("msg = %q, want the smallest missing key every time", msg)
		}
	}
}

func TestCountRecordsUsesIPv4RootForIPv4OnlyDB(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tiny-ipv4.mmdb")
	if err := os.WriteFile(path, tinyIPv4MMDB(), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	db, err := maxminddb.Open(path)
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer db.Close()

	if got := rootPrefix(db).String(); got != "0.0.0.0/0" {
		t.Fatalf("rootPrefix = %s, want 0.0.0.0/0", got)
	}
	got, err := countRecords(db, nil)
	if err != nil {
		t.Fatalf("countRecords: %v", err)
	}
	if got != 2 {
		t.Fatalf("countRecords = %d, want 2", got)
	}
}

func TestFixedProbesIncludeIPv6OnlyForIPv6DBs(t *testing.T) {
	v4 := fixedProbes(4)
	for _, probe := range v4 {
		if !probe.ip.Is4() {
			t.Fatalf("IPv4 probe set contains non-IPv4 address %s", probe.ip)
		}
	}

	v6 := fixedProbes(6)
	if len(v6) <= len(v4) {
		t.Fatalf("IPv6 probe set len=%d, want more than IPv4 len=%d", len(v6), len(v4))
	}
	hasV6 := false
	for _, probe := range v6 {
		if probe.ip.Is6() && !probe.ip.Is4() {
			hasV6 = true
			break
		}
	}
	if !hasV6 {
		t.Fatal("IPv6 probe set has no IPv6-only addresses")
	}
}

func TestSchemaComparisonIgnoresOccurrenceCounts(t *testing.T) {
	a := map[string]int{"$.service=string": 10}
	b := map[string]int{"$.service=string": 1}
	if ok, msg := compareKeySets("schema entry", a, b); !ok {
		t.Fatalf("compareKeySets should ignore counts, got %q", msg)
	}
	if ok, _ := compareKeySets("schema entry", a, map[string]int{"$.other=string": 1}); ok {
		t.Fatal("compareKeySets returned ok for different schema keys")
	}
}

func TestCompareExactPrefixStreams(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tiny-ipv4.mmdb")
	if err := os.WriteFile(path, tinyIPv4MMDB(), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	a, err := maxminddb.Open(path)
	if err != nil {
		t.Fatalf("open a: %v", err)
	}
	defer a.Close()
	b, err := maxminddb.Open(path)
	if err != nil {
		t.Fatalf("open b: %v", err)
	}
	defer b.Close()

	countA, countB, ok, msg, err := compareExactPrefixStreams(a, b, nil, 0)
	if err != nil {
		t.Fatalf("compareExactPrefixStreams: %v", err)
	}
	if !ok {
		t.Fatalf("compareExactPrefixStreams mismatch: %s", msg)
	}
	if countA != 2 || countB != 2 {
		t.Fatalf("counts = %d/%d, want 2/2", countA, countB)
	}
}

// TestCompareExactPrefixStreamsDetectsMismatch proves the prefix-stream
// comparison actually reports a mismatch when the two DBs expose different
// prefix sets. The control fixture exposes two data prefixes (0.0.0.0/1 and
// 128.0.0.0/1); the variant routes the second half to the empty record, so it
// exposes only 0.0.0.0/1. compareExactPrefixStreams must return ok==false.
func TestCompareExactPrefixStreamsDetectsMismatch(t *testing.T) {
	dir := t.TempDir()
	pathA := filepath.Join(dir, "two-prefixes.mmdb")
	if err := os.WriteFile(pathA, tinyIPv4MMDB(), 0o644); err != nil {
		t.Fatalf("write fixture A: %v", err)
	}
	pathB := filepath.Join(dir, "one-prefix.mmdb")
	if err := os.WriteFile(pathB, tinyIPv4MMDBOnePrefix(), 0o644); err != nil {
		t.Fatalf("write fixture B: %v", err)
	}

	a, err := maxminddb.Open(pathA)
	if err != nil {
		t.Fatalf("open a: %v", err)
	}
	defer a.Close()
	b, err := maxminddb.Open(pathB)
	if err != nil {
		t.Fatalf("open b: %v", err)
	}
	defer b.Close()

	// Sanity: the fixtures really do expose different prefix counts.
	nA, err := countRecords(a, nil)
	if err != nil {
		t.Fatalf("countRecords a: %v", err)
	}
	nB, err := countRecords(b, nil)
	if err != nil {
		t.Fatalf("countRecords b: %v", err)
	}
	if nA == nB {
		t.Fatalf("fixtures should differ, both expose %d prefixes", nA)
	}

	_, _, ok, msg, err := compareExactPrefixStreams(a, b, nil, 0)
	if err != nil {
		t.Fatalf("compareExactPrefixStreams: %v", err)
	}
	if ok {
		t.Fatal("compareExactPrefixStreams returned ok for differing prefix sets")
	}
	if msg == "" {
		t.Fatal("compareExactPrefixStreams reported a mismatch with an empty message")
	}
}

func TestCanonicalRecordIgnoresMapOrderAndCapturesSchema(t *testing.T) {
	left := bytes.Join([][]byte{
		mmdbCtrl(7, 2),
		mmdbString("b"), mmdbUint16(2),
		mmdbString("a"), mmdbString("x"),
	}, nil)
	right := bytes.Join([][]byte{
		mmdbCtrl(7, 2),
		mmdbString("a"), mmdbString("x"),
		mmdbString("b"), mmdbUint16(2),
	}, nil)

	leftSchema := map[string]int{}
	leftRecord := canonicalRecord{schema: leftSchema}
	if err := leftRecord.UnmarshalMaxMindDB(mmdbdata.NewDecoder(left, 0)); err != nil {
		t.Fatalf("left canonical decode: %v", err)
	}
	rightRecord := canonicalRecord{}
	if err := rightRecord.UnmarshalMaxMindDB(mmdbdata.NewDecoder(right, 0)); err != nil {
		t.Fatalf("right canonical decode: %v", err)
	}

	if !bytes.Equal(leftRecord.data, rightRecord.data) {
		t.Fatalf("canonical data should ignore map order:\nleft:  %x\nright: %x", leftRecord.data, rightRecord.data)
	}
	for _, key := range []string{"$=map", "$.a=string", "$.b=uint16"} {
		if leftSchema[key] == 0 {
			t.Fatalf("schema missing %q in %#v", key, leftSchema)
		}
	}
}

// canonicalizeMMDB runs an encoded MMDB value through the canonical-record
// machinery and returns the canonical bytes verify compares.
func canonicalizeMMDB(t *testing.T, encoded []byte) []byte {
	t.Helper()
	var rec canonicalRecord
	if err := rec.UnmarshalMaxMindDB(mmdbdata.NewDecoder(encoded, 0)); err != nil {
		t.Fatalf("canonical decode: %v", err)
	}
	return append([]byte(nil), rec.data...)
}

// TestCanonicalRecordDistinguishesDifferentValues guards against an
// over-canonicalization regression: if the canonical encoding ever dropped a
// value payload, verify would rubber-stamp differing records as equal. Each
// pair below encodes semantically different data and must canonicalize to
// different bytes.
func TestCanonicalRecordDistinguishesDifferentValues(t *testing.T) {
	cases := []struct {
		name string
		a, b []byte
	}{
		{
			name: "string A vs B",
			a:    mmdbString("A"),
			b:    mmdbString("B"),
		},
		{
			name: "uint16 1 vs 2",
			a:    mmdbUint16(1),
			b:    mmdbUint16(2),
		},
		{
			name: "map value 1 vs 2",
			a:    mmdbMap(map[string][]byte{"a": mmdbUint16(1)}),
			b:    mmdbMap(map[string][]byte{"a": mmdbUint16(2)}),
		},
		{
			name: "array element differs",
			a:    mmdbArray(mmdbString("x"), mmdbString("y")),
			b:    mmdbArray(mmdbString("x"), mmdbString("z")),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ca := canonicalizeMMDB(t, tc.a)
			cb := canonicalizeMMDB(t, tc.b)
			if bytes.Equal(ca, cb) {
				t.Fatalf("canonical bytes should differ for %s:\n a: %x\n b: %x", tc.name, ca, cb)
			}
		})
	}
}

func tinyIPv4MMDB() []byte {
	const (
		nodeCount  = 1
		recordSize = 24
		dataOffset = nodeCount + 16
	)
	tree := []byte{0, 0, dataOffset, 0, 0, dataOffset}
	data := mmdbMap(map[string][]byte{"service": mmdbString("x")})
	metadata := mmdbMap(map[string][]byte{
		"binary_format_major_version": mmdbUint16(2),
		"binary_format_minor_version": mmdbUint16(0),
		"build_epoch":                 mmdbUint64(0),
		"database_type":               mmdbString("Smoke-Test"),
		"description":                 mmdbMap(map[string][]byte{"en": mmdbString("Smoke test")}),
		"ip_version":                  mmdbUint16(4),
		"languages":                   mmdbArray(mmdbString("en")),
		"node_count":                  mmdbUint32(nodeCount),
		"record_size":                 mmdbUint16(recordSize),
	})
	return bytes.Join([][]byte{
		tree,
		make([]byte, 16),
		data,
		append([]byte{0xab, 0xcd, 0xef}, []byte("MaxMind.com")...),
		metadata,
	}, nil)
}

// tinyIPv4MMDBOnePrefix is tinyIPv4MMDB with the right child of the root node
// routed to the empty record (value == node_count) instead of data, so only
// 0.0.0.0/1 carries data. It exposes a single prefix instead of two.
func tinyIPv4MMDBOnePrefix() []byte {
	const (
		nodeCount  = 1
		recordSize = 24
		dataOffset = nodeCount + 16
		emptyValue = nodeCount
	)
	tree := []byte{0, 0, dataOffset, 0, 0, emptyValue}
	data := mmdbMap(map[string][]byte{"service": mmdbString("x")})
	metadata := mmdbMap(map[string][]byte{
		"binary_format_major_version": mmdbUint16(2),
		"binary_format_minor_version": mmdbUint16(0),
		"build_epoch":                 mmdbUint64(0),
		"database_type":               mmdbString("Smoke-Test"),
		"description":                 mmdbMap(map[string][]byte{"en": mmdbString("Smoke test")}),
		"ip_version":                  mmdbUint16(4),
		"languages":                   mmdbArray(mmdbString("en")),
		"node_count":                  mmdbUint32(nodeCount),
		"record_size":                 mmdbUint16(recordSize),
	})
	return bytes.Join([][]byte{
		tree,
		make([]byte, 16),
		data,
		append([]byte{0xab, 0xcd, 0xef}, []byte("MaxMind.com")...),
		metadata,
	}, nil)
}

func mmdbString(s string) []byte {
	body := []byte(s)
	return bytes.Join([][]byte{mmdbCtrl(2, len(body)), body}, nil)
}

func mmdbMap(m map[string][]byte) []byte {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := [][]byte{mmdbCtrl(7, len(keys))}
	for _, k := range keys {
		parts = append(parts, mmdbString(k), m[k])
	}
	return bytes.Join(parts, nil)
}

func mmdbArray(items ...[]byte) []byte {
	parts := [][]byte{mmdbExtendedCtrl(11, len(items))}
	parts = append(parts, items...)
	return bytes.Join(parts, nil)
}

func mmdbUint16(v uint64) []byte { return mmdbUint(v, 5, 2) }
func mmdbUint32(v uint64) []byte { return mmdbUint(v, 6, 4) }
func mmdbUint64(v uint64) []byte { return mmdbUint(v, 9, 8) }

func mmdbUint(v uint64, kind int, maxBytes int) []byte {
	var tmp [8]byte
	binary.BigEndian.PutUint64(tmp[:], v)
	start := 8 - maxBytes
	for start < 8 && tmp[start] == 0 {
		start++
	}
	width := 8 - start
	if kind == 9 {
		return bytes.Join([][]byte{{byte(width), byte(kind - 7)}, tmp[start:]}, nil)
	}
	return bytes.Join([][]byte{mmdbCtrl(kind, width), tmp[start:]}, nil)
}

func mmdbCtrl(kind int, size int) []byte {
	sizeByte, extra := mmdbSize(size)
	return bytes.Join([][]byte{{byte(kind<<5) | sizeByte}, extra}, nil)
}

func mmdbExtendedCtrl(kind int, size int) []byte {
	sizeByte, extra := mmdbSize(size)
	return bytes.Join([][]byte{{sizeByte, byte(kind - 7)}, extra}, nil)
}

func mmdbSize(size int) (byte, []byte) {
	switch {
	case size <= 28:
		return byte(size), nil
	case size <= 29+255:
		return 29, []byte{byte(size - 29)}
	case size <= 285+65535:
		var b [2]byte
		binary.BigEndian.PutUint16(b[:], uint16(size-285))
		return 30, b[:]
	default:
		v := size - 65821
		return 31, []byte{byte(v >> 16), byte(v >> 8), byte(v)}
	}
}
