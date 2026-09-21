package verify

import (
	"encoding/binary"
	"net/netip"
)

type addrProbe struct {
	label string
	ip    netip.Addr
}

// fixedProbes returns the deterministic edge addresses every run checks:
// IPv4 class and private/reserved boundaries, plus their IPv6 equivalents
// for IPv6 databases.
func fixedProbes(ipVersion uint) []addrProbe {
	v4 := []string{
		"0.0.0.0",
		"0.0.0.1",
		"9.255.255.255",
		"10.0.0.0",
		"10.255.255.255",
		"11.0.0.0",
		"100.63.255.255",
		"100.64.0.0",
		"100.127.255.255",
		"100.128.0.0",
		"127.0.0.0",
		"127.0.0.1",
		"127.255.255.255",
		"169.254.0.0",
		"169.254.255.255",
		"172.15.255.255",
		"172.16.0.0",
		"172.31.255.255",
		"172.32.0.0",
		"192.0.2.0",
		"192.168.0.0",
		"192.168.255.255",
		"223.255.255.255",
		"224.0.0.0",
		"239.255.255.255",
		"255.255.255.255",
	}
	probes := make([]addrProbe, 0, len(v4)+16)
	for _, ip := range v4 {
		probes = append(probes, addrProbe{label: "fixed", ip: netip.MustParseAddr(ip)})
	}
	if ipVersion != 6 {
		return probes
	}
	v6 := []string{
		"::",
		"::1",
		"::ffff:0:0",
		"::ffff:255.255.255.255",
		"2001:db8::",
		"2001:db8::1",
		"2001:db8:ffff:ffff:ffff:ffff:ffff:ffff",
		"fc00::",
		"fdff:ffff:ffff:ffff:ffff:ffff:ffff:ffff",
		"fe80::",
		"febf:ffff:ffff:ffff:ffff:ffff:ffff:ffff",
		"ff00::",
		"ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff",
	}
	for _, ip := range v6 {
		probes = append(probes, addrProbe{label: "fixed", ip: netip.MustParseAddr(ip)})
	}
	return probes
}

// prefixProbes returns the first address of pfx, plus its mid and last
// addresses when includeBoundaries is set.
func prefixProbes(pfx netip.Prefix, includeBoundaries bool) []addrProbe {
	pfx = pfx.Masked()
	probes := []addrProbe{{label: "first", ip: pfx.Addr()}}
	if !includeBoundaries {
		return probes
	}
	mid, last := prefixMidLast(pfx)
	probes = appendDistinctProbe(probes, "mid", mid)
	probes = appendDistinctProbe(probes, "last", last)
	return probes
}

// adjacentProbes returns the addresses immediately before and after pfx,
// when they exist within the same address family.
func adjacentProbes(pfx netip.Prefix) []addrProbe {
	pfx = pfx.Masked()
	first := pfx.Addr()
	_, last := prefixMidLast(pfx)
	probes := []addrProbe{}
	if prev := first.Prev(); prev.IsValid() && prev.BitLen() == first.BitLen() {
		probes = append(probes, addrProbe{label: "before", ip: prev})
	}
	if next := last.Next(); next.IsValid() && next.BitLen() == last.BitLen() {
		probes = append(probes, addrProbe{label: "after", ip: next})
	}
	return probes
}

func appendDistinctProbe(probes []addrProbe, label string, ip netip.Addr) []addrProbe {
	for _, probe := range probes {
		if probe.ip == ip {
			return probes
		}
	}
	return append(probes, addrProbe{label: label, ip: ip})
}

func prefixMidLast(pfx netip.Prefix) (netip.Addr, netip.Addr) {
	pfx = pfx.Masked()
	first := pfx.Addr()
	if first.Is4() {
		return prefixMidLast4(first, pfx.Bits())
	}
	return prefixMidLast16(first, pfx.Bits())
}

func prefixMidLast4(first netip.Addr, prefixBits int) (netip.Addr, netip.Addr) {
	a4 := first.As4()
	v := binary.BigEndian.Uint32(a4[:])
	hostBits := 32 - prefixBits
	mid := v
	last := v
	if hostBits > 0 {
		mid += uint32(1) << uint(hostBits-1)
		if hostBits == 32 {
			last = ^uint32(0)
		} else {
			last |= (uint32(1) << uint(hostBits)) - 1
		}
	}
	return addrFromUint32(mid), addrFromUint32(last)
}

func addrFromUint32(v uint32) netip.Addr {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], v)
	return netip.AddrFrom4(b)
}

func prefixMidLast16(first netip.Addr, prefixBits int) (netip.Addr, netip.Addr) {
	mid := first.As16()
	last := first.As16()
	if prefixBits < 128 {
		setBit16(&mid, prefixBits)
		fillHostBits16(&last, prefixBits)
	}
	return netip.AddrFrom16(mid), netip.AddrFrom16(last)
}

func setBit16(b *[16]byte, bit int) {
	b[bit/8] |= byte(1 << uint(7-bit%8))
}

func fillHostBits16(b *[16]byte, prefixBits int) {
	if prefixBits >= 128 {
		return
	}
	idx := prefixBits / 8
	if rem := prefixBits % 8; rem != 0 {
		b[idx] |= byte(1<<uint(8-rem)) - 1
		idx++
	}
	for ; idx < len(b); idx++ {
		b[idx] = 0xff
	}
}
