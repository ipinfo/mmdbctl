package cmd

import (
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/ipinfo/mmdbctl/mmdbshrink/lib/verify"
)

func TestExitCode(t *testing.T) {
	cases := []struct {
		err  error
		want int
	}{
		{&verify.CheckError{Check: verify.CheckMetadata}, 4},
		{&verify.CheckError{Check: verify.CheckEnumerate}, 3},
		{&verify.CheckError{Check: verify.CheckReverseEnum}, 3},
		{&verify.CheckError{Check: verify.CheckExactPrefixes}, 3},
		{&verify.CheckError{Check: verify.CheckSchema}, 3},
		{&verify.CheckError{Check: verify.CheckFixedProbes}, 2},
		{&verify.CheckError{Check: verify.CheckRandomIPv4}, 2},
		{&verify.CheckError{Check: verify.CheckRandomIPv6}, 2},
		{fmt.Errorf("wrapped: %w", &verify.CheckError{Check: verify.CheckMetadata}), 4},
		{errors.New("open x.mmdb: no such file or directory"), 1},
	}
	for _, c := range cases {
		if got := ExitCode(c.err); got != c.want {
			t.Errorf("ExitCode(%v) = %d, want %d", c.err, got, c.want)
		}
	}
}

func TestMemorySamplerAcceptsAnyProg(t *testing.T) {
	// Building the sampler must not index past the fields of prog, whether
	// it is "mmdbshrink", "mmdbctl shrink" or empty.
	for _, prog := range []string{"mmdbshrink", "mmdbctl shrink", ""} {
		if memorySampler(prog, 10, 1) == nil {
			t.Errorf("memorySampler(%q) returned nil", prog)
		}
	}
}

func TestDefaultOutputPath(t *testing.T) {
	cases := map[string]string{
		"foo.mmdb":            "foo.shrunk.mmdb",
		"dir/foo.mmdb":        "dir/foo.shrunk.mmdb",
		"foo.plain.mmdb":      "foo.plain.shrunk.mmdb",
		"foo":                 "foo.shrunk.mmdb",
		"/data/v1/core.mmdb":  "/data/v1/core.shrunk.mmdb",
		"archive.mmdb.backup": "archive.mmdb.backup.shrunk.mmdb",
	}
	for in, want := range cases {
		if got := defaultOutputPath(in); got != want {
			t.Errorf("defaultOutputPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSameFile(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("a.mmdb", nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if !sameFile("a.mmdb", "./a.mmdb") {
		t.Error("a.mmdb and ./a.mmdb should be the same file")
	}
	if sameFile("a.mmdb", "b.mmdb") {
		t.Error("a missing output can't be the input")
	}
}
