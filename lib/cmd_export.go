package lib

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/oschwald/maxminddb-golang/v2"
	"github.com/spf13/pflag"
)

// CmdExportFlags are flags expected by CmdExport.
type CmdExportFlags struct {
	Help   bool
	NoHdr  bool
	Format string
	Out    string
}

// Init initializes the common flags available to CmdExport with sensible
// defaults.
//
// pflag.Parse() must be called to actually use the final flag values.
func (f *CmdExportFlags) Init() {
	_h := "see description in --help"
	pflag.BoolVarP(
		&f.Help,
		"help", "h", false,
		"show help.",
	)
	pflag.BoolVar(
		&f.NoHdr,
		"no-header", false,
		_h,
	)
	pflag.StringVarP(
		&f.Format,
		"format", "f", "",
		_h,
	)
	pflag.StringVarP(
		&f.Out,
		"out", "o", "",
		_h,
	)
}

func CmdExport(f CmdExportFlags, args []string, printHelp func()) error {
	// help?
	if f.Help || (pflag.NArg() == 1 && pflag.NFlag() == 0) {
		printHelp()
		return nil
	}

	// validate input file.
	if len(args) == 0 {
		return errors.New("input mmdb file required as first argument")
	}

	// prepare output file.
	var outFile *os.File
	if f.Out == "" && len(args) < 2 {
		outFile = os.Stdout
	} else {
		// either flag or argument is defined.
		if f.Out == "" {
			f.Out = args[1]
		}

		var err error
		outFile, err = os.Create(f.Out)
		if err != nil {
			return fmt.Errorf("could not create %v: %w", f.Out, err)
		}
		defer outFile.Close()
	}

	// infer format from extension if not specified.
	if f.Format == "" {
		if strings.HasSuffix(f.Out, ".csv") {
			f.Format = "csv"
		} else if strings.HasSuffix(f.Out, ".tsv") {
			f.Format = "tsv"
		} else if strings.HasSuffix(f.Out, ".json") {
			f.Format = "json"
		} else {
			f.Format = "csv"
		}
	}

	// open tree.
	db, err := maxminddb.Open(args[0])
	if err != nil {
		return fmt.Errorf("couldn't open mmdb file: %w", err)
	}
	defer db.Close()

	var exp exporter
	switch f.Format {
	case "csv":
		exp = newCSVExporter(outFile, f.NoHdr)
	case "tsv":
		exp = newTSVExporter(outFile, f.NoHdr)
	case "json":
		exp = newJSONExporter(outFile)
	default:
		return errors.New("format must be \"csv\" or \"tsv\" or \"json\"")
	}

	return exportNetworks(db, exp)
}
