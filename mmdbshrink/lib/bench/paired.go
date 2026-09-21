package bench

import (
	"errors"
	"fmt"
	"log/slog"
	"math"
	"math/rand/v2"
	"os"
	"runtime"
	"sort"
	"time"

	"github.com/oschwald/maxminddb-golang/v2"
)

// PairedOptions configures Paired.
type PairedOptions struct {
	// Rounds is the number of independent measurement rounds. Every input is
	// measured once per round, in an order shuffled per round so a transient
	// load spike is not pinned to one input. Comparing pairs needs at least
	// two, since the confidence interval is built from per-round differences.
	Rounds uint
	// Lookups is the number of timed lookups per input per round.
	Lookups uint
	// Warmup is the number of untimed lookups run before each measurement.
	Warmup uint
	// Seed is the root seed. Per-round seeds derive from it, and within a
	// round every input sees the same address sequence, so paired differences
	// compare like for like.
	Seed uint64
	// TrimPct is the fraction of slowest rounds dropped when computing the
	// display-only trimmed mean. Headline numbers always use every round.
	// Must be in [0, 1).
	TrimPct float64
	// Bootstrap is the number of resamples used for the paired-delta
	// confidence interval.
	Bootstrap uint
}

// DefaultPairedOptions returns the workload the tool ships with.
func DefaultPairedOptions() PairedOptions {
	return PairedOptions{
		Rounds:    15,
		Lookups:   1_000_000,
		Warmup:    200_000,
		Seed:      1,
		TrimPct:   0.10,
		Bootstrap: 2000,
	}
}

// Pair names a baseline mmdb file and a candidate to compare against it.
type Pair struct {
	Name   string
	Base   string
	Shrunk string
}

// Verdicts Paired assigns to a pair from the sign of its confidence interval.
const (
	// VerdictFaster means the whole 95% CI of the latency delta is negative.
	VerdictFaster = "faster"
	// VerdictSlower means the whole 95% CI of the latency delta is positive.
	VerdictSlower = "slower"
	// VerdictNoise means the CI straddles zero: no statistically
	// significant difference.
	VerdictNoise = "noise"
)

// Stats summarises one input's per-round ns/op samples.
type Stats struct {
	N      int     `json:"n"`
	Mean   float64 `json:"mean_ns"`
	Std    float64 `json:"std_ns"`
	Median float64 `json:"median_ns"`
	P05    float64 `json:"p05_ns"`
	P95    float64 `json:"p95_ns"`
	Min    float64 `json:"min_ns"`
	Max    float64 `json:"max_ns"`
	// Trimmed is display-only: the mean after a one-sided drop of the
	// slowest TrimPct rounds. Headline numbers use the full set.
	Trimmed float64 `json:"trimmed_mean_ns"`
}

// EntryReport is the result for one measured file.
type EntryReport struct {
	Label  string    `json:"label"`
	Pair   string    `json:"pair,omitempty"`
	Role   string    `json:"role"`
	Path   string    `json:"path"`
	Size   int64     `json:"size_bytes"`
	Stats  Stats     `json:"stats"`
	Rounds []float64 `json:"rounds_ns"`
}

// PairReport is the paired comparison for one Pair.
type PairReport struct {
	Name         string  `json:"name"`
	BaseSize     int64   `json:"base_size_bytes"`
	ShrunkSize   int64   `json:"shrunk_size_bytes"`
	SizeDeltaPct float64 `json:"size_delta_pct"`
	BaseStats    Stats   `json:"base_stats"`
	ShrunkStats  Stats   `json:"shrunk_stats"`
	// DiffsNs holds the per-round paired differences, shrunk minus base.
	DiffsNs       []float64 `json:"diffs_ns_per_round"`
	DeltaMedianNs float64   `json:"delta_median_ns"`
	DeltaMeanNs   float64   `json:"delta_mean_ns"`
	DeltaCI95LoNs float64   `json:"delta_ci95_lo_ns"`
	DeltaCI95HiNs float64   `json:"delta_ci95_hi_ns"`
	// LatencyDeltaPct is the median of the per-round paired deltas as a
	// percentage of the base median. Negative means shrunk is faster.
	LatencyDeltaPct     float64 `json:"latency_delta_pct"`
	LatencyDeltaCILoPct float64 `json:"latency_delta_ci_lo_pct"`
	LatencyDeltaCIHiPct float64 `json:"latency_delta_ci_hi_pct"`
	// Verdict is one of VerdictFaster, VerdictSlower or VerdictNoise.
	Verdict string `json:"verdict"`
}

