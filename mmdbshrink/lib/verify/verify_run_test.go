package verify

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maxmind/mmdbwriter"
	"github.com/maxmind/mmdbwriter/mmdbtype"
)

// dbSpec describes a small IPv4 database: one record per /8 for
// i = 1, 8, 15, ..., 218, produced by record(i), plus any extra networks. The
// metadata is pinned, so two files differ only where a test makes them differ.
type dbSpec struct {
	dbType string
	record func(i int) mmdbtype.Map
	extra  map[string]mmdbtype.Map
}

func plainRecord(i int) mmdbtype.Map {
	return mmdbtype.Map{
		"country": mmdbtype.String(fmt.Sprintf("C%d", i)),
		"asn":     mmdbtype.Uint32(uint32(64500 + i)),
	}
}

func writeDB(t *testing.T, name string, spec dbSpec) string {
	t.Helper()
	if spec.dbType == "" {
		spec.dbType = "verify-test"
	}
	if spec.record == nil {
		spec.record = plainRecord
	}
	w, err := mmdbwriter.New(mmdbwriter.Options{
		BuildEpoch:              1_700_000_000,
		DatabaseType:            spec.dbType,
		Description:             map[string]string{"en": "verify fixture"},
		Languages:               []string{"en"},
		IPVersion:               4,
		RecordSize:              24,
		IncludeReservedNetworks: true,
	})
	if err != nil {
		t.Fatal(err)
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
	for i := 1; i < 224; i += 7 {
		insert(fmt.Sprintf("%d.0.0.0/8", i), spec.record(i))
	}
	for cidr, rec := range spec.extra {
		insert(cidr, rec)
	}

	path := filepath.Join(t.TempDir(), name)
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.WriteTo(f); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

// quickOptions runs every check on a workload sized for a test.
func quickOptions() Options {
	opts := DefaultOptions()
	opts.RandomLookups = 2_000
	opts.BenchLookups = 1_000
	return opts
}

// assertCheck requires err to be the failure of the named check.
func assertCheck(t *testing.T, err error, want Check) *CheckError {
	t.Helper()
	var ce *CheckError
	if !errors.As(err, &ce) {
		t.Fatalf("err = %v, want a *CheckError for %q", err, want)
	}
	if ce.Check != want {
		t.Fatalf("failed check = %q, want %q: %s", ce.Check, want, ce.Detail)
	}
	return ce
}

func TestVerifyEquivalentFiles(t *testing.T) {
	a := writeDB(t, "a.mmdb", dbSpec{})
	b := writeDB(t, "b.mmdb", dbSpec{})

	opts := quickOptions()
	opts.Boundary, opts.BoundaryAdjacent = true, true
	sampled := 0
	opts.MemorySampler = func(path string) (MemorySample, error) {
		sampled++
		return MemorySample{MaxRSSBytes: 1}, nil
	}

	r, err := Verify(nil, a, b, opts)
	if err != nil {
		t.Fatal(err)
	}
	if r.Metadata.IPVersion != 4 || r.Metadata.DatabaseType != "verify-test" {
		t.Errorf("metadata stage = %+v", r.Metadata)
	}
	if r.FixedProbes.N == 0 {
		t.Error("fixed probes did not run")
	}
	if r.RandomIPv4 == nil || r.RandomIPv4.N != 2_000 {
		t.Errorf("random IPv4 stage = %+v, want 2000 lookups", r.RandomIPv4)
	}
	if r.RandomIPv6 != nil {
		t.Error("random IPv6 lookups ran against an IPv4-only database")
	}
	e := r.Enumeration
	if e == nil {
		t.Fatal("enumeration did not run")
	}
	// 32 /8 networks, none touching the ends of the address space: first,
	// mid and last for each, plus the address before and after each. The
	// probe counts cover both walks.
	if e.Forward != 32 || e.Reverse != 32 || e.Probes != 2*3*32 || e.AdjacentProbes != 2*2*32 {
		t.Errorf("enumeration = %+v, want 32/32 prefixes, 192 probes, 128 adjacent", *e)
	}
	if r.ExactPrefixes == nil || r.ExactPrefixes.Baseline != 32 || r.ExactPrefixes.Shrunk != 32 {
		t.Errorf("exact prefixes = %+v, want 32/32", r.ExactPrefixes)
	}
	if r.Schema == nil || r.Schema.Entries != 3 {
		t.Errorf("schema = %+v, want the 3 entries $=map, $.asn=uint32, $.country=string", r.Schema)
	}
	if r.Bench == nil || r.Bench.Lookups != 1_000 || r.Bench.Baseline.Size == 0 || r.Bench.Shrunk.Path != b {
		t.Errorf("bench = %+v", r.Bench)
	}
	if sampled != 2 || r.Memory == nil || r.Memory.Baseline.Sample.MaxRSSBytes != 1 {
		t.Errorf("memory sampler ran %d times, stage = %+v; want both files sampled", sampled, r.Memory)
	}
}

func TestVerifyDetectsChangedRecord(t *testing.T) {
	a := writeDB(t, "a.mmdb", dbSpec{})
	b := writeDB(t, "b.mmdb", dbSpec{record: func(i int) mmdbtype.Map {
		rec := plainRecord(i)
		if i == 50 {
			rec["country"] = mmdbtype.String("XX")
		}
		return rec
	}})

	// The seeded random stream lands in 50.0.0.0/8 several times.
	opts := quickOptions()
	assertCheck(t, must(Verify(nil, a, b, opts)), CheckRandomIPv4)

	// Without random lookups the enumeration catches it, at the prefix's
	// first address, and the report keeps what passed before.
	opts.RandomLookups = 0
	r, err := Verify(nil, a, b, opts)
	ce := assertCheck(t, err, CheckEnumerate)
	for _, want := range []string{"50.0.0.0/8", "C50", "XX"} {
		if !strings.Contains(ce.Detail, want) {
			t.Errorf("detail %q lacks %q", ce.Detail, want)
		}
	}
	if r.Metadata.IPVersion != 4 || r.FixedProbes.N == 0 || r.Enumeration != nil || r.Bench != nil {
		t.Errorf("partial report = %+v, want metadata and fixed probes only", r)
	}
}

func TestVerifyDetectsExtraPrefixInEitherOrder(t *testing.T) {
	a := writeDB(t, "a.mmdb", dbSpec{})
	b := writeDB(t, "b.mmdb", dbSpec{extra: map[string]mmdbtype.Map{
		"200.1.0.0/16": {"country": mmdbtype.String("EX")},
	}})

	// A /16 is too small for a few thousand random lookups to hit, so this
	// is the enumeration's job: walking b finds a prefix a knows nothing
	// about, and walking a first passes, since b agrees everywhere a has data.
	opts := quickOptions()
	opts.RandomLookups = 0
	assertCheck(t, must(Verify(nil, a, b, opts)), CheckReverseEnum)
	assertCheck(t, must(Verify(nil, b, a, opts)), CheckEnumerate)
}

func TestVerifyDetectsMetadataMismatchBeforeAnyLookup(t *testing.T) {
	a := writeDB(t, "a.mmdb", dbSpec{})
	b := writeDB(t, "b.mmdb", dbSpec{dbType: "verify-test-2"})

	r, err := Verify(nil, a, b, quickOptions())
	ce := assertCheck(t, err, CheckMetadata)
	if !strings.Contains(ce.Detail, "database_type differs") {
		t.Errorf("detail = %q", ce.Detail)
	}
	if r.Metadata != (MetadataStage{}) || r.FixedProbes.N != 0 {
		t.Errorf("stages ran after a metadata failure: %+v", r)
	}
}

func TestVerifyEnumLimit(t *testing.T) {
	a := writeDB(t, "a.mmdb", dbSpec{})
	b := writeDB(t, "b.mmdb", dbSpec{})

	opts := quickOptions()
	opts.RandomLookups = 0
	opts.EnumLimit = 5
	opts.Bench = false
	r, err := Verify(nil, a, b, opts)
	if err != nil {
		t.Fatal(err)
	}
	if r.Enumeration.Forward != 5 || r.Enumeration.Reverse != 5 || r.Enumeration.Probes != 10 {
		t.Errorf("enumeration = %+v, want 5 prefixes each way", *r.Enumeration)
	}
	if r.ExactPrefixes.Baseline != 5 || r.ExactPrefixes.Shrunk != 5 {
		t.Errorf("exact prefixes = %+v, want 5/5", *r.ExactPrefixes)
	}
	if r.Bench != nil || r.Memory != nil {
		t.Error("bench ran with Bench off")
	}
}

func TestVerifyRejectsBadOptionsAndMissingFiles(t *testing.T) {
	a := writeDB(t, "a.mmdb", dbSpec{})
	var ce *CheckError

	opts := quickOptions()
	opts.EnumLimit = -1
	if _, err := Verify(nil, a, a, opts); err == nil || errors.As(err, &ce) {
		t.Errorf("negative EnumLimit: err = %v, want a plain validation error", err)
	}
	opts = quickOptions()
	opts.BenchLookups = 0
	if _, err := Verify(nil, a, a, opts); err == nil || errors.As(err, &ce) {
		t.Errorf("Bench with zero lookups: err = %v, want a plain validation error", err)
	}

	missing := filepath.Join(t.TempDir(), "missing.mmdb")
	if _, err := Verify(nil, a, missing, quickOptions()); err == nil || errors.As(err, &ce) {
		t.Errorf("missing file: err = %v, want a plain I/O error, not a check failure", err)
	}
}

// must discards a Report so a call can be passed straight to assertCheck.
func must(_ Report, err error) error { return err }
