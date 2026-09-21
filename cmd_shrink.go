package main

import (
	"os"

	shrinkcmd "github.com/ipinfo/mmdbctl/mmdbshrink/cmd"
)

func cmdShrink() error {
	return shrinkcmd.Execute(progBase+" shrink", os.Args[2:])
}
