//go:build linux || darwin

package bench

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestMemorySampleEncodesAsJSON(t *testing.T) {
	path := writeTestMMDB(t)
	s, err := Memory(nil, path, MemoryOptions{Lookups: 200, Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	// Zero warmup is honoured, not replaced by a default.
	if s.N != 200 || s.WarmupN != 0 {
		t.Errorf("N/WarmupN = %d/%d, want 200/0", s.N, s.WarmupN)
	}
	// Faults are a delta over the measured pass, so never negative.
	if s.MinorFaults < 0 || s.MajorFaults < 0 {
		t.Errorf("fault deltas = %d/%d, want non-negative", s.MinorFaults, s.MajorFaults)
	}
	if s.GoVersion == "" {
		t.Error("environment not recorded")
	}
	if _, err := json.Marshal(s); err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
}

func TestMmapSmapsFollowsSymlinks(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("smaps is Linux only")
	}
	link := filepath.Join(t.TempDir(), "current.mmdb")
	if err := os.Symlink(writeTestMMDB(t), link); err != nil {
		t.Fatal(err)
	}
	_, _, cleanup, err := openMappedReader(link)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	_, _, found, err := mmapSmaps(link)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Error("mapping opened through a symlink was not found in smaps")
	}
}

func TestMemoryColdOnMacOSFailsBeforeOpening(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS only")
	}
	// A missing file proves the check runs before the file is even opened.
	_, err := Memory(nil, filepath.Join(t.TempDir(), "missing.mmdb"), MemoryOptions{Lookups: 1, Cold: true})
	if !errors.Is(err, errColdDarwin) {
		t.Fatalf("err = %v, want errColdDarwin", err)
	}
}
