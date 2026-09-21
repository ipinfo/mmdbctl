package verify

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"maps"
	"math"
	"net/netip"
	"reflect"
	"slices"
	"sort"
	"strings"

	"github.com/oschwald/maxminddb-golang/v2"
	"github.com/oschwald/maxminddb-golang/v2/mmdbdata"
)

// canonicalRecord is a record decoded straight from MMDB values into a
// canonical byte string, so two records compare with bytes.Equal regardless
// of map key order and without materialising Go maps. When schema is non-nil
// every key path and value type seen is counted into it.
type canonicalRecord struct {
	data   []byte
	schema map[string]int
}

func (r *canonicalRecord) UnmarshalMaxMindDB(d *mmdbdata.Decoder) error {
	r.data = r.data[:0]
	data, err := appendCanonicalMMDBValue(r.data, d, "$", r.schema)
	if err != nil {
		return err
	}
	r.data = data
	return nil
}

func (r canonicalRecord) found() bool {
	return len(r.data) > 0
}

type canonicalMapEntry struct {
	key   []byte
	value []byte
}

func appendCanonicalMMDBValue(dst []byte, d *mmdbdata.Decoder, path string, schema map[string]int) ([]byte, error) {
	kind, err := d.PeekKind()
	if err != nil {
		return dst, err
	}
	if schema != nil {
		schema[path+"="+schemaKindName(kind)]++
	}

	switch kind {
	case mmdbdata.KindMap:
		iter, size, err := d.ReadMap()
		if err != nil {
			return dst, err
		}
		entries := make([]canonicalMapEntry, 0, size)
		for key, err := range iter {
			if err != nil {
				return dst, err
			}
			value, err := appendCanonicalMMDBValue(nil, d, path+"."+string(key), schema)
			if err != nil {
				return dst, err
			}
			entries = append(entries, canonicalMapEntry{
				key:   append([]byte(nil), key...),
				value: value,
			})
		}
		sort.Slice(entries, func(i, j int) bool {
			if cmp := bytes.Compare(entries[i].key, entries[j].key); cmp != 0 {
				return cmp < 0
			}
			return bytes.Compare(entries[i].value, entries[j].value) < 0
		})
		dst = append(dst, 'm')
		dst = binary.AppendUvarint(dst, uint64(len(entries)))
		for _, entry := range entries {
			dst = appendLenBytes(dst, entry.key)
			dst = appendLenBytes(dst, entry.value)
		}
		return dst, nil
	case mmdbdata.KindSlice:
		iter, size, err := d.ReadSlice()
		if err != nil {
			return dst, err
		}
		dst = append(dst, 'a')
		dst = binary.AppendUvarint(dst, uint64(size))
		for err := range iter {
			if err != nil {
				return dst, err
			}
			dst, err = appendCanonicalMMDBValue(dst, d, path+"[]", schema)
			if err != nil {
				return dst, err
			}
		}
		return dst, nil
	case mmdbdata.KindString:
		v, err := d.ReadString()
		if err != nil {
			return dst, err
		}
		dst = append(dst, 's')
		dst = binary.AppendUvarint(dst, uint64(len(v)))
		return append(dst, v...), nil
	case mmdbdata.KindBytes:
		v, err := d.ReadBytes()
		if err != nil {
			return dst, err
		}
		dst = append(dst, 'b')
		return appendLenBytes(dst, v), nil
	case mmdbdata.KindBool:
		v, err := d.ReadBool()
		if err != nil {
			return dst, err
		}
		dst = append(dst, 't')
		if v {
			return append(dst, 1), nil
		}
		return append(dst, 0), nil
	case mmdbdata.KindFloat32:
		v, err := d.ReadFloat32()
		if err != nil {
			return dst, err
		}
		var buf [4]byte
		binary.BigEndian.PutUint32(buf[:], math.Float32bits(v))
		return append(append(dst, 'f'), buf[:]...), nil
	case mmdbdata.KindFloat64:
		v, err := d.ReadFloat64()
		if err != nil {
			return dst, err
		}
		var buf [8]byte
		binary.BigEndian.PutUint64(buf[:], math.Float64bits(v))
		return append(append(dst, 'F'), buf[:]...), nil
	case mmdbdata.KindInt32:
		v, err := d.ReadInt32()
		if err != nil {
			return dst, err
		}
		var buf [4]byte
		binary.BigEndian.PutUint32(buf[:], uint32(v))
		return append(append(dst, 'i'), buf[:]...), nil
	case mmdbdata.KindUint16:
		v, err := d.ReadUint16()
		if err != nil {
			return dst, err
		}
		var buf [2]byte
		binary.BigEndian.PutUint16(buf[:], v)
		return append(append(dst, 'w'), buf[:]...), nil
	case mmdbdata.KindUint32:
		v, err := d.ReadUint32()
		if err != nil {
			return dst, err
		}
		var buf [4]byte
		binary.BigEndian.PutUint32(buf[:], v)
		return append(append(dst, 'u'), buf[:]...), nil
	case mmdbdata.KindUint64:
		v, err := d.ReadUint64()
		if err != nil {
			return dst, err
		}
		var buf [8]byte
		binary.BigEndian.PutUint64(buf[:], v)
		return append(append(dst, 'U'), buf[:]...), nil
	case mmdbdata.KindUint128:
		hi, lo, err := d.ReadUint128()
		if err != nil {
			return dst, err
		}
		var buf [16]byte
		binary.BigEndian.PutUint64(buf[:8], hi)
		binary.BigEndian.PutUint64(buf[8:], lo)
		return append(append(dst, 'Q'), buf[:]...), nil
	default:
		return dst, fmt.Errorf("unsupported MMDB kind %s at %s", kind, path)
	}
}

