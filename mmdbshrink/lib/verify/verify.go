// Package verify checks that two MMDB files answer every lookup identically.
//
// It is not a format validator: two files corrupt in the same way would
// pass. It answers a different question — after shrinking, does the output
// return exactly what the input returned, at every address?
package verify

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/netip"
	"os"
	"time"

	"github.com/oschwald/maxminddb-golang/v2"
)

// Options configures Verify.
type Options struct {
	// RandomLookups is the number of random addresses checked per address
	// family. Zero skips the random-lookup check.
	RandomLookups uint
	// IPv6 includes random IPv6 lookups. Ignored for IPv4-only databases.
	IPv6 bool
	// Enumerate walks every prefix in each file and confirms the other file
	// agrees. The strongest check, and the slowest: minutes on multi-GB files.
	Enumerate bool
	// EnumLimit stops each enumeration pass after this many prefixes. Zero
	// means no limit.
	EnumLimit int
	// Boundary also probes the mid and last address of every enumerated
	// prefix, not just the first.
	Boundary bool
	// BoundaryAdjacent also probes the addresses immediately before and
	// after every enumerated prefix.
	BoundaryAdjacent bool
	// ExactPrefixes requires both files to expose the same prefix set. Turn
	// off to accept a different layout that is nonetheless lookup-equivalent.
	ExactPrefixes bool
	// Schema compares the key paths and value types observed during
	// enumeration. A complete walk in both directions already proves the two
	// files hold the same records, so this only adds information when
	// EnumLimit cuts the walk short.
	Schema bool
	// AliasedNetworks includes IPv4 alias networks when enumerating IPv6
	// databases.
	AliasedNetworks bool
	// EmptyNetworks includes networks without data when enumerating.
	EmptyNetworks bool
	// Bench times BenchLookups warm-cache lookups against each file.
	Bench        bool
	BenchLookups int
	// MemorySampler, when set and Bench is on, measures each file's reader
	// memory.
	// The measure must run in a fresh process as resident pages
	// outlive the process that faulted them in, so an in-process sample would
	// inherit whatever this process has already touched.
	// Verify calls it for both files before opening either.
	// If nil we skip the memory report.
	MemorySampler func(path string) (MemorySample, error)
	// Seed drives the random-lookup and bench address streams.
	Seed uint64
}

// DefaultOptions returns the checks the tool runs by default.
func DefaultOptions() Options {
	return Options{
		RandomLookups: 1_000_000,
		IPv6:          true,
		Enumerate:     true,
		ExactPrefixes: true,
		Schema:        true,
		Bench:         true,
		BenchLookups:  1_000_000,
		Seed:          1,
	}
}

// MemorySample is what a MemorySampler returns. The JSON tags match the
// single-file `bench --json` output so a sampler can decode it directly.
type MemorySample struct {
	MaxRSSBytes     uint64  `json:"max_rss_bytes"`
	MmapRSSBytes    uint64  `json:"mmap_rss_bytes"`
	MmapCachedBytes uint64  `json:"mmap_cached_bytes"`
	MinorFaults     int64   `json:"minor_faults"`
	MajorFaults     int64   `json:"major_faults"`
	NsPerOp         float64 `json:"ns_per_op"`
}

// Check names one of the verification stages.
type Check string

// The checks, in the order they run.
const (
	CheckMetadata      Check = "metadata"
	CheckFixedProbes   Check = "fixed probes"
	CheckRandomIPv4    Check = "random IPv4"
	CheckRandomIPv6    Check = "random IPv6"
	CheckEnumerate     Check = "enumerate"
	CheckReverseEnum   Check = "reverse-enumerate"
	CheckExactPrefixes Check = "exact-prefixes"
	CheckSchema        Check = "schema"
)

// CheckError is the first failed check. Verify stops at the first failure
// and returns whatever passed before it in the Report.
type CheckError struct {
	Check  Check
	Detail string
}

func (e *CheckError) Error() string {
	return "FAIL " + string(e.Check) + ": " + e.Detail
}

// MetadataStage describes the metadata both files share, plus the one field
// allowed to differ.
type MetadataStage struct {
	IPVersion         uint
	RecordSize        uint
	BinaryFormatMajor uint
	BinaryFormatMinor uint
	DatabaseType      string
	BuildEpoch        uint
	BaselineNodes     uint
	ShrunkNodes       uint
}

