package bench

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/edsrzf/mmap-go"
	"github.com/oschwald/maxminddb-golang/v2"
)

// MemoryOptions configures Memory.
type MemoryOptions struct {
	// Lookups is the number of lookups in the measured pass. Must be positive.
	Lookups uint
	// Warmup is the number of lookups run before the measured pass. May be
	// zero.
	Warmup uint
	// Seed drives the random IP stream.
	Seed uint64
	// Cold evicts this process's resident pages of the mapping right before
	// the measured pass, so every touch faults back in and the fault counters
	// describe a cold start. The pages usually stay in the kernel page cache,
	// so expect minor faults, not major ones. Linux only: macOS offers no
	// way to evict file-backed pages.
	Cold bool
}

// MemorySupported reports whether Memory can measure on this platform: nil on
// Linux and macOS, an error saying why not anywhere else.
func MemorySupported() error {
	return checkMemorySupport(false)
}

// DefaultMemoryOptions returns the workload the tool ships with.
func DefaultMemoryOptions() MemoryOptions {
	return MemoryOptions{
		Lookups: 200_000,
		Warmup:  20_000,
		Seed:    1,
	}
}

// MemorySample is what Memory reports. Field tags define the JSON output.
type MemorySample struct {
	Path        string  `json:"path"`
	FileBytes   int64   `json:"file_bytes"`
	N           uint    `json:"n_lookups"`
	WarmupN     uint    `json:"warmup_lookups"`
	Seed        uint64  `json:"seed"`
	Cold        bool    `json:"cold"`
	WallSeconds float64 `json:"wall_seconds"`
	NsPerOp     float64 `json:"ns_per_op"`
	// MaxRSSBytes is the whole process's peak resident set size.
	MaxRSSBytes uint64 `json:"max_rss_bytes"`
	// MmapRSSBytes and MmapPSSBytes describe this mapping alone, from
	// /proc/self/smaps. Zero where smaps is unavailable.
	MmapRSSBytes uint64 `json:"mmap_rss_bytes,omitempty"`
	MmapPSSBytes uint64 `json:"mmap_pss_bytes,omitempty"`
	// MmapCachedBytes and MmapCachedPages count the file's pages in the
	// kernel page cache, from mincore. That is not what this process holds:
	// it includes pages cached by anyone, and Linux reports every page as
	// cached for a file the caller can't open for writing.
	MmapCachedBytes uint64 `json:"mmap_cached_bytes,omitempty"`
	MmapCachedPages uint64 `json:"mmap_cached_pages,omitempty"`
	PageSize        int    `json:"page_size"`
	// MinorFaults and MajorFaults count faults during the measured pass only.
	// Minor faults needed no disk I/O: page-cache hits on the mapping, but
	// also zero-filled heap pages. Major faults needed a disk read.
	MinorFaults int64 `json:"minor_faults"`
	MajorFaults int64 `json:"major_faults"`
	Env
}

// resourceUsage is the process-level accounting Memory needs from the OS.
// The platform files provide readResourceUsage.
type resourceUsage struct {
	maxRSSBytes uint64
	minorFaults int64
	majorFaults int64
}

