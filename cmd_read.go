package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"

	"github.com/fatih/color"
	"github.com/ipinfo/cli/lib"
	"github.com/ipinfo/cli/lib/complete"
	"github.com/ipinfo/cli/lib/complete/predict"
	"github.com/oschwald/maxminddb-golang"
	"github.com/spf13/pflag"
)

var predictReadFmts = []string{
	"json",
	"json-compact",
	"json-pretty",
	"tsv",
	"csv",
}

var completionsRead = &complete.Command{
	Flags: map[string]complete.Predictor{
		"--nocolor": predict.Nothing,
		"-h":        predict.Nothing,
		"--help":    predict.Nothing,
		"-f":        predict.Set(predictReadFmts),
		"--format":  predict.Set(predictReadFmts),
	},
}

func printHelpRead() {
	fmt.Printf(
		`Usage: %s read [<opts>] <ip | ip-range | cidr | filepath> <mmdb>

Options:
  General:
    --nocolor
      disable colored output.
    --help, -h
      show help.

  Format:
    -f <format>, --format <format>
      the output format.
      can be "json", "json-compact", "json-pretty", "tsv" or "csv".
      note that "json" is short for "json-compact".
      default: json.
`, progBase)
}

func cmdRead() error {
	var fFormat string

	_h := "see description in --help"
	pflag.StringVarP(&fFormat, "format", "f", "json", _h)
	pflag.Parse()

	if fNoColor {
		color.NoColor = true
	}

	// help?
	if fHelp || pflag.NArg() == 1 {
		printHelpRead()
		return nil
	}

	// get args excluding subcommand.
	args := pflag.Args()[1:]

	// validate format.
	if fFormat == "json" {
		fFormat = "json-compact"
	}
	validFormat := false
	for _, format := range predictReadFmts {
		if fFormat == format {
			validFormat = true
			break
		}
	}
	if !validFormat {
		return fmt.Errorf("format must be one of %v", predictReadFmts)
	}

	// last arg must be mmdb file; open it.
	mmdbFileArg := args[len(args)-1]
	db, err := maxminddb.Open(mmdbFileArg)
	if err != nil {
		return fmt.Errorf("couldn't open mmdb file %v: %w", mmdbFileArg, err)
	}
	defer db.Close()

	// get IP list.
	ips, err := lib.IPListFromAllSrcs(args[:len(args)-1])
	if err != nil {
		return fmt.Errorf("couldn't get IP list: %w", err)
	}

	requiresHdr := fFormat == "csv" || fFormat == "tsv"
	hdrWritten := false
	var wr writer
	if fFormat == "csv" {
		csvwr := csv.NewWriter(os.Stdout)
		wr = csvwr
	} else if fFormat == "tsv" {
		tsvwr := NewTsvWriter(os.Stdout)
		wr = tsvwr
	}
	for _, ip := range ips {
		record := make(map[string]string)
		if err := db.Lookup(ip, &record); err != nil || len(record) == 0 {
			if !requiresHdr {
				fmt.Fprintf(os.Stderr,
					"err: couldn't get data for %s\n",
					ip.String(),
				)
			}
			continue
		}

		if !hdrWritten {
			hdrWritten = true

			if requiresHdr {
				hdr := append([]string{"ip"}, sortedMapKeys(record)...)
				if err := wr.Write(hdr); err != nil {
					return fmt.Errorf(
						"failed to write header %v: %w",
						hdr, err,
					)
				}
			}
		}

		if fFormat == "json-compact" {
			b, err := json.Marshal(record)
			if err != nil {
				fmt.Fprintf(os.Stderr,
					"err: couldn't print data for %s\n",
					ip.String(),
				)
				continue
			}
			fmt.Printf("%s\n", b)
		} else if fFormat == "json-pretty" {
			b, err := json.MarshalIndent(record, "", "  ")
			if err != nil {
				fmt.Fprintf(os.Stderr,
					"err: couldn't print data for %s\n",
					ip.String(),
				)
				continue
			}
			fmt.Printf("%s\n", b)
		} else { // if fFormat == "csv" || fFormat == "tsv"
			line := append([]string{ip.String()}, sortedMapValsByKeys(record)...)
			if err := wr.Write(line); err != nil {
				return fmt.Errorf("failed to write line %v: %w", line, err)
			}
		}
	}
	if wr != nil {
		wr.Flush()
		if err := wr.Error(); err != nil {
			return fmt.Errorf("writer had failure: %w", err)
		}
	}

	return nil
}