// LookupStage summarises a batch of paired lookups that all matched.
type LookupStage struct {
	N       uint
	Hits    uint
	Elapsed time.Duration
}

// EnumerationStage summarises the prefix walk in both directions.
type EnumerationStage struct {
	Forward        int
	Reverse        int
	Probes         int
	AdjacentProbes int
	Elapsed        time.Duration
}

// ExactPrefixStage records the prefix counts confirmed identical.
type ExactPrefixStage struct {
	Baseline int
	Shrunk   int
}

// SchemaStage records the observed schema confirmed identical.
type SchemaStage struct {
	Forward int
	Reverse int
	Entries int
}

// BenchSide is one file's lookup-speed result.
type BenchSide struct {
	Path    string
	Size    uint64
	NsPerOp int64
	QPS     float64
}

// BenchStage holds both files' lookup-speed results.
type BenchStage struct {
	Lookups  int
	Baseline BenchSide
	Shrunk   BenchSide
}

// MemorySide is one file's memory sample, or the reason it's missing.
type MemorySide struct {
	Sample MemorySample
	Err    error
}

// MemoryStage holds both files' memory samples.
type MemoryStage struct {
	Baseline MemorySide
	Shrunk   MemorySide
}

// Report is what Verify returns. A nil stage pointer means that check was
// skipped by the Options.
type Report struct {
	Seed          uint64
	Metadata      MetadataStage
	FixedProbes   LookupStage
	RandomIPv4    *LookupStage
	RandomIPv6    *LookupStage
	Enumeration   *EnumerationStage
	ExactPrefixes *ExactPrefixStage
	Schema        *SchemaStage
	Bench         *BenchStage
	Memory        *MemoryStage
}

