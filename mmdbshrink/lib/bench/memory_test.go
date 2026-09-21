package bench

import (
	"strings"
	"testing"
)

// smapsFixture is a trimmed /proc/self/smaps: two regions backing the file we
// care about, one region for an unrelated file, and one anonymous region. Only
// the first two must be summed.
const smapsFixture = `7f8a2c000000-7f8a2c021000 r--s 00000000 08:01 1234 /data/db.mmdb
Size:                132 kB
Rss:                 100 kB
Pss:                  50 kB
Shared_Clean:        100 kB
7f8a2c021000-7f8a2c030000 r--s 00021000 08:01 1234 /data/db.mmdb
Size:                 60 kB
Rss:                  20 kB
Pss:                  10 kB
7f8a2d000000-7f8a2d100000 r--s 00000000 08:01 9999 /data/other.mmdb
Rss:                 999 kB
Pss:                 999 kB
7f8a2e000000-7f8a2e021000 rw-p 00000000 00:00 0
Rss:                 777 kB
Pss:                 777 kB
`

func TestParseSmapsSumsOnlyMatchingRegions(t *testing.T) {
	rss, pss, found, err := parseSmaps(strings.NewReader(smapsFixture), "/data/db.mmdb")
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("found = false, want true")
	}
	if want := uint64((100 + 20) * 1024); rss != want {
		t.Errorf("rss = %d, want %d", rss, want)
	}
	if want := uint64((50 + 10) * 1024); pss != want {
		t.Errorf("pss = %d, want %d", pss, want)
	}
}

func TestParseSmapsNoMatch(t *testing.T) {
	rss, pss, found, err := parseSmaps(strings.NewReader(smapsFixture), "/data/missing.mmdb")
	if err != nil {
		t.Fatal(err)
	}
	if found || rss != 0 || pss != 0 {
		t.Errorf("got found=%v rss=%d pss=%d for an unmapped path, want false/0/0", found, rss, pss)
	}
}

func TestParseSmapsPathWithSpaces(t *testing.T) {
	// smaps prints the path verbatim, so a path with spaces spans several
	// fields and must be rejoined before comparing.
	in := "7f0000000000-7f0000001000 r--s 00000000 08:01 1 /data/my db.mmdb\nRss:  4 kB\nPss:  4 kB\n"
	_, _, found, err := parseSmaps(strings.NewReader(in), "/data/my db.mmdb")
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Error("path containing a space was not matched")
	}
}

func TestIsSmapsHeader(t *testing.T) {
	cases := []struct {
		line string
		want bool
	}{
		{"7f8a2c000000-7f8a2c021000 r--s 00000000 08:01 1234 /data/db.mmdb", true},
		{"7f8a2e000000-7f8a2e021000 rw-p 00000000 00:00 0", true}, // anonymous, no path
		{"Rss:                 100 kB", false},
		{"Size:                132 kB", false},
		{"VmFlags: rd mr mw me", false},
		{"", false},
	}
	for _, c := range cases {
		if got := isSmapsHeader(c.line); got != c.want {
			t.Errorf("isSmapsHeader(%q) = %v, want %v", c.line, got, c.want)
		}
	}
}

func TestParseSmapsKB(t *testing.T) {
	if v, ok := parseSmapsKB("Rss:                 100 kB"); !ok || v != 100 {
		t.Errorf("got (%d, %v), want (100, true)", v, ok)
	}
	if _, ok := parseSmapsKB("Rss:"); ok {
		t.Error("line with no value parsed as ok")
	}
	if _, ok := parseSmapsKB("Rss: lots kB"); ok {
		t.Error("non-numeric value parsed as ok")
	}
}
