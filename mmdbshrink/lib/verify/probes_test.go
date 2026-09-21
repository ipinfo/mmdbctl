package verify

import (
	"net/netip"
	"strings"
	"testing"
)

func TestPrefixMidLast(t *testing.T) {
	cases := []struct{ pfx, mid, last string }{
		{"10.0.0.0/8", "10.128.0.0", "10.255.255.255"},
		{"0.0.0.0/0", "128.0.0.0", "255.255.255.255"},
		{"1.2.3.0/25", "1.2.3.64", "1.2.3.127"},
		{"1.2.3.0/31", "1.2.3.1", "1.2.3.1"},
		{"1.2.3.4/32", "1.2.3.4", "1.2.3.4"},
		{"2001:db8::/32", "2001:db8:8000::", "2001:db8:ffff:ffff:ffff:ffff:ffff:ffff"},
		{"2001:db8::/33", "2001:db8:4000::", "2001:db8:7fff:ffff:ffff:ffff:ffff:ffff"},
		{"::/0", "8000::", "ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff"},
		{"::1/128", "::1", "::1"},
		// An IPv4 network inside an IPv6 tree keeps its 128-bit form.
		{"::ffff:1.2.3.0/120", "::ffff:1.2.3.128", "::ffff:1.2.3.255"},
	}
	for _, c := range cases {
		mid, last := prefixMidLast(netip.MustParsePrefix(c.pfx))
		if mid.String() != c.mid || last.String() != c.last {
			t.Errorf("%s: mid=%s last=%s, want %s %s", c.pfx, mid, last, c.mid, c.last)
		}
	}
}

func TestAdjacentProbesStayInsideTheAddressFamily(t *testing.T) {
	cases := []struct{ pfx, want string }{
		{"10.0.0.0/8", "before=9.255.255.255 after=11.0.0.0"},
		{"0.0.0.0/8", "after=1.0.0.0"},
		{"255.0.0.0/8", "before=254.255.255.255"},
		{"0.0.0.0/0", ""},
		{"2001:db8::/32", "before=2001:db7:ffff:ffff:ffff:ffff:ffff:ffff after=2001:db9::"},
		{"ffff::/16", "before=fffe:ffff:ffff:ffff:ffff:ffff:ffff:ffff"},
		{"::/0", ""},
	}
	for _, c := range cases {
		var got []string
		for _, p := range adjacentProbes(netip.MustParsePrefix(c.pfx)) {
			got = append(got, p.label+"="+p.ip.String())
		}
		if s := strings.Join(got, " "); s != c.want {
			t.Errorf("%s: got %q, want %q", c.pfx, s, c.want)
		}
	}
}

func TestPrefixProbesDedupeBoundaries(t *testing.T) {
	cases := []struct {
		pfx  string
		want int
	}{
		{"10.0.0.0/8", 3}, // first, mid, last
		{"1.2.3.0/31", 2}, // mid and last coincide
		{"1.2.3.4/32", 1}, // all three coincide
	}
	for _, c := range cases {
		if got := prefixProbes(netip.MustParsePrefix(c.pfx), true); len(got) != c.want {
			t.Errorf("%s with boundaries: %d probes, want %d", c.pfx, len(got), c.want)
		}
	}
	if got := prefixProbes(netip.MustParsePrefix("10.0.0.0/8"), false); len(got) != 1 || got[0].label != "first" {
		t.Errorf("without boundaries: %+v, want just the first address", got)
	}
}