// PairedReport is what Paired returns.
type PairedReport struct {
	Rounds    uint          `json:"rounds"`
	Lookups   uint          `json:"bench_n"`
	Warmup    uint          `json:"warmup_n"`
	TrimPct   float64       `json:"trim_pct"`
	Bootstrap uint          `json:"bootstrap_b"`
	Seed      uint64        `json:"seed"`
	Entries   []EntryReport `json:"entries"`
	Pairs     []PairReport  `json:"pairs"`
	StartedAt time.Time     `json:"started_at"`
	Env
}

// entry is one file under measurement.
type entry struct {
	label   string
	pair    string
	role    string
	path    string
	size    int64
	db      *maxminddb.Reader
	roundNs []float64
}

type pairEntries struct {
	name   string
	base   *entry
	shrunk *entry
}

// Paired benchmarks every file in pairs over opts.Rounds rounds and reports
// per-file statistics plus, for each Pair, the paired latency delta with a
// bootstrap confidence interval.
//
// It exists because a single measurement is too noisy to trust: a load spike
// during the run skews the result with no way to tell it happened. Measuring
// in rounds, shuffling order each round, and pairing within a round means
// baseline and candidate see similar conditions over the same wall-clock
// window.
//
// Each measurement is preceded by a GC so every timed window starts from the
// same heap state. Lookups allocate, so collections still run inside the
// window and are part of the measured cost. The GC is process-wide.
//
// A nil logger discards progress.
func Paired(logger *slog.Logger, pairs []Pair, opts PairedOptions) (PairedReport, error) {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	if opts.Rounds < 2 {
		return PairedReport{}, errors.New("Rounds must be at least 2 to compare pairs")
	}
	if opts.Lookups == 0 {
		return PairedReport{}, errors.New("Lookups must be positive")
	}
	if opts.Bootstrap == 0 {
		return PairedReport{}, errors.New("Bootstrap must be positive")
	}
	if opts.TrimPct < 0 || opts.TrimPct >= 1 {
		return PairedReport{}, errors.New("TrimPct must be in [0, 1)")
	}
	if len(pairs) == 0 {
		return PairedReport{}, errors.New("nothing to benchmark")
	}

	var entries []*entry
	closeAll := func() {
		for _, e := range entries {
			_ = e.db.Close()
		}
	}
	defer closeAll()

	var pes []pairEntries
	for _, p := range pairs {
		base, err := openEntry(p.Base, p.Name+":base", p.Name, "base")
		if err != nil {
			return PairedReport{}, err
		}
		entries = append(entries, base)
		shrunk, err := openEntry(p.Shrunk, p.Name+":shrunk", p.Name, "shrunk")
		if err != nil {
			return PairedReport{}, err
		}
		entries = append(entries, shrunk)
		pes = append(pes, pairEntries{name: p.Name, base: base, shrunk: shrunk})
	}

	logger.Debug(fmt.Sprintf("bench: %d entries, %d pairs, %d rounds × %d lookups (warmup=%d)",
		len(entries), len(pes), opts.Rounds, opts.Lookups, opts.Warmup))

	orderRng := rand.New(rand.NewPCG(opts.Seed, opts.Seed^0xdadcafe))
	startedAt := time.Now()

	for r := uint(0); r < opts.Rounds; r++ {
		order := make([]int, len(entries))
		for i := range order {
			order[i] = i
		}
		orderRng.Shuffle(len(order), func(i, j int) { order[i], order[j] = order[j], order[i] })
		// Same workload trace for every entry in the round, so within-round
		// paired diffs come from the same lookup sequence.
		roundSeed := opts.Seed*1_000_003 + uint64(r)*2_654_435_761

		for _, idx := range order {
			e := entries[idx]
			ns := runOne(e, roundSeed, opts.Warmup, opts.Lookups)
			e.roundNs = append(e.roundNs, ns)
			logger.Debug(fmt.Sprintf("  round %2d/%d  %-40s  %7.1f ns/op", r+1, opts.Rounds, e.label, ns))
		}
	}

	report := PairedReport{
		Rounds:    opts.Rounds,
		Lookups:   opts.Lookups,
		Warmup:    opts.Warmup,
		TrimPct:   opts.TrimPct,
		Bootstrap: opts.Bootstrap,
		Seed:      opts.Seed,
		StartedAt: startedAt,
		Env:       currentEnv(),
	}

	for _, e := range entries {
		report.Entries = append(report.Entries, EntryReport{
			Label:  e.label,
			Pair:   e.pair,
			Role:   e.role,
			Path:   e.path,
			Size:   e.size,
			Stats:  summarize(e.roundNs, opts.TrimPct),
			Rounds: append([]float64(nil), e.roundNs...),
		})
	}

	for _, p := range pes {
		bs := summarize(p.base.roundNs, opts.TrimPct)
		ss := summarize(p.shrunk.roundNs, opts.TrimPct)

		diffs := make([]float64, len(p.base.roundNs))
		for i := range diffs {
			diffs[i] = p.shrunk.roundNs[i] - p.base.roundNs[i]
		}
		sortedDiffs := append([]float64(nil), diffs...)
		sort.Float64s(sortedDiffs)
		dMedian := percentile(sortedDiffs, 0.5)
		dMean := 0.0
		for _, d := range diffs {
			dMean += d
		}
		if len(diffs) > 0 {
			dMean /= float64(len(diffs))
		}
		lo, hi := bootstrapMedianDeltaCI(diffs, opts.Bootstrap, opts.Seed^0xb00150)

		sizeDelta := 0.0
		if p.base.size > 0 {
			sizeDelta = float64(p.shrunk.size-p.base.size) * 100 / float64(p.base.size)
		}

		pctMid, pctLo, pctHi := math.NaN(), math.NaN(), math.NaN()
		if bs.Median != 0 {
			pctMid = dMedian * 100 / bs.Median
			pctLo = lo * 100 / bs.Median
			pctHi = hi * 100 / bs.Median
		}

		report.Pairs = append(report.Pairs, PairReport{
			Name:                p.name,
			BaseSize:            p.base.size,
			ShrunkSize:          p.shrunk.size,
			SizeDeltaPct:        sizeDelta,
			BaseStats:           bs,
			ShrunkStats:         ss,
			DiffsNs:             diffs,
			DeltaMedianNs:       dMedian,
			DeltaMeanNs:         dMean,
			DeltaCI95LoNs:       lo,
			DeltaCI95HiNs:       hi,
			LatencyDeltaPct:     pctMid,
			LatencyDeltaCILoPct: pctLo,
			LatencyDeltaCIHiPct: pctHi,
			Verdict:             verdict(pctLo, pctHi),
		})
	}

	return report, nil
}

