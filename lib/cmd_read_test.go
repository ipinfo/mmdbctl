package lib

import (
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"testing"

	"github.com/oschwald/maxminddb-golang/v2"
)

// TestReadLookupOnIPv6Database verifies that both IPv4 and IPv6 lookups work
// on IPv6 databases. The IPv4 cases are a regression test: netip.AddrFromSlice
// on a 16-byte net.IP (as returned by net.ParseIP for IPv4) produces an
// IPv4-mapped-IPv6 address (::ffff:x.x.x.x) which maxminddb-golang/v2 does
// not resolve through the IPv4 subtree. The fix is to call .Unmap() before
// lookup.
func TestReadLookupOnIPv6Database(t *testing.T) {
	// Create a small IPv6 MMDB with IPv4 data via import.
	tempDir := t.TempDir()
	inputFile := filepath.Join(tempDir, "input.csv")
	mmdbFile := filepath.Join(tempDir, "test.mmdb")

	csvData := "network,descr\n127.0.0.0/8,Loopback\n192.168.0.0/16,Private-Use\n2001:db8::/32,Documentation\nfc00::/7,Unique-Local\n"
	if err := os.WriteFile(inputFile, []byte(csvData), 0644); err != nil {
		t.Fatal(err)
	}

	err := CmdImport(CmdImportFlags{
		Ip:    6,
		Size:  32,
		Merge: "none",
		In:    inputFile,
		Out:   mmdbFile,
		Csv:   true,
	}, []string{}, func() {})
	if err != nil {
		t.Fatalf("import failed: %s", err)
	}

	db, err := maxminddb.Open(mmdbFile)
	if err != nil {
		t.Fatalf("failed to open MMDB: %s", err)
	}
	defer db.Close()

	if db.Metadata.IPVersion != 6 {
		t.Fatalf("expected IPv6 database, got version %d", db.Metadata.IPVersion)
	}

	tests := []struct {
		ip       string
		expected string
	}{
		{"127.0.0.1", "Loopback"},
		{"192.168.1.1", "Private-Use"},
		{"2001:db8::1", "Documentation"},
		{"fd00::1", "Unique-Local"},
	}

	for _, tc := range tests {
		t.Run(tc.ip, func(t *testing.T) {
			// net.ParseIP returns a 16-byte slice for IPv4, same as
			// iputil.IPListFromAllSrcs. This is the code path that
			// CmdRead uses.
			ip := net.ParseIP(tc.ip)
			if len(ip) != 16 {
				t.Fatalf("expected 16-byte IP, got %d", len(ip))
			}

			addr, ok := netip.AddrFromSlice(ip)
			if !ok {
				t.Fatalf("AddrFromSlice failed for %s", tc.ip)
			}
			addr = addr.Unmap()

			var record map[string]interface{}
			if err := db.Lookup(addr).Decode(&record); err != nil {
				t.Fatalf("lookup failed: %s", err)
			}
			if len(record) == 0 {
				t.Fatalf("empty record for %s", tc.ip)
			}
			if got := record["descr"]; got != tc.expected {
				t.Errorf("got descr=%v, want %s", got, tc.expected)
			}
		})
	}
}
