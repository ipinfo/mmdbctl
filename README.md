# [<img src="https://ipinfo.io/static/ipinfo-small.svg" alt="IPinfo" width="24"/>](https://ipinfo.io/) IPinfo `mmdbctl`

`mmdbctl` is an MMDB file management CLI by [IPinfo.io](https://ipinfo.io) that provides you
the following features:

- Read data for IPs in an MMDB file.
- Import data in non-MMDB format into MMDB.
- Export data from MMDB format into non-MMDB format.
- See the difference between two MMDB files.
- Print the metadata of an MMDB file.
- Check that an MMDB file is not corrupted or invalid.
- Losslessly shrink an MMDB file.

## Installation

The `mmdbctl` CLI is available for download via multiple mechanisms.

Releases also ship `mmdbshrink`, a standalone binary that does what
`mmdbctl shrink` does (see [Shrinking](#shrinking)). The
install scripts and the Debian package below install both binaries.

### macOS

Install the latest version. The script detects your Mac's architecture and
installs the matching binaries: `arm64` on Apple Silicon, including when run
under Rosetta, and `amd64` on Intel:

```bash
curl -Ls https://github.com/ipinfo/mmdbctl/releases/download/mmdbctl-1.4.10/macos.sh | sh
```

### Debian / Ubuntu (amd64)

```bash
curl -Ls https://github.com/ipinfo/mmdbctl/releases/download/mmdbctl-1.4.10/deb.sh | sh
```

OR

```bash
curl -LO https://github.com/ipinfo/mmdbctl/releases/download/mmdbctl-1.4.10/mmdbctl_1.4.10.deb
sudo dpkg -i mmdbctl_1.4.10.deb
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
go install github.com/ipinfo/mmdbctl@latest

# optionally, the standalone mmdbshrink binary
go install github.com/ipinfo/mmdbctl/mmdbshrink@latest
```

### Using `curl`/`wget`

The pre-built binaries for all platforms are available on GitHub via artifacts
in releases. You need to simply download, unpack and move them to your shell's
binary search path.

The following OS & arch combinations are supported (if you use one not listed
on here, please open an issue):

```
darwin_amd64
darwin_arm64
dragonfly_amd64
freebsd_386
freebsd_amd64
freebsd_arm
freebsd_arm64
linux_386
linux_amd64
linux_arm
linux_arm64
netbsd_386
netbsd_amd64
netbsd_arm
netbsd_arm64
openbsd_386
openbsd_amd64
openbsd_arm
openbsd_arm64
solaris_amd64
windows_386
windows_amd64
windows_arm
windows_arm64
```

After choosing a platform `PLAT` from above, run:

```bash
# for Windows, use ".zip" instead of ".tar.gz"
curl -LO https://github.com/ipinfo/mmdbctl/releases/download/mmdbctl-1.4.10/mmdbctl_1.4.10_${PLAT}.tar.gz
# OR
wget https://github.com/ipinfo/mmdbctl/releases/download/mmdbctl-1.4.10/mmdbctl_1.4.10_${PLAT}.tar.gz
tar -xvf mmdbctl_1.4.10_${PLAT}.tar.gz
mv mmdbctl_1.4.10_${PLAT} /usr/local/bin/mmdbctl
```

The standalone `mmdbshrink` binary is published in the same release, for the
same platforms:

```bash
# for Windows, use ".zip" instead of ".tar.gz"
curl -LO https://github.com/ipinfo/mmdbctl/releases/download/mmdbctl-1.4.10/mmdbshrink_1.4.10_${PLAT}.tar.gz
tar -xvf mmdbshrink_1.4.10_${PLAT}.tar.gz
mv mmdbshrink_1.4.10_${PLAT} /usr/local/bin/mmdbshrink
```

### Using `git`

Installing from source requires at least the Golang version specified in
`go.mod`. You can install the Golang toolchain from
[the official site](https://golang.org/doc/install).

Once the correct Golang version is installed, simply clone the repository and
install the binaries:

```bash
git clone https://github.com/ipinfo/mmdbctl
cd mmdbctl
go install . ./mmdbshrink
$GOPATH/bin/mmdbctl
$GOPATH/bin/mmdbshrink
```

You can add `$GOPATH/bin` to your `$PATH` to access `mmdbctl` and `mmdbshrink`
directly from anywhere.

Alternatively, you can do the following to output the binaries somewhere
specific:

```bash
git clone https://github.com/ipinfo/mmdbctl
cd mmdbctl
go build -o <dir>/ . ./mmdbshrink
```

Replace `<dir>` with the required directory; both binaries are written into
it. `./scripts/build.sh` does the same into `build/`. Run the tests with
`go test ./...`.

## Quick Start

This will help you quickly get started with the `mmdbctl` CLI.

### Default Help Message

By default, invoking one of the CLIs shows a help message:

```
$ mmdbctl
Usage: mmdbctl <cmd> [<opts>] [<args>]

Commands:
  read        read data for IPs in an mmdb file.
  import      import data in non-mmdb format into mmdb.
  export      export data from mmdb format into non-mmdb format.
  diff        see the difference between two mmdb files.
  metadata    print metadata from the mmdb file.
  verify      check that the mmdb file is not corrupted or invalid.
  shrink      losslessly shrink an mmdb file.
  completion  install or output shell auto-completion script.

Options:
  General:
    --nocolor
      disable colored output.
    --help, -h
      show help.

$ mmdbshrink
Usage: mmdbshrink [<opts>] <input_mmdb_file> [<output_mmdb_file>]

Description:
  Losslessly shrink an mmdb file by deduplicating identical subtrees of the
  search trie. The output is a standard mmdb file with identical lookup
  semantics that any reader can open as is. The input file is left untouched.

  Without <output_mmdb_file>, the output is written next to the input with a
  .shrunk.mmdb extension: foo.mmdb becomes foo.shrunk.mmdb.

  After shrinking, the output is validated against the input: metadata, IPv4
  class and private/reserved boundaries, and 1,000,000 random addresses per
  address family must all answer identically. If any lookup differs the
  command fails, and the output must not be used.

Options:
  General:
    --help, -h
      show help.
    --verbose, -v
      log phase boundaries, memory snapshots and progress counts to stderr.
      default: false.

  Input/Output:
    --overwrite, -o
      overwrite the output file if it already exists.
      default: false.
    --dry-run
      run the full shrink and report the savings, but discard the output
      instead of writing it. Nothing is validated.
      default: false.

  Report:
    --full-report
      also measure how much memory a reader holds for the input and the
      output, each in a fresh child process.
      default: false.

  Shrinking:
    --no-compact
      skip data-section compaction and copy the data section verbatim.
      debug only.
      default: false.
```

### Reading

You can read from MMDB files in various different ways - as individual IPs,
CIDRs or IP ranges, coming from the command line as arguments, or from files,
or from stdin.

Pretty JSON format:

```bash
$ mmdbctl read -f json-pretty 8.8.8.8 location.mmdb
{
  "city": "Mountain View",
  "country": "US",
  "geoname_id": "5375480",
  "latitude": "37.4056",
  "longitude": "-122.0775",
  "postalcode": "94043",
  "region": "California",
  "timezone": "America/Los_Angeles"
}
```

CSV format:

```bash
$ mmdbctl read -f csv 8.8.8.8 location.mmdb
ip,city,country,geoname_id,latitude,longitude,postalcode,region,timezone
8.8.8.8,Mountain View,US,5375480,37.4056,-122.0775,94043,California,America/Los_Angeles
```

TSV format:

```bash
$ mmdbctl read -f tsv 8.8.8.8 location.mmdb
ip	city	country	geoname_id	latitude	longitude	postalcode	region	timezone
8.8.8.8	Mountain View	US	5375480	37.4056	-122.0775	94043	California	America/Los_Angeles
```

Via a file:

```bash
$ cat ips.txt
8.8.8.8
8.8.8.0/31
8.8.8.0-8.8.8.1
8.8.8.0,8.8.8.1

$ mmdbctl read ips.txt location.mmdb | sort -u
{"city":"Mountain View","country":"US","geoname_id":"5375480","latitude":"37.4056","longitude":"-122.0775","postalcode":"94043","region":"California","timezone":"America/Los_Angeles"}
```

Via stdin:

```bash
$ echo 8.8.8.8 | mmdbctl read location.mmdb
{"city":"Mountain View","country":"US","geoname_id":"5375480","latitude":"37.4056","longitude":"-122.0775","postalcode":"94043","region":"California","timezone":"America/Los_Angeles"}
```

Multiple inputs are also possible - these all return the same thing:

```bash
$ echo -e '8.8.8.8\n1.2.3.4' | mmdbctl read location.mmdb
$ mmdbctl read 8.8.8.8 1.2.3.4 location.mmdb
{"city":"Mountain View","country":"US","geoname_id":"5375480","latitude":"37.4056","longitude":"-122.0775","postalcode":"94043","region":"California","timezone":"America/Los_Angeles"}
{"city":"Brisbane","country":"AU","geoname_id":"2174003","latitude":"-27.48203","longitude":"153.01358","postalcode":"4101","region":"Queensland","timezone":"Australia/Brisbane"}
```

Can check CIDRs and ranges - these will all return the same thing:

```bash
$ mmdbctl read 8.8.8.0/31 location.mmdb
$ mmdbctl read 8.8.8.0-8.8.8.1 location.mmdb
$ mmdbctl read 8.8.8.0,8.8.8.1 location.mmdb
{"city":"Mountain View","country":"US","geoname_id":"5375480","latitude":"37.4056","longitude":"-122.0775","postalcode":"94043","region":"California","timezone":"America/Los_Angeles"}
{"city":"Mountain View","country":"US","geoname_id":"5375480","latitude":"37.4056","longitude":"-122.0775","postalcode":"94043","region":"California","timezone":"America/Los_Angeles"}
{"city":"Mountain View","country":"US","geoname_id":"5375480","latitude":"37.4056","longitude":"-122.0775","postalcode":"94043","region":"California","timezone":"America/Los_Angeles"}
```

### Importing

Importing allows taking in files as CSV/TSV/JSON, and outputting an MMDB file.

Importing is one of the most powerful/flexible features in `mmdbctl`. However,
we only allow strings throughout the data at the current time.

See `mmdbctl import --help` for full details on usage.

Here are some basic examples:

```bash
# basic CSV importing into MMDB.
$ mmdbctl import --in data.csv --out data.mmdb

# generate MMDB from a TSV file containing IPv4 data.
$ cat data.tsv | mmdbctl import --ip 4 --tsv --out data.mmdb

# don't include the implicit `network` field in the output MMDB:
$ mmdbctl import --no-network --in data.csv --out data.mmdb

# generate an MMDB without any fields, just IP ranges that meet a criteria.
$ mmdbctl import                                                              \
    --size 24 --no-fields --ip 4                                              \
    --in anycast.csv --out anycast.mmdb
```

### Exporting

Exporting allows taking in an MMDB file and outputting CSV/TSV/JSON.

See `mmdbctl export --help` for full details on usage.

```bash
# basic export.
$ mmdbctl export data.mmdb data.csv

# basic export without a header.
$ mmdbctl export --no-header data.mmdb data.csv

# just see the number of entries it'd output.
$ mmdbctl export --no-header --format csv data.mmdb | wc -l
```

### Metadata

You can retrieve data in the `metadata` section of the MMDB file using the
`metadata` subcommand.

Pretty format:

```bash
$ mmdbctl metadata location.mmdb
- Binary Format 2.0
- Database Type ipinfo location.mmdb
- IP Version    4
- Record Size   32
- Node Count    123456789
- Description
    en ipinfo location.mmdb
- Languages     en
- Build Epoch   123456789
```

JSON format:

```bash
$ mmdbctl metadata -f json location.mmdb
{
    "binary_format": "2.0",
    "db_type": "ipinfo location.mmdb",
    "ip": 4,
    "record_size": 32,
    "node_count": 123456789,
    "description": {
        "en": "ipinfo location.mmdb"
    },
    "languages": [
        "en"
    ],
    "build_epoch": 123456789
}
```

### Verification

You can verify if a MMDB file is correctly structured with the `verify`
subcommand:

```bash
$ mmdbctl verify location.mmdb
valid
```

Let's force it to be invalid and check again:

```bash
$ cp location.mmdb location-tmp.mmdb
$ cat location.mmdb >> location-tmp.mmdb
$ mmdbctl verify location-tmp.mmdb
invalid: received decoding error (the MaxMind DB file's data section contains bad data (uint16 size of 11)) at offset of 13825601
```

### Shrinking

`mmdbctl shrink` losslessly shrinks MMDB files by deduplicating identical
subtrees of the search tree. The output is a standard MMDB file that returns
identical lookup results and that any MMDB reader can open as is, including
`mmdbctl read`: there is nothing to decompress before reading it.

The standalone `mmdbshrink` binary does the same: `mmdbshrink <in>` is
`mmdbctl shrink <in>`. The one difference is exit codes: `mmdbshrink` exits
non-zero when validation fails, so it can gate a CI pipeline, while
`mmdbctl shrink` prints the error and exits 0 like every other `mmdbctl`
command. See [`mmdbshrink`](mmdbshrink/README.md) for full details.

Shrink a file. Without an output path, `foo.mmdb` is shrunk to
`foo.shrunk.mmdb`; the input is left untouched. After writing, the shrunk
file is validated against the input: the metadata, the IPv4 class and
private/reserved boundaries, and 1,000,000 random addresses per address
family must all answer identically:

```bash
$ mmdbctl shrink ipinfo_core.mmdb

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

Pass an output path to choose another name, and `--overwrite` to replace a
file that already exists. `--full-report` also measures how much memory a
reader holds for the input and the shrunk file, each in a fresh process:

```bash
$ mmdbctl shrink --full-report ipinfo_core.mmdb
...
memory (200,000 lookups each, separate processes):
  input:    mmap_cached=910.41 MiB  proc_rss=839.86 MiB  per_op=8600ns  minflt=247  majflt=28,250
  output:   mmap_cached=461.92 MiB  proc_rss=463.16 MiB  per_op=4666ns  minflt=238  majflt=10,227
```

To see how much a file would shrink without writing anything, use
`--dry-run`. It runs the full shrink and discards the output, so the numbers
are exact. Nothing is written, so nothing is validated.

The result is a regular MMDB file:

```bash
$ mmdbctl read 8.8.8.8 ipinfo_core.shrunk.mmdb
{"as_domain":"google.com","as_name":"Google LLC","as_type":"hosting","asn":"AS15169","city":"Mountain View","continent":"North America","continent_code":"NA","country":"United States","country_code":"US","ip":"8.8.8.8","is_anonymous":false,"is_anycast":true,"is_hosting":true,"is_mobile":false,"is_satellite":false,"latitude":38.00881,"longitude":-122.11746,"postal_code":"94043","region":"California","region_code":"CA","timezone":"America/Los_Angeles"}
```

## Go Library

The shrinking tooling is also available as Go packages under
`github.com/ipinfo/mmdbctl/mmdbshrink/lib`. `shrink` and `verify` are the code
the CLIs run, and `verify` and `bench` also offer checks the CLIs don't run on
every shrink: a walk of every prefix in both files, the strongest and slowest
equivalence check, and a noise-robust benchmark that compares lookup speed
over many interleaved rounds.

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

Auto-completion is supported for at least the following shells:

```
bash
zsh
fish
```

NOTE: it may work for other shells as well because the implementation is in
Golang and is not necessarily shell-specific.

### Installation

Installing auto-completions is as simple as running one command (works for
`bash`, `zsh` and `fish` shells):

```bash
mmdbctl completion install
```

The standalone `mmdbshrink` binary has the same command:

```bash
mmdbshrink completion install
```

If you want to customize the installation process (e.g. in case the
auto-installation doesn't work as expected), you can request the actual
completion script for each shell:

```bash
# get bash completion script
mmdbctl completion bash

# get zsh completion script
mmdbctl completion zsh

# get fish completion script
mmdbctl completion fish
```

### Shell not listed?

If your shell is not listed here, you can open an issue.

Note that as long as the `COMP_LINE` environment variable is provided to the
binary itself, it will output completion results. So if your shell provides a
way to pass `COMP_LINE` on auto-completion attempts to a binary, then have your
shell do that with the `mmdbctl` binary itself (or any of our binaries).

## Color Output

### Disabling Color Output

All our CLIs respect either the `--nocolor` flag or the
[`NO_COLOR`](https://no-color.org/) environment variable to disable color
output.

### Color on Windows

To enable color support for the Windows command prompt, run the following to
enable [`Console Virtual Terminal Sequences`](https://docs.microsoft.com/en-us/windows/console/console-virtual-terminal-sequences).

```cmd
REG ADD HKCU\CONSOLE /f /v VirtualTerminalLevel /t REG_DWORD /d 1
```

You can disable this by running the following:

```cmd
REG DELETE HKCU\CONSOLE /f /v VirtualTerminalLevel
```

## Other IPinfo Tools

There are official IPinfo client libraries available for many languages including PHP, Python, Go, Java, Ruby, and many popular frameworks such as Django, Rails and Laravel. There are also many third party libraries and integrations available for our API.

See [https://ipinfo.io/developers/libraries](https://ipinfo.io/developers/libraries) for more details.

## About IPinfo

Founded in 2013, IPinfo prides itself on being the most reliable, accurate, and in-depth source of IP address data available anywhere. We process terabytes of data to produce our custom IP geolocation, company, carrier, VPN detection, hosted domains, and IP type data sets. Our API handles over 40 billion requests a month for businesses and developers.

[![image](https://avatars3.githubusercontent.com/u/15721521?s=128&u=7bb7dde5c4991335fb234e68a30971944abc6bf3&v=4)](https://ipinfo.io/)
