package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/ipinfo/mmdbctl/mmdbshrink/lib/bench"
	"github.com/ipinfo/mmdbctl/mmdbshrink/lib/format"
	"github.com/ipinfo/mmdbctl/mmdbshrink/lib/verify"
)

// memorySampleCmd is the hidden command a full report runs in a child
// process to sample one file's reader memory. It is internal: it's left out
// of help and completions, and its output is meant for memorySampler only.
const memorySampleCmd = "__bench-memory"

// memorySampleLookups is how many lookups each memory sample runs.
const memorySampleLookups = 200_000

// reportOptions is the validation a report runs after shrinking: the checks
// of verify plus its lookup-speed benchmark, without the full prefix
// enumeration, which takes minutes on large files.
func reportOptions() verify.Options {
	opts := verify.DefaultOptions()
	opts.Enumerate = false
	opts.ExactPrefixes = false
	opts.Schema = false
	return opts
}

// report validates the shrunk file at outputPath against inputPath and
// prints the results. With full, it also measures both files' reader memory
// first, each in a fresh child process.
//
// prog is the prefix shown in usage strings; the memory sampler reuses it to
// run the hidden sample command at the right nesting depth.
func report(prog, inputPath, outputPath string, full bool) error {
	var memory []memorySide
	memoryErr := bench.MemorySupported()
	if full && memoryErr == nil {
		// Sampled before this process opens either file, so the children
		// don't inherit pages this process touched while validating.
		sample := memorySampler(prog, memorySampleLookups, reportOptions().Seed)
		for _, side := range []struct{ label, path string }{
			{"input:", inputPath},
			{"output:", outputPath},
		} {
			s, err := sample(side.path)
			memory = append(memory, memorySide{label: side.label, sample: s, err: err})
		}
	}

	opts := reportOptions()
	r, err := verify.Verify(nil, inputPath, outputPath, opts)
	var checkErr *verify.CheckError
	if err != nil && !errors.As(err, &checkErr) {
		return fmt.Errorf("validate %s: %w", outputPath, err)
	}

	fmt.Println()
	fmt.Printf("validation (seed %d):\n", r.Seed)
	printValidation(r, opts)
	if err != nil {
		// The file is written but answers some lookup differently from
		// the input, so it must not be used.
		return fmt.Errorf("%s failed validation, do not use it: %w", outputPath, err)
	}

	printBench(r)
	switch {
	case !full:
	case memoryErr != nil:
		// Memory is an extra: the shrink and its validation stand on their
		// own, so a platform that can't measure it only loses this section.
		fmt.Println()
		fmt.Printf("memory: SKIPPED (%v)\n", memoryErr)
	default:
		printMemory(memory)
	}
	return nil
}

// printValidation prints whichever checks completed, in the order they ran.
// On failure the report is partial and the caller prints the error after.
func printValidation(r verify.Report, opts verify.Options) {
	m := r.Metadata
	if m.IPVersion == 0 {
		// metadata check failed; nothing else ran
		return
	}
	nodePct := 100 * (float64(m.ShrunkNodes)/float64(m.BaselineNodes) - 1)
	fmt.Printf("  [OK] %-14s  ip_version=%d  record_size=%d  binary_format=%d.%d\n",
		"metadata",
		m.IPVersion,
		m.RecordSize,
		m.BinaryFormatMajor,
		m.BinaryFormatMinor,
	)
	fmt.Printf("%23sdatabase_type: %s  build_epoch=%d\n", "", m.DatabaseType, m.BuildEpoch)
	fmt.Printf("%23snode_count: %s -> %s (%+.2f%%)\n", "",
		format.Int(uint64(m.BaselineNodes)),
		format.Int(uint64(m.ShrunkNodes)),
		nodePct,
	)

	if r.FixedProbes.N == 0 {
		return
	}
	fmt.Printf("  [OK] %-14s  n=%s  hits=%s  (%s)\n", "fixed probes",
		format.Int(uint64(r.FixedProbes.N)),
		format.Int(uint64(r.FixedProbes.Hits)),
		r.FixedProbes.Elapsed.Round(time.Millisecond),
	)

	printLookup := func(label string, s *verify.LookupStage) {
		fmt.Printf("  [OK] %-14s  n=%s  hits=%s (%.2f%%)  mismatches=0  (%s)\n", label,
			format.Int(uint64(s.N)),
			format.Int(uint64(s.Hits)),
			100*float64(s.Hits)/float64(s.N),
			s.Elapsed.Round(time.Millisecond),
		)
	}
	if r.RandomIPv4 == nil {
		return
	}
	printLookup("random IPv4", r.RandomIPv4)
	if r.RandomIPv6 != nil {
		printLookup("random IPv6", r.RandomIPv6)
	} else if opts.IPv6 && m.IPVersion != 6 {
		fmt.Printf("  %-19s  SKIPPED (IPv4-only database)\n", "random IPv6")
	}
}

