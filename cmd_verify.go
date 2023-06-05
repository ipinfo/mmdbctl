package main

import (
	"fmt"

	"github.com/ipinfo/cli/lib/complete"
	"github.com/ipinfo/cli/lib/complete/predict"
	"github.com/ipinfo/mmdbctl/lib"
	"github.com/spf13/pflag"
)

var completionsVerify = &complete.Command{
	Flags: map[string]complete.Predictor{
		"-h":     predict.Nothing,
		"--help": predict.Nothing,
	},
}

func printHelpVerify() {
	fmt.Printf(
		`Usage: %s verify [<opts>] <mmdb_file>

Options:
  General:
    --help, -h
      show help.
`, progBase)
}

func cmdVerify() error {
	f := lib.CmdVerifyFlags{}
	f.Init()
	pflag.Parse()

	return lib.CmdVerify(f, pflag.Args()[1:], printHelpVerify)
}
