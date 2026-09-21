package bench

import (
	"encoding/json"
	"fmt"
	"math"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maxmind/mmdbwriter"
	"github.com/maxmind/mmdbwriter/mmdbtype"
)

// writeTestMMDB writes a small IPv4 database to a temp file and returns its
// path.
func writeTestMMDB(t *testing.T) string {
	t.Helper()
	w, err := mmdbwriter.New(mmdbwriter.Options{
		DatabaseType:            "bench-test",
		IPVersion:               4,
		RecordSize:              24,
		IncludeReservedNetworks: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i < 224; i += 7 {
		_, n, err := net.ParseCIDR(fmt.Sprintf("%d.0.0.0/8", i))
		if err != nil {
			t.Fatal(err)
		}
		rec := mmdbtype.Map{"country": mmdbtype.String(fmt.Sprintf("C%d", i))}
		if err := w.Insert(n, rec); err != nil {
			t.Fatalf("insert %s: %v", n, err)
		}
	}

	path := filepath.Join(t.TempDir(), "db.mmdb")
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

func TestPercentile(t *testing.T) {
	sorted := []float64{10, 20, 30, 40}
	cases := []struct {
		p    float64
		want float64
	}{
		{-1, 10},  // clamps low
		{0, 10},   // first
		{1, 40},   // last
		{2, 40},   // clamps high
		{0.5, 25}, // between 20 and 30
		{0.25, 17.5},
		{1.0 / 3, 20}, // lands exactly on index 1
	}
	for _, c := range cases {
		if got := percentile(sorted, c.p); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("percentile(p=%v) = %v, want %v", c.p, got, c.want)
		}
	}
	if got := percentile(nil, 0.5); !math.IsNaN(got) {
		t.Errorf("percentile(empty) = %v, want NaN", got)
	}
}

func TestSummarize(t *testing.T) {
	// Deliberately unsorted; one obvious outlier at 100.
	xs := []float64{30, 10, 100, 20, 40}
	s := summarize(xs, 0.2) // drop the slowest 20% = 1 sample

	if s.N != 5 {
		t.Errorf("N = %d, want 5", s.N)
	}
	if s.Min != 10 || s.Max != 100 {
		t.Errorf("min/max = %v/%v, want 10/100", s.Min, s.Max)
	}
	if s.Median != 30 {
		t.Errorf("median = %v, want 30", s.Median)
	}
	if want := 40.0; s.Mean != want {
		t.Errorf("mean = %v, want %v", s.Mean, want)
	}
	// Trimmed drops the 100: mean of {10,20,30,40} = 25.
	if want := 25.0; s.Trimmed != want {
		t.Errorf("trimmed = %v, want %v", s.Trimmed, want)
	}
	// Population std of {10,20,30,40,100}: variance = 1000, std ≈ 31.62.
	if want := math.Sqrt(1000); math.Abs(s.Std-want) > 1e-9 {
		t.Errorf("std = %v, want %v", s.Std, want)
	}
	if got := summarize(nil, 0.1); got != (Stats{}) {
		t.Errorf("summarize(empty) = %+v, want zero Stats", got)
	}
}

func TestSummarizeTrimNeverDropsEverything(t *testing.T) {
	// trimPct close to 1 on a tiny sample must still keep at least one value.
	s := summarize([]float64{5, 6}, 0.99)
	if math.IsNaN(s.Trimmed) || s.Trimmed == 0 {
		t.Errorf("trimmed = %v, expected a real mean over the kept samples", s.Trimmed)
	}
}

func TestBootstrapMedianDeltaCI(t *testing.T) {
	diffs := []float64{-5, -4, -6, -5, -3, -7, -5, -4, -6, -5}

	lo1, hi1 := bootstrapMedianDeltaCI(diffs, 500, 7)
	lo2, hi2 := bootstrapMedianDeltaCI(diffs, 500, 7)
	if lo1 != lo2 || hi1 != hi2 {
		t.Errorf("same seed gave different CIs: [%v,%v] vs [%v,%v]", lo1, hi1, lo2, hi2)
	}
	if !(lo1 <= hi1) {
		t.Errorf("CI inverted: lo=%v hi=%v", lo1, hi1)
	}
	// Every diff is negative, so the CI on the median must be too.
	if hi1 >= 0 {
		t.Errorf("CI upper bound %v should be negative for all-negative diffs", hi1)
	}

	// Fewer than two samples: no interval.
	if lo, hi := bootstrapMedianDeltaCI([]float64{-1}, 100, 1); !math.IsNaN(lo) || !math.IsNaN(hi) {
		t.Errorf("single-sample CI = [%v,%v], want NaN", lo, hi)
	}
}

func TestVerdict(t *testing.T) {
	cases := []struct {
		lo, hi float64
		want   string
	}{
		{-8, -2, VerdictFaster}, // whole CI below zero
		{2, 8, VerdictSlower},   // whole CI above zero
		{-3, 3, VerdictNoise},   // straddles zero
		{-3, 0, VerdictNoise},   // touches zero: not strictly negative
		{0, 3, VerdictNoise},
	}
	for _, c := range cases {
		if got := verdict(c.lo, c.hi); got != c.want {
			t.Errorf("verdict(%v, %v) = %q, want %q", c.lo, c.hi, got, c.want)
		}
	}
}

func TestPairedRejectsSingleRound(t *testing.T) {
	// One round gives one paired diff, and no confidence interval: the NaN
	// bounds would make the report impossible to encode as JSON.
	opts := DefaultPairedOptions()
	opts.Rounds = 1
	_, err := Paired(nil, []Pair{{Name: "p", Base: "a.mmdb", Shrunk: "b.mmdb"}}, opts)
	if err == nil || !strings.Contains(err.Error(), "at least 2") {
		t.Fatalf("err = %v, want a Rounds error", err)
	}
}

func TestPairedReportEncodesAsJSON(t *testing.T) {
	path := writeTestMMDB(t)
	opts := PairedOptions{Rounds: 2, Lookups: 200, Warmup: 20, Seed: 1, TrimPct: 0.1, Bootstrap: 50}
	r, err := Paired(nil, []Pair{{Name: "p", Base: path, Shrunk: path}}, opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Entries) != 2 || len(r.Pairs) != 1 {
		t.Fatalf("got %d entries and %d pairs, want 2 and 1", len(r.Entries), len(r.Pairs))
	}
	if r.GoVersion == "" || r.GoMaxProcs <= 0 || r.BinaryArch == "" {
		t.Errorf("environment not recorded: %+v", r.Env)
	}
	if _, err := json.Marshal(r); err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
}
