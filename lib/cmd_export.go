package lib

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	maxminddb "github.com/oschwald/maxminddb-golang/v2"
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

	// validate format.
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
	if f.Format != "csv" && f.Format != "tsv" && f.Format != "json" {
		return errors.New("format must be \"csv\" or \"tsv\" or \"json\"")
	}

	// open tree.
	db, err := maxminddb.Open(args[0])
	if err != nil {
		return fmt.Errorf("couldn't open mmdb file: %w", err)
	}
	defer db.Close()

	if f.Format == "tsv" || f.Format == "csv" {
		// export.
		hdrWritten := false
		var wr writer
		if f.Format == "csv" {
			csvwr := csv.NewWriter(outFile)
			wr = csvwr
		} else {
			tsvwr := NewTsvWriter(outFile)
			wr = tsvwr
		}

		// Cache for decoded and serialized records, keyed by data offset.
		// Many networks can point to the same data record in MMDB files.
		cache := make(map[uintptr]map[string]string)
		var hdrKeys []string

		for result := range db.Networks() {
			if err := result.Err(); err != nil {
				return fmt.Errorf("failed networks traversal: %w", err)
			}

			offset := result.Offset()
			prefix := result.Prefix()

			var recordStr map[string]string
			if cached, ok := cache[offset]; ok {
				recordStr = cached
			} else {
				// Cache miss: decode and serialize.
				record := make(map[string]any)
				if err := result.Decode(&record); err != nil {
					return fmt.Errorf("failed to decode record: %w", err)
				}
				recordStr = mapInterfaceToStr(record)
				cache[offset] = recordStr
			}

			if !hdrWritten {
				hdrWritten = true
				hdrKeys = sortedMapKeys(recordStr)
				if !f.NoHdr {
					hdr := append([]string{"range"}, hdrKeys...)
					if err := wr.Write(hdr); err != nil {
						return fmt.Errorf(
							"failed to write header %v: %w",
							hdr, err,
						)
					}
				}
			}

			// Build values in header key order, using empty string for missing keys.
			vals := make([]string, len(hdrKeys))
			for i, k := range hdrKeys {
				vals[i] = recordStr[k] // Returns "" if key doesn't exist
			}

			line := append([]string{prefix.String()}, vals...)
			if err := wr.Write(line); err != nil {
				return fmt.Errorf("failed to write line %v: %w", line, err)
			}
		}
		wr.Flush()
		if err := wr.Error(); err != nil {
			return fmt.Errorf("writer had failure: %w", err)
		}
	} else if f.Format == "json" {
		// Cache for JSON-encoded records (without "range" field), keyed by data offset.
		cache := make(map[uintptr][]byte)
		bw := bufio.NewWriter(outFile)

		for result := range db.Networks() {
			if err := result.Err(); err != nil {
				return fmt.Errorf("failed networks traversal: %w", err)
			}

			offset := result.Offset()
			prefix := result.Prefix()

			var jsonSuffix []byte
			if cached, ok := cache[offset]; ok {
				jsonSuffix = cached
			} else {
				// Cache miss: decode and encode to JSON.
				record := make(map[string]any)
				if err := result.Decode(&record); err != nil {
					return fmt.Errorf("failed to decode record: %w", err)
				}

				encoded, err := json.Marshal(record)
				if err != nil {
					return fmt.Errorf("failed to encode record: %w", err)
				}
				// Cache everything after the opening '{'.
				jsonSuffix = encoded[1:]
				cache[offset] = jsonSuffix
			}

			// Write: {"range":"<prefix>",...}\n
			// jsonSuffix is either "}" (empty record) or `"key":val,...}`
			bw.WriteString(`{"range":"`)
			bw.WriteString(prefix.String())
			bw.WriteString(`"`)
			if len(jsonSuffix) > 1 { // More than just "}"
				bw.WriteByte(',')
			}
			bw.Write(jsonSuffix)
			bw.WriteByte('\n')
		}
		bw.Flush()
	}
	return nil
}