// Verify runs the layered equivalence checks between the mmdb files at
// basePath and shrunkPath, in increasing order of cost: metadata, fixed edge
// addresses, random lookups, full enumeration, then the optional benchmark.
// It stops at the first mismatch, returned as a *CheckError alongside the
// partial Report. The check is symmetric; argument order does not matter.
//
// A nil logger discards progress.
func Verify(logger *slog.Logger, basePath, shrunkPath string, opts Options) (Report, error) {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	if opts.EnumLimit < 0 {
		return Report{}, errors.New("EnumLimit must not be negative")
	}
	if opts.Bench && opts.BenchLookups <= 0 {
		return Report{}, errors.New("BenchLookups must be positive when Bench is set")
	}

	report := Report{Seed: opts.Seed}

	// Memory samples go first, before this process opens either file:
	// the sampler's child processes need a page cache we haven't warmed.
	if opts.Bench && opts.MemorySampler != nil {
		logger.Debug("memory: sampling both files in child processes")
		var m MemoryStage
		m.Baseline.Sample, m.Baseline.Err = opts.MemorySampler(basePath)
		m.Shrunk.Sample, m.Shrunk.Err = opts.MemorySampler(shrunkPath)
		report.Memory = &m
	}

	a, err := maxminddb.Open(basePath)
	if err != nil {
		return report, err
	}
	defer a.Close()
	b, err := maxminddb.Open(shrunkPath)
	if err != nil {
		return report, err
	}
	defer b.Close()

	iterOpts := networkOptions(opts.AliasedNetworks, opts.EmptyNetworks)

	// 1. Metadata.
	if ok, msg := metadataEquivalent(a.Metadata, b.Metadata); !ok {
		return report, &CheckError{Check: CheckMetadata, Detail: msg}
	}
	report.Metadata = MetadataStage{
		IPVersion:         a.Metadata.IPVersion,
		RecordSize:        a.Metadata.RecordSize,
		BinaryFormatMajor: a.Metadata.BinaryFormatMajorVersion,
		BinaryFormatMinor: a.Metadata.BinaryFormatMinorVersion,
		DatabaseType:      a.Metadata.DatabaseType,
		BuildEpoch:        a.Metadata.BuildEpoch,
		BaselineNodes:     a.Metadata.NodeCount,
		ShrunkNodes:       b.Metadata.NodeCount,
	}
	logger.Debug(fmt.Sprintf("metadata: ok, node_count %d -> %d", a.Metadata.NodeCount, b.Metadata.NodeCount))

	// 2. Deterministic edge addresses.
	fixed := fixedProbes(a.Metadata.IPVersion)
	t0 := time.Now()
	for _, probe := range fixed {
		found, err := checkLookupPair(CheckFixedProbes, a, b, probe.ip)
		if err != nil {
			return report, err
		}
		if found {
			report.FixedProbes.Hits++
		}
	}
	report.FixedProbes.N = uint(len(fixed))
	report.FixedProbes.Elapsed = time.Since(t0)
	logger.Debug(fmt.Sprintf("fixed probes: %d ok in %s", len(fixed), report.FixedProbes.Elapsed.Round(time.Millisecond)))

	// 3. Random lookups. Both families draw from one generator so the
	// stream is a function of the seed alone.
	if opts.RandomLookups > 0 {
		r := rand.New(rand.NewPCG(opts.Seed, opts.Seed^0xdeadbeef))
		mkV4 := func() netip.Addr {
			v := r.Uint32()
			return netip.AddrFrom4([4]byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)})
		}
		mkV6 := func() netip.Addr {
			var b [16]byte
			v1, v2 := r.Uint64(), r.Uint64()
			for i := range 8 {
				b[i] = byte(v1 >> (i * 8))
				b[8+i] = byte(v2 >> (i * 8))
			}
			return netip.AddrFrom16(b)
		}

		stage, err := randomLookups(CheckRandomIPv4, a, b, mkV4, opts.RandomLookups)
		if err != nil {
			return report, err
		}
		report.RandomIPv4 = &stage
		logger.Debug(fmt.Sprintf("random IPv4: %d ok in %s", stage.N, stage.Elapsed.Round(time.Millisecond)))

		if opts.IPv6 && a.Metadata.IPVersion == 6 {
			stage, err := randomLookups(CheckRandomIPv6, a, b, mkV6, opts.RandomLookups)
			if err != nil {
				return report, err
			}
			report.RandomIPv6 = &stage
			logger.Debug(fmt.Sprintf("random IPv6: %d ok in %s", stage.N, stage.Elapsed.Round(time.Millisecond)))
		}
	}

	// 4. Full prefix enumeration, both directions. Forward catches records
	// the shrunk file changed or lost; reverse catches prefixes it
	// invented.
	if opts.Enumerate {
		t0 := time.Now()
		var enum EnumerationStage
		aSchema := map[string]int{}
		bSchema := map[string]int{}

		enum.Forward, err = enumerate(CheckEnumerate, a, b, "shrunk", aSchema, iterOpts, opts, &enum)
		if err != nil {
			return report, err
		}
		logger.Debug(fmt.Sprintf("enumerate: %d prefixes forward", enum.Forward))
		enum.Reverse, err = enumerate(CheckReverseEnum, b, a, "baseline", bSchema, iterOpts, opts, &enum)
		if err != nil {
			return report, err
		}
		enum.Elapsed = time.Since(t0)
		report.Enumeration = &enum
		logger.Debug(fmt.Sprintf("enumerate: %d prefixes reverse, %d probes, done in %s", enum.Reverse, enum.Probes, enum.Elapsed.Round(time.Millisecond)))

		if opts.ExactPrefixes {
			exactA, exactB, ok, msg, err := compareExactPrefixStreams(a, b, iterOpts, opts.EnumLimit)
			if err != nil {
				return report, &CheckError{Check: CheckExactPrefixes, Detail: err.Error()}
			}
			if !ok {
				return report, &CheckError{Check: CheckExactPrefixes, Detail: msg}
			}
			report.ExactPrefixes = &ExactPrefixStage{Baseline: exactA, Shrunk: exactB}
			logger.Debug("exact prefixes: ok")
		}
		if opts.Schema {
			if ok, msg := compareKeySets("schema entry", aSchema, bSchema); !ok {
				return report, &CheckError{Check: CheckSchema, Detail: msg}
			}
			report.Schema = &SchemaStage{Forward: enum.Forward, Reverse: enum.Reverse, Entries: len(aSchema)}
			logger.Debug(fmt.Sprintf("schema: %d entries ok", len(aSchema)))
		}
	}

	// 5. Lookup-speed benchmark.
	if opts.Bench {
		report.Bench = &BenchStage{
			Lookups:  opts.BenchLookups,
			Baseline: benchOne(basePath, a, opts.Seed, opts.BenchLookups),
			Shrunk:   benchOne(shrunkPath, b, opts.Seed, opts.BenchLookups),
		}
		logger.Debug(fmt.Sprintf("bench: baseline %dns/op, shrunk %dns/op", report.Bench.Baseline.NsPerOp, report.Bench.Shrunk.NsPerOp))
	}

	return report, nil
}