// Memory runs a lookup workload against the mmdb file at path and reports how
// much memory the reader ended up holding, plus page-fault counters.
//
// The numbers are only meaningful when this is the first thing the process
// does with the file. Resident pages persist in the kernel page cache after a
// process exits, so measuring several files, or the same file twice, from one
// process makes later measurements inherit the earlier ones' warm cache. The
// CLI runs one measurement per process for exactly this reason.
//
// Supported on Linux and macOS. Two things additionally need Linux:
// per-mapping RSS/PSS, which is zero elsewhere, and Cold, which is an error
// elsewhere. On any other OS Memory returns an error. Unsupported requests
// fail before any lookups run.
//
// A nil logger discards progress.
func Memory(logger *slog.Logger, path string, opts MemoryOptions) (MemorySample, error) {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	if opts.Lookups == 0 {
		return MemorySample{}, errors.New("Lookups must be positive")
	}
	if err := checkMemorySupport(opts.Cold); err != nil {
		return MemorySample{}, err
	}

	rdr, mapped, cleanup, err := openMappedReader(path)
	if err != nil {
		return MemorySample{}, err
	}
	defer func() {
		if err := cleanup(); err != nil {
			logger.Warn(fmt.Sprintf("cleanup %s: %v", path, err))
		}
	}()

	mkIP := makeIPv4(opts.Seed)

	logger.Debug(fmt.Sprintf("warmup: %d lookups", opts.Warmup))
	for range opts.Warmup {
		var rec any
		_ = rdr.Lookup(mkIP()).Decode(&rec)
	}

	// Settle the Go side so the measured pass starts from the same heap state
	// whether or not the mapping is evicted next; otherwise a collection left
	// pending by the warmup would land in one mode's fault count and not the
	// other's. No FreeOSMemory: returning the heap to the OS would make the
	// measured pass re-fault it and inflate the minor faults.
	runtime.GC()

	if opts.Cold {
		// Drop this process's pages of the mapping, so every touch in the
		// measured pass faults back in.
		if err := evictMapping(mapped); err != nil {
			return MemorySample{}, fmt.Errorf("cold mode: %w", err)
		}
		logger.Debug("cold: mapping evicted")
	}

	// Snapshot the counters so the faults reported are the measured pass's
	// alone, not everything since process start.
	before, err := readResourceUsage()
	if err != nil {
		return MemorySample{}, err
	}

	logger.Debug(fmt.Sprintf("measure: %d lookups", opts.Lookups))
	t0 := time.Now()
	for range opts.Lookups {
		var rec any
		_ = rdr.Lookup(mkIP()).Decode(&rec)
	}
	wall := time.Since(t0).Seconds()

	after, err := readResourceUsage()
	if err != nil {
		return MemorySample{}, err
	}

	pages, bytes, pageSize, err := residentPages(mapped)
	if err != nil {
		logger.Warn(fmt.Sprintf("mincore: %v", err))
	}
	rss, pss, found, err := mmapSmaps(path)
	switch {
	case err != nil:
		logger.Warn(fmt.Sprintf("smaps: %v", err))
	case !found && runtime.GOOS == "linux":
		logger.Warn(fmt.Sprintf("smaps: no mapping of %s found; per-mapping RSS/PSS omitted", path))
	}

	return MemorySample{
		Path:            path,
		FileBytes:       int64(len(mapped)),
		N:               opts.Lookups,
		WarmupN:         opts.Warmup,
		Seed:            opts.Seed,
		Cold:            opts.Cold,
		WallSeconds:     wall,
		NsPerOp:         wall * 1e9 / float64(opts.Lookups),
		MaxRSSBytes:     after.maxRSSBytes,
		MmapRSSBytes:    rss,
		MmapPSSBytes:    pss,
		MmapCachedBytes: bytes,
		MmapCachedPages: pages,
		PageSize:        pageSize,
		MinorFaults:     after.minorFaults - before.minorFaults,
		MajorFaults:     after.majorFaults - before.majorFaults,
		Env:             currentEnv(),
	}, nil
}

// openMappedReader maps the file and opens a reader over that mapping, so the
// mapping the OS accounts for is the one the reader actually touches.
func openMappedReader(path string) (*maxminddb.Reader, mmap.MMap, func() error, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, nil, err
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		return nil, nil, nil, err
	}
	if st.Size() == 0 {
		return nil, nil, nil, errors.New("file is empty")
	}

	mapped, err := mmap.Map(f, mmap.RDONLY, 0)
	if err != nil {
		return nil, nil, nil, err
	}

	rdr, err := maxminddb.OpenBytes(mapped)
	if err != nil {
		_ = mapped.Unmap()
		return nil, nil, nil, err
	}

	cleanup := func() error {
		closeErr := rdr.Close()
		if err := mapped.Unmap(); err != nil && closeErr == nil {
			closeErr = err
		}
		return closeErr
	}
	return rdr, mapped, cleanup, nil
}

// mmapSmaps reports the RSS and PSS of this process's mapping of path, from
// /proc/self/smaps. found is false, with no error, where smaps doesn't exist.
func mmapSmaps(path string) (rss, pss uint64, found bool, err error) {
	f, err := os.Open("/proc/self/smaps")
	if err != nil {
		if os.IsNotExist(err) {
			return 0, 0, false, nil
		}
		return 0, 0, false, err
	}
	defer f.Close()

	// smaps prints the path the kernel resolved, so symlinks must be too.
	abs, err := filepath.Abs(path)
	if err != nil {
		return 0, 0, false, err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return 0, 0, false, err
	}
	return parseSmaps(f, resolved)
}

// parseSmaps sums the Rss and Pss lines of every smaps region whose backing
// path equals mappedPath. A file mapped more than once contributes each
// region.
func parseSmaps(r io.Reader, mappedPath string) (rss, pss uint64, found bool, err error) {
	match := false
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if isSmapsHeader(line) {
			fields := strings.Fields(line)
			match = len(fields) >= 6 && strings.Join(fields[5:], " ") == mappedPath
			continue
		}
		if !match {
			continue
		}
		switch {
		case strings.HasPrefix(line, "Rss:"):
			if kb, ok := parseSmapsKB(line); ok {
				rss += kb * 1024
				found = true
			}
		case strings.HasPrefix(line, "Pss:"):
			if kb, ok := parseSmapsKB(line); ok {
				pss += kb * 1024
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, 0, false, err
	}
	return rss, pss, found, nil
}

// isSmapsHeader recognises a region header such as
// "7f8a2c000000-7f8a2c021000 r--s 00000000 08:01 1234 /path": an address
// range, then a four-character permission field.
func isSmapsHeader(line string) bool {
	fields := strings.Fields(line)
	return len(fields) >= 5 && strings.Contains(fields[0], "-") && len(fields[1]) == 4
}

// parseSmapsKB reads the number out of a "Key:   123 kB" line.
func parseSmapsKB(line string) (uint64, bool) {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return 0, false
	}
	v, err := strconv.ParseUint(fields[1], 10, 64)
	return v, err == nil
}
