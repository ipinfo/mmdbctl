package main

import (
	"fmt"

	"github.com/fatih/color"
	"github.com/spf13/pflag"
)

func printHelpDefault() {
	fmt.Printf(
		`Usage: %s <cmd> [<opts>] [<args>]

Commands:
  read        read data for IPs in an mmdb file.
  import      import data in non-mmdb format into mmdb.
  export      export data from mmdb format into non-mmdb format.
  diff        see the difference between two mmdb files.
  metadata    print metadata from the mmdb file.
  verify      check that the mmdb file is not corrupted or invalid.
  completion  install or output shell auto-completion script.

Options:
  General:
    --nocolor
      disable colored output.
    --help, -h
      show help.
`, progBase)
}

func cmdDefault() (err error) {
	pflag.BoolVarP(&fHelp, "help", "h", false, "show help.")
	pflag.BoolVar(&fNoColor, "nocolor", false, "disable colored output.")
	pflag.Parse()

	if fNoColor {
		color.NoColor = true
	}

	if fHelp {
		printHelpDefault()
		return nil
	}

	// currently we do nothing by default.
	printHelpDefault()
	return nil
}
