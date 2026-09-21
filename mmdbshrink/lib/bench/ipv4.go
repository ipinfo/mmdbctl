package bench

import (
	"math/rand/v2"
	"net/netip"
)

// The half-open range makeIPv4 draws from: 1.0.0.0 up to but not including
// 224.0.0.0. That spans classes A–C, including some reserved and special-use
// blocks (10/8, 100.64/10, 127/8, 169.254/16, 192.168/16, TEST-NET), so it is
// not limited to globally allocated space.
const (
	ipv4RangeStart = 0x01000000
	ipv4RangeEnd   = 0xE0000000
)

// makeIPv4 returns a generator of random IPv4 addresses drawn uniformly from
// [ipv4RangeStart, ipv4RangeEnd). Generators built from the same seed produce
// the same sequence, which is what lets two bench runs compare like for like.
func makeIPv4(seed uint64) func() netip.Addr {
	r := rand.New(rand.NewPCG(seed, seed^0xdeadbeef))
	return func() netip.Addr {
		v := r.Uint32N(ipv4RangeEnd-ipv4RangeStart) + ipv4RangeStart
		return netip.AddrFrom4([4]byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)})
	}
}
