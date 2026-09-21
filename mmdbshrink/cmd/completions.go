package cmd

import (
	"maps"

	"github.com/ipinfo/cli/lib/complete"
)

// Completions returns the shell-completion tree for mmdbshrink: the shrink
// flags.
//
// It is ready to mount as a subcommand of a larger CLI (e.g. under "shrink"
// in mmdbctl) or to serve as a root. A fresh tree is built per call, so
// callers may add entries to Sub without affecting anyone else.
func Completions() *complete.Command {
	return &complete.Command{
		Sub:   map[string]*complete.Command{},
		Flags: maps.Clone(completionsShrinkFlags),
	}
}
