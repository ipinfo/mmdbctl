# [<img src="https://ipinfo.io/static/ipinfo-small.svg" alt="IPinfo" width="24"/>](https://ipinfo.io/) IPinfo `mmdbshrink`

`mmdbshrink` is a CLI by [IPinfo.io](https://ipinfo.io) that losslessly shrinks
MMDB files. It finds identical subtrees in the file's search tree and stores
each one only once. The result is a standard MMDB file that returns identical
lookup results and that any MMDB reader can open as is: there is nothing to
decompress before reading it.

Every shrink is checked: after writing the shrunk file, `mmdbshrink` confirms
that it answers lookups exactly like the original, and fails if it doesn't.

The same command is also available as `mmdbctl shrink`, part of the
[`mmdbctl`](https://github.com/ipinfo/mmdbctl) CLI.

## Results

On IPinfo's own databases, measured on 2026-09-28:

| Database                  | Before |  After | Reduction |
| ------------------------- | -----: | -----: | --------: |
| `ipinfo_lite`             |  38 MB |  25 MB |       34% |
| `ipinfo_core`             | 1.5 GB | 728 MB |       53% |
| `ipinfo_plus`             | 5.6 GB | 3.8 GB |       32% |
| `ipinfo_location`         | 687 MB | 316 MB |       54% |
| `location_extended_v2`    | 1.2 GB | 541 MB |       53% |
| `ipinfo_privacy`          | 789 MB | 155 MB |       80% |
| `ipinfo_privacy_extended` | 827 MB | 285 MB |       66% |
| `resproxy_7d`             | 3.5 GB | 2.6 GB |       27% |

How much a file shrinks depends on how much structure repeats in it. Dense
datasets with few distinct records, like privacy data, shrink the most. A file
that has already been shrunk won't shrink any further.

## Installation

`mmdbshrink` is released together with `mmdbctl`, under the same version.

### macOS

Install the latest version of both `mmdbctl` and `mmdbshrink`. The script
detects your Mac's architecture and installs the matching binaries: `arm64` on
Apple Silicon, including when run under Rosetta, and `amd64` on Intel:

```bash
curl -Ls https://github.com/ipinfo/mmdbctl/releases/download/mmdbctl-1.4.10/macos.sh | sh
```

### Debian / Ubuntu (amd64)

Installs both `mmdbctl` and `mmdbshrink`:

```bash
curl -Ls https://github.com/ipinfo/mmdbctl/releases/download/mmdbctl-1.4.10/deb.sh | sh
```

### Windows Powershell

_Note_: run powershell as administrator before executing this command.

```bash
iwr -useb https://github.com/ipinfo/mmdbctl/releases/download/mmdbctl-1.4.10/windows.ps1 | iex
```

### Using `go install`

Make sure that `$GOPATH/bin` is in your `$PATH`, because that's where this gets
installed:

```bash
go install github.com/ipinfo/mmdbctl/mmdbshrink@latest
```

### Using `curl`/`wget`

Pre-built binaries are available in the
[releases](https://github.com/ipinfo/mmdbctl/releases), for the same platforms
as `mmdbctl` (see its
[list](https://github.com/ipinfo/mmdbctl#using-curlwget)). After choosing a
platform `PLAT`, run:

```bash
# for Windows, use ".zip" instead of ".tar.gz"
curl -LO https://github.com/ipinfo/mmdbctl/releases/download/mmdbctl-1.4.10/mmdbshrink_1.4.10_${PLAT}.tar.gz
tar -xvf mmdbshrink_1.4.10_${PLAT}.tar.gz
mv mmdbshrink_1.4.10_${PLAT} /usr/local/bin/mmdbshrink
```

### Using `git`

Installing from source requires at least the Golang version specified in
`go.mod`:

```bash
git clone https://github.com/ipinfo/mmdbctl
cd mmdbctl
go install ./mmdbshrink
```

### Memory requirements

Shrinking loads the whole file into memory, and peak usage is roughly 6 to 12
times the input file size. `ipinfo_core` (1.5 GB) peaks at about 12 GB, and
`ipinfo_plus` (5.6 GB) at about 43 GB. Machines with less memory will still
finish if they can swap, but much more slowly.

## Quick Start

Shrink a file. The shrunk file is written next to it, with a `.shrunk.mmdb`
extension, and the input is left untouched:

```bash
$ mmdbshrink ipinfo_core.mmdb

input:   ipinfo_core.mmdb
output:  ipinfo_core.shrunk.mmdb

  size:   1.51 GB      ->  717.89 MB   (saved 791.08 MB, -52.43%)
  nodes:  169,017,278  ->  70,132,748  (-58.51%)
  tree:   1.35 GB      ->  561.06 MB
  data:   156.82 MB    ->  156.82 MB   (unchanged)

elapsed: 25.399s

validation (seed 1):
  [OK] metadata        ip_version=6  record_size=32  binary_format=2.0
                       database_type: ipinfo bundle_location_core.mmdb  build_epoch=1789632260
                       node_count: 169,017,278 -> 70,132,748 (-58.51%)
  [OK] fixed probes    n=39  hits=7  (1ms)
  [OK] random IPv4     n=1,000,000  hits=861,882 (86.19%)  mismatches=0  (7.471s)
  [OK] random IPv6     n=1,000,000  hits=1,985 (0.20%)  mismatches=0  (131ms)

bench (1,000,000 lookups each):
  input:    size=1.51 GB     per_op=2496ns  qps=400,492
  output:   size=717.89 MB   per_op=2460ns  qps=406,466
```

The result is a regular MMDB file, readable by any MMDB reader:

```bash
$ mmdbctl read 8.8.8.8 ipinfo_core.shrunk.mmdb
{"as_domain":"google.com","as_name":"Google LLC","as_type":"hosting","asn":"AS15169","city":"Mountain View","continent":"North America","continent_code":"NA","country":"United States","country_code":"US","ip":"8.8.8.8","is_anonymous":false,"is_anycast":true,"is_hosting":true,"is_mobile":false,"is_satellite":false,"latitude":38.00881,"longitude":-122.11746,"postal_code":"94043","region":"California","region_code":"CA","timezone":"America/Los_Angeles"}
```

Run `mmdbshrink --help` for all options.

## Usage

```
mmdbshrink [<opts>] <input_mmdb_file> [<output_mmdb_file>]
```

Without an output path, `foo.mmdb` is shrunk to `foo.shrunk.mmdb`. Pass an
output path to choose another name, and `--overwrite` to replace a file that
already exists.

### Validation

After writing the shrunk file, `mmdbshrink` checks it against the input: the
metadata, the IPv4 class and private/reserved boundaries (plus their IPv6
equivalents), and 1,000,000 random addresses per address family must all
answer identically. The addresses are deterministic, so two runs check the
same ones.

If any lookup differs, `mmdbshrink` reports the offending address and exits
non-zero, and the shrunk file must not be used:

| Exit code | Meaning                                    |
| --------: | ------------------------------------------ |
|         0 | shrunk and validated                       |
|         2 | a lookup returned a different result       |
|         4 | the metadata differs                       |
|         1 | any other error, e.g. a file can't be read |

`mmdbctl shrink` runs the same validation, but like every other `mmdbctl`
command it prints the error and exits 0.

The report ends with a `bench` section: 1,000,000 lookups timed on each file,
with `size` the file size on disk. It's a single run per file, so it shows
that lookup speed is about the same, and differences of a few percent either
way are noise.

### Full report

`--full-report` also measures how much memory a reader holds for the input
and the shrunk file after a lookup workload. Each file is measured in a fresh
process, so neither benefits from pages the other left in memory:

```bash
$ mmdbshrink --full-report ipinfo_core.mmdb
...
memory (200,000 lookups each, separate processes):
  input:    mmap_cached=910.41 MiB  proc_rss=839.86 MiB  per_op=8600ns  minflt=247  majflt=28,250
  output:   mmap_cached=461.92 MiB  proc_rss=463.16 MiB  per_op=4666ns  minflt=238  majflt=10,227
```

`proc_rss` is the reader's peak resident memory, and `mmap_cached`
(`mmap_rss` on Linux) how much of the file ended up in memory. `minflt` and
`majflt` count the page faults during the workload; they depend heavily on
what the OS already has cached, so they vary from run to run. Measuring
memory is supported on Linux and macOS.

### Dry run

To see how much a file would shrink without writing anything, use
`--dry-run`. It runs the full shrink and discards the output, so the numbers
are exact. Nothing is written, so nothing is validated:

```bash
$ mmdbshrink --dry-run ipinfo_core.mmdb

input:   ipinfo_core.mmdb
output:  /dev/null

  size:   1.51 GB      ->  717.89 MB   (saved 791.08 MB, -52.43%)
  nodes:  169,017,278  ->  70,132,748  (-58.51%)
  tree:   1.35 GB      ->  561.06 MB
  data:   156.82 MB    ->  156.82 MB   (unchanged)

elapsed: 20.719s
```

## Go Library

The shrinking tooling is also available as Go packages under
`github.com/ipinfo/mmdbctl/mmdbshrink/lib`. `shrink` and `verify` are the code
the CLI runs, and `verify` and `bench` also offer checks the CLI doesn't run
on every shrink: a walk of every prefix in both files, the strongest and
slowest equivalence check, and a noise-robust benchmark that compares lookup
speed over many interleaved rounds.

| Package      | Entry point                                          |
| ------------ | ---------------------------------------------------- |
| `lib/shrink` | `shrink.Shrink(logger, inputPath, outputPath, opts)` |
| `lib/verify` | `verify.Verify(logger, basePath, shrunkPath, opts)`  |
| `lib/bench`  | `bench.Paired(logger, pairs, opts)`                  |
| `lib/bench`  | `bench.Memory(logger, path, opts)`                   |

Every entry point takes a `*slog.Logger` and an options struct, and returns a
result struct and an error. Progress goes to the logger at Debug level, so
pass a logger with that level enabled to see it, or `nil` to discard it.
Start from a package's `DefaultOptions()` where one exists: the zero value is
valid but does not always match the CLI defaults. Nothing in `lib` prints,
reads flags or exits the process.

```go
package main

import (
	"errors"
	"fmt"
	"log"
	"log/slog"
	"os"

	"github.com/ipinfo/mmdbctl/mmdbshrink/lib/shrink"
	"github.com/ipinfo/mmdbctl/mmdbshrink/lib/verify"
)

func main() {
	// Progress is logged at Debug level; a nil logger discards it.
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	res, err := shrink.Shrink(logger, "ipinfo_core.mmdb", "ipinfo_core.shrunk.mmdb", shrink.Options{})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%d -> %d bytes, %d -> %d nodes\n",
		res.InputBytes, res.OutputBytes, res.InputNodeCount, res.OutputNodeCount)

	opts := verify.DefaultOptions()
	opts.Enumerate = false // skip the slow full prefix walk
	opts.Bench = false
	report, err := verify.Verify(nil, "ipinfo_core.mmdb", "ipinfo_core.shrunk.mmdb", opts)
	if err != nil {
		var check *verify.CheckError
		if errors.As(err, &check) {
			log.Fatalf("%s check failed: %s", check.Check, check.Detail)
		}
		log.Fatal(err)
	}
	fmt.Printf("%d random IPv4 lookups matched\n", report.RandomIPv4.N)
}
```

`verify.Verify` stops at the first failed check and returns it as a
`*verify.CheckError`, alongside a report of the checks that passed before it.

## Auto-Completion

Auto-completion is supported for at least `bash`, `zsh` and `fish`. Install
it with:

```bash
mmdbshrink completion install
```

To customize the installation, print the completion script for your shell
instead:

```bash
mmdbshrink completion bash
mmdbshrink completion zsh
mmdbshrink completion fish
```

## Other IPinfo Tools

There are official IPinfo client libraries available for many languages including PHP, Python, Go, Java, Ruby, and many popular frameworks such as Django, Rails and Laravel. There are also many third party libraries and integrations available for our API.

See [https://ipinfo.io/developers/libraries](https://ipinfo.io/developers/libraries) for more details.

## About IPinfo

Founded in 2013, IPinfo prides itself on being the most reliable, accurate, and in-depth source of IP address data available anywhere. We process terabytes of data to produce our custom IP geolocation, company, carrier, VPN detection, hosted domains, and IP type data sets. Our API handles over 40 billion requests a month for businesses and developers.

[![image](https://avatars3.githubusercontent.com/u/15721521?s=128&u=7bb7dde5c4991335fb234e68a30971944abc6bf3&v=4)](https://ipinfo.io/)
