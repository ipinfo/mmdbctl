# [<img src="https://ipinfo.io/static/ipinfo-small.svg" alt="IPinfo" width="24"/>](https://ipinfo.io/) IPinfo `mmdbctl`

`mmdbctl` is an MMDB file management CLI  by [IPinfo.io](https://ipinfo.io) that provides you
the following features:

- Read data for IPs in an MMDB file.
- Import data in non-MMDB format into MMDB.
- Export data from MMDB format into non-MMDB format.
- See the difference between two MMDB files.
- Print the metadata of an MMDB file.
- Check that an MMDB file is not corrupted or invalid.

## Installation

The `mmdbctl` CLI is available for download via multiple mechanisms.

### macOS

Install the latest `amd64` version:

```bash
curl -Ls https://github.com/ipinfo/mmdbctl/releases/download/mmdbctl-1.1.0/macos.sh | sh
```

### Debian / Ubuntu (amd64)

```bash
curl -Ls https://github.com/ipinfo/mmdbctl/releases/download/mmdbctl-1.1.0/deb.sh | sh
```

OR

```bash
curl -LO https://github.com/ipinfo/mmdbctl/releases/download/mmdbctl-1.1.0/mmdbctl_1.1.0.deb
sudo dpkg -i mmdbctl_1.1.0.deb
```

### Windows Powershell

*Note*: run powershell as administrator before executing this command.

```bash
iwr -useb https://github.com/ipinfo/mmdbctl/releases/download/mmdbctl-1.1.0/windows.ps1 | iex
```

### Using `go install`

Make sure that `$GOPATH/bin` is in your `$PATH`, because that's where this gets
installed:

```bash
go install github.com/ipinfo/mmdbctl@latest
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
```

After choosing a platform `PLAT` from above, run:

```bash
# for Windows, use ".zip" instead of ".tar.gz"
curl -LO https://github.com/ipinfo/mmdbctl/releases/download/mmdbctl-1.1.0/mmdbctl_1.1.0_${PLAT}.tar.gz
# OR
wget https://github.com/ipinfo/mmdbctl/releases/download/mmdbctl-1.1.0/mmdbctl_1.1.0_${PLAT}.tar.gz
tar -xvf mmdbctl_1.1.0_${PLAT}.tar.gz
mv mmdbctl_1.1.0_${PLAT} /usr/local/bin/mmdbctl
```

### Using `git`

Installing from source requires at least the Golang version specified in
`go.mod`. You can install the Golang toolchain from
[the official site](https://golang.org/doc/install).

Once the correct Golang version is installed, simply clone the repository and
install the binary:

```bash
git clone https://github.com/ipinfo/mmdbctl
cd mmdbctl
go install .
$GOPATH/bin/mmdbctl
```

You can add `$GOPATH/bin` to your `$PATH` to access `mmdbctl` directly from
anywhere.

Alternatively, you can do the following to output the binary somewhere
specific:

```bash
git clone https://github.com/ipinfo/mmdbctl
cd mmdbctl
go build -o <path> .
```

Replace `<path>` with the required location.

## Quick Start

This will help you quickly get started with the `mmdbctl` CLI.

### Default Help Message

By default, invoking the CLI shows a help message:

![mmdbctl](images/help.png)

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
[`NO_COLOR`](https://no-color.org/)  environment variable to disable color
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

Founded in 2013, IPinfo prides itself on being the most reliable, accurate, and in-depth source of IP address data available anywhere. We process terabytes of data to produce our custom IP geolocation, company, carrier, VPN detection, hosted domains, and IP type data sets. Our API handles over 40 billion requests a month for 100,000 businesses and developers.

[![image](https://avatars3.githubusercontent.com/u/15721521?s=128&u=7bb7dde5c4991335fb234e68a30971944abc6bf3&v=4)](https://ipinfo.io/)
