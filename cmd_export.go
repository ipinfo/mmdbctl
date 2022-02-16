package main

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/ipinfo/cli/lib/complete"
	"github.com/ipinfo/cli/lib/complete/predict"
	"github.com/oschwald/maxminddb-golang"
	"github.com/spf13/pflag"
)

var predictFormats = []string{"csv", "tsv", "json"}

var completionsExport = &complete.Command{
	Flags: map[string]complete.Predictor{
		"-h":          predict.Nothing,
		"--help":      predict.Nothing,
		"-o":          predict.Nothing,
		"--out":       predict.Nothing,
		"-f":          predict.Set(predictFormats),
		"--format":    predict.Set(predictFormats),
		"--no-header": predict.Nothing,
	},
}

func printHelpExport() {
	fmt.Printf(
		`Usage: %s export [<opts>] <mmdb_file> [<out_file>]

Options:
  General:
    --help, -h
      show help.

  Input/Output:
    -o <fname>, --out <fname>
      output file name. (e.g. out.csv)
      default: <out_file> if specified, otherwise stdout.

  Format:
    -f <format>, --format <format>
      the output file format.
      can be "csv", "tsv" or "json".
      default: csv if output file ends in ".csv", tsv if ".tsv",
      json if ".json", otherwise csv.
    --no-header
      don't output the header for file formats that include one, like
      CSV/TSV/JSON.
      default: false.
`, progBase)
}

func cmdExport() error {
	var fOut string
	var fFormat string
	var fNoHdr bool

	_h := "see description in --help"
	pflag.StringVarP(&fOut, "out", "o", "", _h)
	pflag.StringVarP(&fFormat, "format", "f", "", _h)
	pflag.BoolVar(&fNoHdr, "no-header", false, _h)
	pflag.Parse()

	// help?
	if fHelp || (pflag.NArg() == 1 && pflag.NFlag() == 0) {
		printHelpExport()
		return nil
	}

	// get args excluding subcommand.
	args := pflag.Args()[1:]

	// validate input file.
	if len(args) == 0 {
		return errors.New("input mmdb file required as first argument")
	}

	// prepare output file.
	var outFile *os.File
	if fOut == "" && len(args) < 2 {
		outFile = os.Stdout
	} else {
		// either flag or argument is defined.
		if fOut == "" {
			fOut = args[1]
		}

		var err error
		outFile, err = os.Create(fOut)
		if err != nil {
			return fmt.Errorf("could not create %v: %w", fOut, err)
		}
		defer outFile.Close()
	}

	// validate format.
	if fFormat == "" {
		if strings.HasSuffix(fOut, ".csv") {
			fFormat = "csv"
		} else if strings.HasSuffix(fOut, ".tsv") {
			fFormat = "tsv"
		} else if strings.HasSuffix(fOut, ".json") {
			fFormat = "json"
		} else {
			fFormat = "csv"
		}
	}
	if fFormat != "csv" && fFormat != "tsv" && fFormat != "json" {
		return errors.New("format must be \"csv\" or \"tsv\" or \"json\"")
	}

	// open tree.
	db, err := maxminddb.Open(args[0])
	if err != nil {
		return fmt.Errorf("couldn't open mmdb file: %w", err)
	}
	defer db.Close()

	if fFormat == "tsv" || fFormat == "csv" {
		// export.
		hdrWritten := false
		var wr writer
		if fFormat == "csv" {
			csvwr := csv.NewWriter(outFile)
			wr = csvwr
		} else {
			tsvwr := NewTsvWriter(outFile)
			wr = tsvwr
		}
		record := make(map[string]string)
		networks := db.Networks(maxminddb.SkipAliasedNetworks)
		for networks.Next() {
			subnet, err := networks.Network(&record)
			if err != nil {
				return fmt.Errorf("failed to get record for next subnet: %w", err)
			}

			if !hdrWritten {
				hdrWritten = true

				if !fNoHdr {
					hdr := append([]string{"range"}, sortedMapKeys(record)...)
					if err := wr.Write(hdr); err != nil {
						return fmt.Errorf(
							"failed to write header %v: %w",
							hdr, err,
						)
					}
				}
			}

			line := append(
				[]string{subnet.String()},
				sortedMapValsByKeys(record)...,
			)
			if err := wr.Write(line); err != nil {
				return fmt.Errorf("failed to write line %v: %w", line, err)
			}
		}
		wr.Flush()
		if err := wr.Error(); err != nil {
			return fmt.Errorf("writer had failure: %w", err)
		}
		if err := networks.Err(); err != nil {
			return fmt.Errorf("failed networks traversal: %w", err)
		}
	} else if fFormat == "json" {
		// always print opening brace, even if no items.
		fmt.Fprintf(outFile, "{")

		// the first iter of the loop is special (don't print a comma).
		onFirst := true

		networks := db.Networks(maxminddb.SkipAliasedNetworks)
		for networks.Next() {
			record := make(map[string]string)

			subnet, err := networks.Network(&record)
			if err != nil {
				return fmt.Errorf("failed to get record for next subnet: %w", err)
			}

			// marshal each record one-by-one and print it cleanly.
			data, err := json.MarshalIndent(record, "    ", "    ")
			if err != nil {
				return fmt.Errorf("%w", err)
			}
			if onFirst {
				onFirst = false
				fmt.Fprintf(
					outFile, "\n    \"%s\": %s",
					subnet.String(), data,
				)
			} else {
				fmt.Fprintf(
					outFile, ",\n    \"%s\": %s",
					subnet.String(), data,
				)
			}
		}
		if err := networks.Err(); err != nil {
			return fmt.Errorf("failed networks traversal: %w", err)
		}

		// if we had no items, close the top-level object without any newline.
		if !onFirst {
			fmt.Fprintf(outFile, "\n}")
		} else {
			fmt.Fprintf(outFile, "}")
		}
	}
	return nil
}