// randomLookups compares n addresses from mk in both readers, counting hits
// and stopping at the first mismatch.
func randomLookups(check Check, a, b *maxminddb.Reader, mk func() netip.Addr, n uint) (LookupStage, error) {
	t0 := time.Now()
	var hits uint
	for range n {
		ip := mk()
		ra, err := lookupRecord(a, ip)
		if err != nil {
			return LookupStage{}, &CheckError{Check: check, Detail: fmt.Sprintf("baseline lookup err ip=%s: %v", ip, err)}
		}
		rb, err := lookupRecord(b, ip)
		if err != nil {
			return LookupStage{}, &CheckError{Check: check, Detail: fmt.Sprintf("shrunk lookup err ip=%s: %v", ip, err)}
		}
		if !bytes.Equal(ra.data, rb.data) {
			return LookupStage{}, &CheckError{Check: check, Detail: fmt.Sprintf("ip=%s:\n  baseline:  %v\n  shrunk: %v",
				ip, lookupDisplay(a, ip), lookupDisplay(b, ip))}
		}
		if ra.found() {
			hits++
		}
	}
	return LookupStage{N: n, Hits: hits, Elapsed: time.Since(t0)}, nil
}

// enumerate walks every prefix of from and confirms to agrees at each probe
// address. Probe counts accumulate into enum; the prefix count is returned.
func enumerate(
	check Check,
	from, to *maxminddb.Reader,
	toLabel string,
	schema map[string]int,
	iterOpts []maxminddb.NetworksOption,
	opts Options,
	enum *EnumerationStage,
) (int, error) {
	count := 0
	for result := range from.NetworksWithin(rootPrefix(from), iterOpts...) {
		if err := result.Err(); err != nil {
			return count, &CheckError{Check: check, Detail: err.Error()}
		}
		pfx := result.Prefix().Masked()
		rec := canonicalRecord{}
		if opts.Schema {
			rec.schema = schema
		}
		if result.Found() {
			if err := result.Decode(&rec); err != nil {
				return count, &CheckError{Check: check, Detail: "decode err: " + err.Error()}
			}
		}
		for _, probe := range prefixProbes(pfx, opts.Boundary) {
			if err := checkExpectedAt(check, from, toLabel, to, rec, pfx, probe); err != nil {
				return count, err
			}
			enum.Probes++
		}
		if opts.BoundaryAdjacent {
			for _, probe := range adjacentProbes(pfx) {
				if err := checkPairAt(check, from, to, toLabel+" enumeration", pfx, probe); err != nil {
					return count, err
				}
				enum.AdjacentProbes++
			}
		}
		count++
		if opts.EnumLimit > 0 && count >= opts.EnumLimit {
			break
		}
	}
	return count, nil
}

// benchOne times n warm-cache lookups against db. The address stream is
// full-range IPv4 seeded independently of the random-lookup check.
func benchOne(path string, db *maxminddb.Reader, seed uint64, n int) BenchSide {
	r := rand.New(rand.NewPCG(seed^0xfeedface, seed^0xcafebabe))
	mk := func() netip.Addr {
		v := r.Uint32()
		return netip.AddrFrom4([4]byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)})
	}
	var sink canonicalRecord
	for range 100_000 {
		sink.data = sink.data[:0]
		_ = db.Lookup(mk()).Decode(&sink)
	}
	t0 := time.Now()
	for range n {
		sink.data = sink.data[:0]
		_ = db.Lookup(mk()).Decode(&sink)
	}
	dt := time.Since(t0)

	side := BenchSide{
		Path:    path,
		NsPerOp: (dt / time.Duration(n)).Nanoseconds(),
		QPS:     float64(n) / dt.Seconds(),
	}
	if fi, err := os.Stat(path); err == nil {
		side.Size = uint64(fi.Size())
	}
	return side
}