// printBench prints the lookup-speed timing verify ran on both files. It is a
// single run per file, so it shows rough parity, not a precise difference.
func printBench(r verify.Report) {
	if r.Bench == nil {
		return
	}
	fmt.Println()
	fmt.Printf("bench (%s lookups each):\n", format.Int(uint64(r.Bench.Lookups)))
	for _, side := range []struct {
		label string
		b     verify.BenchSide
	}{
		{"input:", r.Bench.Baseline},
		{"output:", r.Bench.Shrunk},
	} {
		fmt.Printf("  %-8s  size=%-10s  per_op=%4dns  qps=%s\n",
			side.label,
			format.DecimalBytes(side.b.Size),
			side.b.NsPerOp,
			format.Int(uint64(side.b.QPS)),
		)
	}
}

// memorySide is one file's memory sample, or the reason it's missing.
type memorySide struct {
	label  string
	sample verify.MemorySample
	err    error
}

func printMemory(sides []memorySide) {
	fmt.Println()
	fmt.Printf("memory (%s lookups each, separate processes):\n", format.Int(memorySampleLookups))
	for _, s := range sides {
		if s.err != nil {
			fmt.Printf("  %-8s  SKIPPED (%v)\n", s.label, s.err)
			continue
		}
		mmapBytes, mmapLabel := s.sample.MmapRSSBytes, "mmap_rss"
		if mmapBytes == 0 {
			mmapBytes, mmapLabel = s.sample.MmapCachedBytes, "mmap_cached"
		}
		// Memory in IEC units: the kernel accounts in pages.
		fmt.Printf("  %-8s  %s=%-10s  proc_rss=%-10s  per_op=%4dns  minflt=%s  majflt=%s\n",
			s.label,
			mmapLabel,
			format.Bytes(mmapBytes),
			format.Bytes(s.sample.MaxRSSBytes),
			int64(s.sample.NsPerOp),
			format.Int(uint64(s.sample.MinorFaults)),
			format.Int(uint64(s.sample.MajorFaults)),
		)
	}
}

// memorySampler returns a sampler function that runs the hidden memory
// sample command in a child process. It reuses the binary that called it,
// either "mmdbshrink" or "mmdbctl shrink": whatever follows the binary name
// in prog is the command path the child needs as well.
func memorySampler(prog string, lookups uint, seed uint64) func(string) (verify.MemorySample, error) {
	var prefix []string
	if fields := strings.Fields(prog); len(fields) > 1 {
		prefix = fields[1:]
	}
	return func(path string) (verify.MemorySample, error) {
		var sample verify.MemorySample
		exe, err := os.Executable()
		if err != nil {
			return sample, err
		}
		args := append(append([]string{}, prefix...),
			memorySampleCmd,
			"--lookups", strconv.FormatUint(uint64(lookups), 10),
			"--seed", strconv.FormatUint(seed, 10),
			path,
		)
		cmd := exec.Command(exe, args...)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		out, err := cmd.Output()
		if err != nil {
			if msg := strings.TrimSpace(stderr.String()); msg != "" {
				return sample, fmt.Errorf("%w: %s", err, msg)
			}
			return sample, err
		}
		if err := json.Unmarshal(out, &sample); err != nil {
			return sample, fmt.Errorf("decode memory sample JSON: %w", err)
		}
		return sample, nil
	}
}

// cmdMemorySample is the hidden command behind memorySampler: it measures
// one file's reader memory and prints the sample as JSON.
func cmdMemorySample(args []string) error {
	opts := bench.DefaultMemoryOptions()

	fs := newFlagSet(memorySampleCmd)
	fs.UintVar(&opts.Lookups, "lookups", opts.Lookups, "lookups in the measured pass")
	fs.Uint64Var(&opts.Seed, "seed", opts.Seed, "seed for the random IP stream")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) != 1 {
		return errors.New(memorySampleCmd + " takes one mmdb file")
	}
	opts.Warmup = opts.Lookups / 10

	s, err := bench.Memory(nil, rest[0], opts)
	if err != nil {
		return err
	}
	if err := json.NewEncoder(os.Stdout).Encode(s); err != nil {
		return fmt.Errorf("encode json: %w", err)
	}
	return nil
}
