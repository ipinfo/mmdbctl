// Package cmd is the command-line surface for mmdbshrink: flag parsing, help
// text and subcommand dispatch. The shrinking logic itself lives in
// github.com/ipinfo/mmdbctl/mmdbshrink/lib, which knows nothing about the CLI.
//
// Every entry point takes the program prefix to show in usage strings, so the
// same commands serve both the standalone `mmdbshrink` binary and
// `mmdbctl shrink`.
package cmd

import (
	"errors"
	"io"

	"github.com/ipinfo/mmdbctl/mmdbshrink/lib/verify"
	"github.com/spf13/pflag"
)

// ExitCode maps an error returned by Execute to a process exit status.
//
//	4 validation: metadata mismatch
//	3 validation: mismatch found while enumerating prefixes
//	2 validation: lookup mismatch
//	1 anything else
func ExitCode(err error) int {
	var check *verify.CheckError
	switch {
	case errors.As(err, &check):
		switch check.Check {
		case verify.CheckMetadata:
			return 4
		case verify.CheckEnumerate, verify.CheckReverseEnum,
			verify.CheckExactPrefixes, verify.CheckSchema:
			return 3
		default:
			return 2
		}
	default:
		return 1
	}
}

// newFlagSet returns a flag set for a single (sub)command.
//
// Parse errors are reported through the error it returns rather than printed
// by the flag set itself, so callers surface them like any other command error
// and keep control of their own help output.
func newFlagSet(name string) *pflag.FlagSet {
	fs := pflag.NewFlagSet(name, pflag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}

// Execute runs mmdbshrink: `mmdbshrink <in> [<out>]` shrinks <in> and
// validates the result.
//
// prog is the prefix shown in usage strings, e.g. "mmdbshrink" for the
// standalone binary or "mmdbctl shrink" when nested. args is everything after
// that prefix, so callers strip exactly the tokens they own and no layer needs
// to know how deeply it is nested.
func Execute(prog string, args []string) error {
	var cmd string
	if len(args) > 0 {
		cmd = args[0]
	}

	// Everything after the command name.
	var rest []string
	if len(args) > 1 {
		rest = args[1:]
	}

	switch cmd {
	case memorySampleCmd:
		return cmdMemorySample(rest)
	default:
		return cmdShrink(prog, args)
	}
}
