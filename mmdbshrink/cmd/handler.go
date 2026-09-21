package cmd

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
)

// cliHandler renders slog records and prints them to stdout and stderr.
//
// We use this handler for the logger passed to the commands implementations in
// the lib package, this way we make it possible to use the lib implementation as
// a library without printing to stdout or stderr.
//
// If the consumer of the library needs to print to stdout or stderr, like the
// cmd package that is the CLI implementation, they can implement a custom handler
// that builds the logs as they prefer.
type cliHandler struct {
	verbose bool
	out     io.Writer
	err     io.Writer
}

// Enabled returns whether a record with this level needs to be rendered
func (h cliHandler) Enabled(_ context.Context, level slog.Level) bool {
	if level < slog.LevelInfo {
		return h.verbose
	}
	return true
}

// Handle renders a single slog record and prints it to stdout or stderr
// depending on the level
func (h cliHandler) Handle(_ context.Context, r slog.Record) error {
	var err error

	switch {
	case r.Level < slog.LevelInfo:
		_, err = fmt.Fprintf(h.err, "[%s] %s\n", r.Time.Format("15:04:05"), r.Message)
	case r.Level >= slog.LevelWarn:
		_, err = fmt.Fprintf(h.err, "%s\n", r.Message)
	default:
		_, err = fmt.Fprintf(h.out, "%s\n", r.Message)
	}

	return err
}

// WithAttrs is a noop for this handler
func (h cliHandler) WithAttrs([]slog.Attr) slog.Handler { return h }

// WithGroup is a noop for this handler
func (h cliHandler) WithGroup(string) slog.Handler { return h }

// newLogger returns a new configure logger that prints
// to stdout and stderr
func newLogger(verbose bool) *slog.Logger {
	return slog.New(cliHandler{
		verbose: verbose,
		out:     os.Stdout,
		err:     os.Stderr,
	})
}
