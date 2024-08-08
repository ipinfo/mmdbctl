# 1.4.6

- [#39](https://github.com/ipinfo/mmdbctl/pull/39) Switch to "github.com/edsrzf/mmap-go" for cross-platform mmap implementation (to make Windows builds work again)
- [#37](https://github.com/ipinfo/mmdbctl/pull/37) `import`: JSON input processing supports `--fields` and `--fields-from-header` flags
- [#30](https://github.com/ipinfo/mmdbctl/pull/30) added type sizes info found within the data section)
- [#28](https://github.com/ipinfo/mmdbctl/pull/28) added low-level mmdb data to the metadata output
- [#26](https://github.com/ipinfo/mmdbctl/pull/26) Fix: Compatibility Issue with IP2Location DB using int for IP Range

# 1.4.4

- Revert back to `maxmind/mmdbwriter`

# 1.4.3

- Temporarily use `max-info/mmdbwriter`
