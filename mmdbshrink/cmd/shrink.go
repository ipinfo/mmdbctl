package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/ipinfo/cli/lib/complete"
	"github.com/ipinfo/cli/lib/complete/predict"
	"github.com/ipinfo/mmdbctl/mmdbshrink/lib/format"
	"github.com/ipinfo/mmdbctl/mmdbshrink/lib/shrink"
)

var completionsShrinkFlags = map[string]complete.Predictor{
	"-h":            predict.Nothing,
	"--help":        predict.Nothing,
	"-v":            predict.Nothing,
	"--verbose":     predict.Nothing,
	"-o":            predict.Nothing,
	"--overwrite":   predict.Nothing,
	"--dry-run":     predict.Nothing,
	"--full-report": predict.Nothing,
	"--no-compact":  predict.Nothing,
}

func printHelpShrink(prog string) {
	fmt.Printf(
		`Usage: %s [<opts>] <input_mmdb_file> [<output_mmdb_file>]

Description:
  Losslessly shrink an mmdb file by deduplicating identical subtrees of the
  search trie. The output is a standard mmdb file with identical lookup
  semantics that any reader can open as is. The input file is left untouched.

  Without <output_mmdb_file>, the output is written next to the input with a
  .shrunk.mmdb extension: foo.mmdb becomes foo.shrunk.mmdb.

  After shrinking, the output is validated against the input: metadata, IPv4
  class and private/reserved boundaries, and 1,000,000 random addresses per
  address family must all answer identically. If any lookup differs the
  command fails, and the output must not be used.

Options:
  General:
    --help, -h
      show help.
    --verbose, -v
      log phase boundaries, memory snapshots and progress counts to stderr.
      default: false.

  Input/Output:
    --overwrite, -o
      overwrite the output file if it already exists.
      default: false.
    --dry-run
      run the full shrink and report the savings, but discard the output
      instead of writing it. Nothing is validated.
      default: false.

  Report:
    --full-report
      also measure how much memory a reader holds for the input and the
      output, each in a fresh child process.
      default: false.

  Shrinking:
    --no-compact
      skip data-section compaction and copy the data section verbatim.
      debug only.
      default: false.
`, prog)
}

func cmdShrink(prog string, args []string) error {
	var help bool
	var dryRun bool
	var verbose bool
	var disableCompact bool
	var overwrite bool
	var fullReport bool

	fs := newFlagSet("shrink")
	fs.BoolVar(&dryRun, "dry-run", false, "skip writing output; just report the savings")
	fs.BoolVarP(&verbose, "verbose", "v", false, "verbose: log phase boundaries, memory snapshots, periodic finalize counts")
	fs.BoolVar(&disableCompact, "no-compact", false, "skip data-section compaction; copy data section verbatim (debug)")
	fs.BoolVarP(&overwrite, "overwrite", "o", false, "overwrite output file if it already exists, defaults to false")
	fs.BoolVar(&fullReport, "full-report", false, "also measure reader memory of input and output")
	fs.BoolVarP(&help, "help", "h", false, "show help.")
	if err := fs.Parse(args); err != nil {
		return err
	}

	rest := fs.Args()
	if help || len(rest) == 0 {
		printHelpShrink(prog)
		return nil
	}
	if len(rest) > 2 {
		return errors.New("too many arguments: expected an input file and an optional output file")
	}
	if dryRun && fullReport {
		return errors.New("--full-report needs the output file, so it can't be combined with --dry-run")
	}

	inputPath := rest[0]
	var outputPath string
	switch {
	case dryRun:
		outputPath = os.DevNull
		// The check right after this one would always fail if we don't
		// set overwrite to true as os.DevNull always exists.
		overwrite = true
	case len(rest) == 2:
		outputPath = rest[1]
	default:
		outputPath = defaultOutputPath(inputPath)
	}

	if !dryRun && sameFile(inputPath, outputPath) {
		return errors.New("output file is the input file; choose a different output path")
	}
	if _, err := os.Stat(outputPath); err == nil && !overwrite {
		return fmt.Errorf("output file %s exists, use --overwrite to overwrite it", outputPath)
	}

	logger := newLogger(verbose)
	opts := shrink.Options{
		DisableCompact:     disableCompact,
		LogMemorySnapshots: verbose,
	}

	result, err := shrink.Shrink(logger, inputPath, outputPath, opts)
	if err != nil {
		return err
	}
	printSummary(inputPath, outputPath, result)
	if dryRun {
		return nil
	}
	return report(prog, inputPath, outputPath, fullReport)
}

// defaultOutputPath names the output after the input, next to it:
// foo.mmdb becomes foo.shrunk.mmdb, and a name without the .mmdb extension
// gets .shrunk.mmdb appended.
func defaultOutputPath(inputPath string) string {
	return strings.TrimSuffix(inputPath, ".mmdb") + ".shrunk.mmdb"
}

// sameFile reports whether a and b name the same existing file, so shrinking
// can't overwrite the input it is still reading.
func sameFile(a, b string) bool {
	ai, err := os.Stat(a)
	if err != nil {
		return false
	}
	bi, err := os.Stat(b)
	if err != nil {
		return false
	}
	return os.SameFile(ai, bi)
}

func printSummary(inputPath, outputPath string, res shrink.Result) {
	fmt.Println()
	fmt.Printf("input:   %s\n", inputPath)
	fmt.Printf("output:  %s\n", outputPath)
	fmt.Println()

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "  size:\t%s\t->\t%s\t%s\n",
		format.DecimalBytes(res.InputBytes),
		format.DecimalBytes(res.OutputBytes),
		format.SizeDelta(int64(res.InputBytes), int64(res.OutputBytes)),
	)
	nodesDiff := int64(res.OutputNodeCount) - int64(res.InputNodeCount)
	fmt.Fprintf(w, "  nodes:\t%s\t->\t%s\t%s\n",
		format.Int(uint64(res.InputNodeCount)),
		format.Int(uint64(res.OutputNodeCount)),
		format.PctDelta(nodesDiff, int64(res.InputNodeCount)),
	)
	fmt.Fprintf(w, "  tree:\t%s\t->\t%s\t\n",
		format.DecimalBytes(res.InputTreeBytes),
		format.DecimalBytes(res.OutputTreeBytes),
	)
	bytesDiff := int64(res.OutputDataBytes) - int64(res.InputDataBytes)
	fmt.Fprintf(w, "  data:\t%s\t->\t%s\t%s\n",
		format.DecimalBytes(res.InputDataBytes),
		format.DecimalBytes(res.OutputDataBytes),
		format.PctDelta(bytesDiff, int64(res.InputDataBytes)),
	)
	w.Flush()

	fmt.Println()
	fmt.Printf("elapsed: %s\n", res.Elapsed.Round(time.Millisecond))
}
