module github.com/ipinfo/mmdbctl

go 1.20

replace github.com/maxmind/mmdbwriter => github.com/max-ipinfo/mmdbwriter v0.0.0-20230925223449-530497b68040

require (
	github.com/fatih/color v1.13.0
	github.com/ipinfo/cli v0.0.0-20220206225602-2df62cb10614
	github.com/maxmind/mmdbwriter v0.0.0-20220808142708-766ad8188582
	github.com/oschwald/maxminddb-golang v1.12.0
	github.com/spf13/pflag v1.0.5
)

require (
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/mattn/go-colorable v0.1.9 // indirect
	github.com/mattn/go-isatty v0.0.14 // indirect
	github.com/posener/script v1.1.5 // indirect
	go4.org/netipx v0.0.0-20220812043211-3cc044ffd68d // indirect
	golang.org/x/sys v0.10.0 // indirect
)