func appendLenBytes(dst, v []byte) []byte {
	dst = binary.AppendUvarint(dst, uint64(len(v)))
	return append(dst, v...)
}

func schemaKindName(kind mmdbdata.Kind) string {
	switch kind {
	case mmdbdata.KindMap:
		return "map"
	case mmdbdata.KindSlice:
		return "array"
	case mmdbdata.KindString:
		return "string"
	case mmdbdata.KindBytes:
		return "bytes"
	case mmdbdata.KindBool:
		return "bool"
	case mmdbdata.KindFloat32:
		return "float32"
	case mmdbdata.KindFloat64:
		return "float64"
	case mmdbdata.KindInt32:
		return "int32"
	case mmdbdata.KindUint16:
		return "uint16"
	case mmdbdata.KindUint32:
		return "uint32"
	case mmdbdata.KindUint64:
		return "uint64"
	case mmdbdata.KindUint128:
		return "uint128"
	default:
		return kind.String()
	}
}

// metadataEquivalent compares every metadata field except node_count, which
// is expected to differ: shrinking it is the whole point.
func metadataEquivalent(a, b maxminddb.Metadata) (bool, string) {
	a.NodeCount = 0
	b.NodeCount = 0
	// A nil and an empty collection are the same thing in the file, so don't
	// let DeepEqual report "[] vs []".
	for _, m := range []*maxminddb.Metadata{&a, &b} {
		if len(m.Languages) == 0 {
			m.Languages = nil
		}
		if len(m.Description) == 0 {
			m.Description = nil
		}
	}
	if reflect.DeepEqual(a, b) {
		return true, ""
	}

	av := reflect.ValueOf(a)
	bv := reflect.ValueOf(b)
	t := av.Type()
	for i := 0; i < av.NumField(); i++ {
		field := t.Field(i)
		// Interface() panics on an unexported field. DeepEqual above still
		// covered it, so a difference there ends in the generic message.
		if field.Name == "NodeCount" || !field.IsExported() {
			continue
		}
		left := av.Field(i).Interface()
		right := bv.Field(i).Interface()
		if !reflect.DeepEqual(left, right) {
			return false, fmt.Sprintf("%s differs: %v vs %v", metadataFieldName(field), left, right)
		}
	}
	return false, "metadata differs"
}

func metadataFieldName(field reflect.StructField) string {
	if tag := field.Tag.Get("maxminddb"); tag != "" {
		name, _, _ := strings.Cut(tag, ",")
		if name != "" && name != "-" {
			return name
		}
	}
	var b strings.Builder
	for i, r := range field.Name {
		if i > 0 && r >= 'A' && r <= 'Z' {
			b.WriteByte('_')
		}
		b.WriteRune(r)
	}
	return strings.ToLower(b.String())
}

func lookupRecord(db *maxminddb.Reader, ip netip.Addr) (canonicalRecord, error) {
	result := db.Lookup(ip)
	if err := result.Err(); err != nil {
		return canonicalRecord{}, err
	}
	if !result.Found() {
		return canonicalRecord{}, nil
	}
	var r canonicalRecord
	err := result.Decode(&r)
	return r, err
}

// lookupDisplay renders a record for a mismatch report. Only called on the
// failure path, so the cost of materialising a map doesn't matter.
func lookupDisplay(db *maxminddb.Reader, ip netip.Addr) string {
	result := db.Lookup(ip)
	if err := result.Err(); err != nil {
		return fmt.Sprintf("<lookup error: %v>", err)
	}
	if !result.Found() {
		return "<not found>"
	}
	r := map[string]any{}
	if err := result.Decode(&r); err != nil {
		return fmt.Sprintf("<decode error: %v>", err)
	}
	return fmt.Sprint(r)
}