func openEntry(path, label, pair, role string) (*entry, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	db, err := maxminddb.Open(path)
	if err != nil {
		return nil, err
	}
	return &entry{label: label, pair: pair, role: role, path: path, size: st.Size(), db: db}, nil
}

// runOne measures one (file, seed) pass: warmup, a GC, then the timed
// lookups. Each lookup decodes into a fresh map so the full decode path is
// exercised, not just the tree walk. Returns ns/op.
func runOne(e *entry, seed uint64, warmupN, benchN uint) float64 {
	mkW := makeIPv4(seed ^ 0x77777777)
	var sink map[string]any
	for range warmupN {
		sink = map[string]any{}
		_ = e.db.Lookup(mkW()).Decode(&sink)
	}
	// No FreeOSMemory: returning the heap to the OS would only make the timed
	// loop re-fault it.
	runtime.GC()

	mk := makeIPv4(seed)
	t0 := time.Now()
	for range benchN {
		sink = map[string]any{}
		_ = e.db.Lookup(mk()).Decode(&sink)
	}
	dt := time.Since(t0)
	_ = sink
	return float64(dt.Nanoseconds()) / float64(benchN)
}

// verdict classifies a pair from the 95% CI of its latency delta in percent.
// A CI strictly below zero is an improvement, strictly above a regression,
// anything straddling zero is not statistically significant.
func verdict(ciLoPct, ciHiPct float64) string {
	switch {
	case ciHiPct < 0:
		return VerdictFaster
	case ciLoPct > 0:
		return VerdictSlower
	default:
		return VerdictNoise
	}
}

