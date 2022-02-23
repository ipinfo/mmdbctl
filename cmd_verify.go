package main

import (
	"errors"
	"fmt"

	"github.com/ipinfo/cli/lib/complete"
	"github.com/ipinfo/cli/lib/complete/predict"
	"github.com/oschwald/maxminddb-golang"
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
	pflag.Parse()

	// help?
	if fHelp || (pflag.NArg() == 1 && pflag.NFlag() == 0) {
		printHelpVerify()
		return nil
	}

	// get args excluding subcommand.
	args := pflag.Args()[1:]

	// validate input file.
	if len(args) == 0 {
		return errors.New("input mmdb file required as first argument")
	}

	// open tree.
	db, err := maxminddb.Open(args[0])
	if err != nil {
		return fmt.Errorf("couldn't open mmdb file: %w", err)
	}
	defer db.Close()

	// verify.
	err = db.Verify()
	if err != nil {
		fmt.Printf("invalid: %w\n", err)
	} else {
		fmt.Println("valid")
	}

	return nil
}
