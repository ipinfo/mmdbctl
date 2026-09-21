package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ipinfo/mmdbctl/mmdbshrink/cmd"
)

var progBase = filepath.Base(os.Args[0])

func main() {
	var err error

	handleCompletions()

	args := os.Args[1:]

	// Shell-completion installation is a property of this binary rather than
	// of the mmdbshrink commands themselves, so it is handled here instead of in the
	// shared cmd package. mmdbctl has its own `completion` command.
	if len(args) > 0 && args[0] == "completion" {
		err = cmdCompletion(args[1:])
	} else {
		err = cmd.Execute(progBase, args)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "err: %v\n", err)
		os.Exit(cmd.ExitCode(err))
	}
}