// percentile returns the linearly interpolated p-quantile of sorted.
func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return math.NaN()
	}
	if p <= 0 {
		return sorted[0]
	}
	if p >= 1 {
		return sorted[len(sorted)-1]
	}
	idx := p * float64(len(sorted)-1)
	lo := int(math.Floor(idx))
	hi := int(math.Ceil(idx))
	if lo == hi {
		return sorted[lo]
	}
	return sorted[lo] + (idx-float64(lo))*(sorted[hi]-sorted[lo])
}

// summarize computes descriptive statistics over xs. The trimmed mean drops
// the slowest trimPct fraction of samples; everything else uses all of them.
func summarize(xs []float64, trimPct float64) Stats {
	if len(xs) == 0 {
		return Stats{}
	}
	cp := append([]float64(nil), xs...)
	sort.Float64s(cp)

	mean := 0.0
	for _, x := range cp {
		mean += x
	}
	mean /= float64(len(cp))

	v := 0.0
	for _, x := range cp {
		v += (x - mean) * (x - mean)
	}
	std := math.Sqrt(v / float64(len(cp)))

	// One-sided trim: the slowest rounds are where load spikes land.
	dropHi := int(math.Floor(float64(len(cp))*trimPct + 0.5))
	if dropHi >= len(cp) {
		dropHi = len(cp) - 1
	}
	keep := cp[:len(cp)-dropHi]
	if len(keep) == 0 {
		keep = cp
	}
	trimmed := 0.0
	for _, x := range keep {
		trimmed += x
	}
	trimmed /= float64(len(keep))

	return Stats{
		N:       len(cp),
		Mean:    mean,
		Std:     std,
		Median:  percentile(cp, 0.5),
		P05:     percentile(cp, 0.05),
		P95:     percentile(cp, 0.95),
		Min:     cp[0],
		Max:     cp[len(cp)-1],
		Trimmed: trimmed,
	}
}

// bootstrapMedianDeltaCI resamples diffs resample times, takes the median of each
// resample, and returns the 2.5th and 97.5th percentiles of those medians:
// a 95% confidence interval on the median delta.
func bootstrapMedianDeltaCI(diffs []float64, resample uint, seed uint64) (lo, hi float64) {
	if len(diffs) < 2 {
		return math.NaN(), math.NaN()
	}
	r := rand.New(rand.NewPCG(seed, seed^0xb007))
	medians := make([]float64, resample)
	tmp := make([]float64, len(diffs))
	for j := range resample {
		for k := range tmp {
			tmp[k] = diffs[r.IntN(len(diffs))]
		}
		sort.Float64s(tmp)
		medians[j] = percentile(tmp, 0.5)
	}
	sort.Float64s(medians)
	return percentile(medians, 0.025), percentile(medians, 0.975)
}