// checkLookupPair looks ip up in both readers and reports whether the
// baseline found a record. A lookup error or a record mismatch is returned as
// a *CheckError for the named check.
func checkLookupPair(check Check, a, b *maxminddb.Reader, ip netip.Addr) (bool, error) {
	ra, err := lookupRecord(a, ip)
	if err != nil {
		return false, &CheckError{Check: check, Detail: fmt.Sprintf("baseline lookup err ip=%s: %v", ip, err)}
	}
	rb, err := lookupRecord(b, ip)
	if err != nil {
		return false, &CheckError{Check: check, Detail: fmt.Sprintf("shrunk lookup err ip=%s: %v", ip, err)}
	}
	if !bytes.Equal(ra.data, rb.data) {
		return false, &CheckError{Check: check, Detail: fmt.Sprintf("ip=%s:\n  baseline:  %v\n  shrunk: %v",
			ip, lookupDisplay(a, ip), lookupDisplay(b, ip))}
	}
	return ra.found(), nil
}

// checkExpectedAt confirms targetDB returns expected at probe.ip, where
// expected was decoded from expectedDB for the prefix pfx.
func checkExpectedAt(check Check, expectedDB *maxminddb.Reader, targetLabel string, targetDB *maxminddb.Reader, expected canonicalRecord, pfx netip.Prefix, probe addrProbe) error {
	got, err := lookupRecord(targetDB, probe.ip)
	if err != nil {
		return &CheckError{Check: check, Detail: fmt.Sprintf("%s lookup err for %s %s ip=%s: %v",
			targetLabel, pfx, probe.label, probe.ip, err)}
	}
	if !bytes.Equal(expected.data, got.data) {
		return &CheckError{Check: check, Detail: fmt.Sprintf("at %s %s ip=%s:\n  expected: %v\n  %s: %v",
			pfx, probe.label, probe.ip, lookupDisplay(expectedDB, probe.ip), targetLabel, lookupDisplay(targetDB, probe.ip))}
	}
	return nil
}

// checkPairAt confirms both readers agree at probe.ip, an address adjacent
// to pfx.
func checkPairAt(check Check, a, b *maxminddb.Reader, label string, pfx netip.Prefix, probe addrProbe) error {
	ra, err := lookupRecord(a, probe.ip)
	if err != nil {
		return &CheckError{Check: check, Detail: fmt.Sprintf("baseline lookup err near %s %s ip=%s: %v", pfx, probe.label, probe.ip, err)}
	}
	rb, err := lookupRecord(b, probe.ip)
	if err != nil {
		return &CheckError{Check: check, Detail: fmt.Sprintf("shrunk lookup err near %s %s ip=%s: %v", pfx, probe.label, probe.ip, err)}
	}
	if !bytes.Equal(ra.data, rb.data) {
		return &CheckError{Check: check, Detail: fmt.Sprintf("near %s %s ip=%s (%s):\n  baseline:  %v\n  shrunk: %v",
			pfx, probe.label, probe.ip, label, lookupDisplay(a, probe.ip), lookupDisplay(b, probe.ip))}
	}
	return nil
}

// compareKeySets reports whether two schema maps have the same keys, naming
// the first missing key in sorted order so the message is the same on every
// run. Occurrence counts are deliberately ignored: a shrunk file has fewer
// records, so counts differ even when the schema is identical.
func compareKeySets(kind string, a, b map[string]int) (bool, string) {
	for _, k := range slices.Sorted(maps.Keys(a)) {
		if _, ok := b[k]; !ok {
			return false, fmt.Sprintf("%s %s present only in baseline", kind, k)
		}
	}
	for _, k := range slices.Sorted(maps.Keys(b)) {
		if _, ok := a[k]; !ok {
			return false, fmt.Sprintf("%s %s present only in shrunk", kind, k)
		}
	}
	return true, ""
}

func rootPrefix(db *maxminddb.Reader) netip.Prefix {
	if db.Metadata.IPVersion == 4 {
		return netip.MustParsePrefix("0.0.0.0/0")
	}
	return netip.MustParsePrefix("::/0")
}

func networkOptions(includeAliased, includeEmpty bool) []maxminddb.NetworksOption {
	opts := []maxminddb.NetworksOption{}
	if includeAliased {
		opts = append(opts, maxminddb.IncludeAliasedNetworks())
	}
	if includeEmpty {
		opts = append(opts, maxminddb.IncludeNetworksWithoutData())
	}
	return opts
}
