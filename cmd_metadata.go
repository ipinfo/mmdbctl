package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/fatih/color"
	"github.com/ipinfo/cli/lib/complete"
	"github.com/ipinfo/cli/lib/complete/predict"
	"github.com/oschwald/maxminddb-golang"
	"github.com/spf13/pflag"
)

var predictMetadataFmts = []string{"pretty", "json"}

var completionsMetadata = &complete.Command{
	Flags: map[string]complete.Predictor{
		"--nocolor": predict.Nothing,
		"-h":        predict.Nothing,
		"--help":    predict.Nothing,
		"-f":        predict.Set(predictMetadataFmts),
		"--format":  predict.Set(predictMetadataFmts),
	},
}

func printHelpMetadata() {
	fmt.Printf(
		`Usage: %s metadata [<opts>] <mmdb_file>

Options:
  General:
    --nocolor
      disable colored output.
    --help, -h
      show help.

  Format:
    -f <format>, --format <format>
      the metadata output format.
      can be "pretty" or "json".
      default: pretty.
`, progBase)
}

func cmdMetadata() error {
	var fFormat string

	_h := "see description in --help"
	pflag.StringVarP(&fFormat, "format", "f", "", _h)
	pflag.Parse()

	if fNoColor {
		color.NoColor = true
	}

	// help?
	if fHelp || (pflag.NArg() == 1 && pflag.NFlag() == 0) {
		printHelpMetadata()
		return nil
	}

	// get args excluding subcommand.
	args := pflag.Args()[1:]

	// validate input file.
	if len(args) == 0 {
		return errors.New("input mmdb file required as first argument")
	}

	// validate format
	if fFormat == "" {
		fFormat = "pretty"
	}
	if fFormat != "pretty" && fFormat != "json" {
		return errors.New("format must be one of \"pretty\" or \"json\"")
	}

	// open tree.
	db, err := maxminddb.Open(args[0])
	if err != nil {
		return fmt.Errorf("couldn't open mmdb file: %w", err)
	}
	defer db.Close()

	mdFromLib := db.Metadata
	binaryFmt := strconv.Itoa(int(mdFromLib.BinaryFormatMajorVersion)) + "." + strconv.Itoa(int(mdFromLib.BinaryFormatMinorVersion))
	if fFormat == "pretty" {
		fmtEntry := color.New(color.FgCyan)
		fmtVal := color.New(color.FgGreen)
		printlineGen := func(entryLen string) func(string, string) {
			return func(name string, val string) {
				fmt.Printf(
					"- %v %v\n",
					fmtEntry.Sprintf("%-"+entryLen+"s", name),
					fmtVal.Sprintf("%v", val),
				)
			}
		}
		printline := printlineGen("13")
		printline("Binary Format", binaryFmt)
		printline("Database Type", mdFromLib.DatabaseType)
		printline("IP Version", strconv.Itoa(int(mdFromLib.IPVersion)))
		printline("Record Size", strconv.Itoa(int(mdFromLib.RecordSize)))
		printline("Node Count", strconv.Itoa(int(mdFromLib.NodeCount)))
		printline("Description", "")
		descKeys, descVals := sortedMapKeysAndVals(mdFromLib.Description)
		longestDescKeyLen := strconv.Itoa(len(longestStrInStringSlice(descKeys)))
		for i := 0; i < len(descKeys); i++ {
			fmt.Printf(
				"    %v %v\n",
				fmtEntry.Sprintf("%-"+longestDescKeyLen+"s", descKeys[i]),
				fmtVal.Sprintf("%v", descVals[i]),
			)
		}
		printline("Languages", strings.Join(mdFromLib.Languages, ", "))
		printline("Build Epoch", strconv.Itoa(int(mdFromLib.BuildEpoch)))
	} else { // json
		md := struct {
			BinaryFormatVsn string            `json:"binary_format"`
			DatabaseType    string            `json:"db_type"`
			IPVersion       uint              `json:"ip"`
			RecordSize      uint              `json:"record_size"`
			NodeCount       uint              `json:"node_count"`
			Description     map[string]string `json:"description"`
			Languages       []string          `json:"languages"`
			BuildEpoch      uint              `json:"build_epoch"`
		}{
			binaryFmt,
			mdFromLib.DatabaseType,
			mdFromLib.IPVersion,
			mdFromLib.RecordSize,
			mdFromLib.NodeCount,
			mdFromLib.Description,
			mdFromLib.Languages,
			mdFromLib.BuildEpoch,
		}
		out, err := json.MarshalIndent(md, "", "    ")
		if err != nil {
			return fmt.Errorf("couldn't marshal json metadata: %w", err)
		}

		fmt.Println(string(out))
	}

	return nil
}
