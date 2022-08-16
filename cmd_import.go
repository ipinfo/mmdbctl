package main

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/ipinfo/cli/lib/complete"
	"github.com/ipinfo/cli/lib/complete/predict"
	"github.com/maxmind/mmdbwriter"
	"github.com/maxmind/mmdbwriter/inserter"
	"github.com/maxmind/mmdbwriter/mmdbtype"
	"github.com/spf13/pflag"
)

var predictIpVsn = []string{"4", "6"}
var predictSize = []string{"24", "28", "32"}
var predictMerge = []string{"none", "toplevel", "recurse"}

var completionsImport = &complete.Command{
	Flags: map[string]complete.Predictor{
		"-h":                          predict.Nothing,
		"--help":                      predict.Nothing,
		"-i":                          predict.Nothing,
		"--in":                        predict.Nothing,
		"-o":                          predict.Nothing,
		"--out":                       predict.Nothing,
		"-c":                          predict.Nothing,
		"--csv":                       predict.Nothing,
		"-t":                          predict.Nothing,
		"--tsv":                       predict.Nothing,
		"-j":                          predict.Nothing,
		"--json":                      predict.Nothing,
		"-f":                          predict.Nothing,
		"--fields":                    predict.Nothing,
		"--fields-from-header":        predict.Nothing,
		"--range-multicol":            predict.Nothing,
		"--joinkey-col":               predict.Nothing,
		"--no-fields":                 predict.Nothing,
		"--no-network":                predict.Nothing,
		"--ip":                        predict.Set(predictIpVsn),
		"-s":                          predict.Set(predictSize),
		"--size":                      predict.Set(predictSize),
		"-m":                          predict.Set(predictMerge),
		"--merge":                     predict.Set(predictMerge),
		"--ignore-empty-values":       predict.Nothing,
		"--disallow-reserved":         predict.Nothing,
		"--alias-6to4":                predict.Nothing,
		"--disable-metadata-pointers": predict.Nothing,
	},
}

func printHelpImport() {
	fmt.Printf(
		`Usage: %s import [<opts>]

Options:
  General:
    --help, -h
      show help.

  Example:
    # Imports an input file and outputs an mmdb file with default configurations. 
    $ %[1]s import input.csv output.mmdb

  Input/Output:
    -i <fname>, --in <fname>
      input file name. (e.g. data.csv or - for stdin)
      must be in CSV, TSV or JSON.
      default: stdin.
    -o <fname>, --out <fname>
      output file name. (e.g. sample.mmdb)
      default: stdout.
    -c, --csv
      interpret input file as CSV.
      by default, the .csv extension will turn this on.
    -t, --tsv
      interpret input file as TSV.
      by default, the .tsv extension will turn this on.
    -j, --json
      interpret input file as JSON.
      by default, the .json extension will turn this on.

  Fields:
    One of the following fields flags, or other flags that implicitly specify
    these, must be used, otherwise --fields-from-header is assumed.

    The first field is always implicitly the network field, unless
    --range-multicol is used, in which case the first 2 fields are considered
    to be start_ip,end_ip.

    When specifying --fields, do not specify the network column(s).

    -f, --fields <comma-separated-fields>
      explicitly specify the fields to assume exist in the input file.
      example: col1,col2,col3
      default: N/A.
    --fields-from-header
      assume first line of input file is a header, and set the fields as that.
      default: true if no other field source is used, false otherwise.
    --range-multicol
      assume that the network field is actually two columns start_ip,end_ip.
      default: false.
    --joinkey-col
      assume --range-multicol and that the 3rd column is join_key, and ignore
      this column when converting to JSON.
      default: false.
    --no-fields
      specify that no fields exist except the implicit network field.
      when enabled, --no-network has no effect; the network field is written.
      default: false.
    --no-network
      if --fields-from-header is set, then don't write the network field, which
      is assumed to be the *first* field in the header.
      default: false.

  Meta:
    --ip <4 | 6>
      output file's ip version.
      default: 6.
    -s, --size <24 | 28 | 32>
      size of records in the mmdb tree.
      default: 32.
    -m, --merge <none | toplevel | recurse>
      the merge strategy to use when inserting entries that conflict.
        none     => no merge; only replace conflicts.
        toplevel => merge only top-level keys.
        recurse  => recursively merge.
      default: none.
    --ignore-empty-values
      if enabled, write into /0 with empty values for all fields, and for any
      entry, don't write out a field whose value is the empty string.
      default: false.
    --disallow-reserved
      disallow reserved networks to be added to the tree.
      default: false.
    --alias-6to4
      enable the mapping of some IPv6 networks into the IPv4 network, e.g.
      ::ffff:0:0/96, 2001::/32 & 2002::/16.
      default: false.
    --disable-metadata-pointers
      some mmdb readers fail to properly read pointers within metadata. this
      allows turning off such pointers.
      NOTE: on by default until we use a different reader in the data repo.
      default: true.
`, progBase)
}

func cmdImport() error {
	var fIn string
	var fOut string
	var fCsv bool
	var fTsv bool
	var fJson bool
	var fFields []string
	var fFieldsFromHdr bool
	var fRangeMultiCol bool
	var fJoinKeyCol bool
	var fNoFields bool
	var fNoNetwork bool
	var fIp int
	var fSize int
	var fMerge string
	var fIgnoreEmptyVals bool
	var fDisallowReserved bool
	var fAlias6to4 bool
	var fDisableMetadataPtrs bool

	_h := "see description in --help"
	pflag.StringVarP(&fIn, "in", "i", "", _h)
	pflag.StringVarP(&fOut, "out", "o", "", _h)
	pflag.BoolVarP(&fCsv, "csv", "c", false, _h)
	pflag.BoolVarP(&fTsv, "tsv", "t", false, _h)
	pflag.BoolVarP(&fJson, "json", "j", false, _h)
	pflag.StringSliceVarP(&fFields, "fields", "f", nil, _h)
	pflag.BoolVar(&fFieldsFromHdr, "fields-from-header", false, _h)
	pflag.BoolVar(&fRangeMultiCol, "range-multicol", false, _h)
	pflag.BoolVar(&fJoinKeyCol, "joinkey-col", false, _h)
	pflag.BoolVar(&fNoFields, "no-fields", false, _h)
	pflag.BoolVar(&fNoNetwork, "no-network", false, _h)
	pflag.IntVar(&fIp, "ip", 6, _h)
	pflag.IntVarP(&fSize, "size", "s", 32, _h)
	pflag.StringVarP(&fMerge, "merge", "m", "none", _h)
	pflag.BoolVar(&fIgnoreEmptyVals, "ignore-empty-values", false, _h)
	pflag.BoolVar(&fDisallowReserved, "disallow-reserved", false, _h)
	pflag.BoolVar(&fAlias6to4, "alias-6to4", false, _h)
	pflag.BoolVar(&fDisableMetadataPtrs, "disable-metadata-pointers", true, _h)
	pflag.Parse()

	// help?
	if fHelp || (pflag.NArg() == 1 && pflag.NFlag() == 0) {
		printHelpImport()
		return nil
	}

	// optional input as 1st argument.
	if strings.HasSuffix(pflag.Arg(1), ".csv") ||
		strings.HasSuffix(pflag.Arg(1), ".tsv") ||
		strings.HasSuffix(pflag.Arg(1), ".json") {
		fIn = pflag.Arg(1)
		fOut = pflag.Arg(2)
	}

	// validate IP version.
	if fIp != 4 && fIp != 6 {
		return errors.New("ip version must be \"4\" or \"6\"")
	}

	// validate record size.
	if fSize != 24 && fSize != 28 && fSize != 32 {
		return errors.New("record size must be 24, 28 or 32")
	}

	// validate merge strategy.
	var mergeStrategy inserter.FuncGenerator
	if fMerge == "none" {
		mergeStrategy = inserter.ReplaceWith
	} else if fMerge == "toplevel" {
		mergeStrategy = inserter.TopLevelMergeWith
	} else if fMerge == "recurse" {
		mergeStrategy = inserter.DeepMergeWith
	} else {
		return errors.New("merge strategy must be \"none\", \"toplevel\" or \"recurse\"")
	}

	// figure out file type.
	var delim rune
	if !fCsv && !fTsv && !fJson {
		if strings.HasSuffix(fIn, ".csv") {
			delim = ','
		} else if strings.HasSuffix(fIn, ".tsv") {
			delim = '\t'
		} else if strings.HasSuffix(fIn, ".json") {
			delim = '-'
		} else {
			return errors.New("input file type unknown")
		}
	} else {
		if fCsv && fTsv || fCsv && fJson || fTsv && fJson {
			return errors.New("multiple input file types specified")
		} else if fCsv {
			delim = ','
		} else if fTsv {
			delim = '\t'
		} else {
			delim = '-'
		}
	}

	// figure out fields.
	fieldSrcCnt := 0
	if fFields != nil && len(fFields) > 0 {
		fieldSrcCnt += 1
	}
	if fFieldsFromHdr {
		fieldSrcCnt += 1
	}
	if fNoFields {
		fieldSrcCnt += 1
	}
	if fieldSrcCnt > 1 {
		return errors.New("conflicting field sources specified.")
	}
	if fNoFields {
		fFields = []string{}
		fNoNetwork = false
	} else if !fFieldsFromHdr && (fFields == nil || len(fFields) == 0) {
		fFieldsFromHdr = true
	}

	if fJoinKeyCol {
		fRangeMultiCol = true
	}

	// prepare output file.
	var outFile *os.File
	if fOut == "" {
		outFile = os.Stdout
	} else {
		var err error
		outFile, err = os.Create(fOut)
		if err != nil {
			return fmt.Errorf("could not create %v: %w", fOut, err)
		}
		defer outFile.Close()
	}

	// init tree.
	dbdesc := "ipinfo " + filepath.Base(fOut)
	tree, err := mmdbwriter.New(
		mmdbwriter.Options{
			DatabaseType: dbdesc,
			Description: map[string]string{
				"en": dbdesc,
			},
			Languages:               []string{"en"},
			DisableIPv4Aliasing:     !fAlias6to4,
			IncludeReservedNetworks: !fDisallowReserved,
			IPVersion:               fIp,
			RecordSize:              fSize,
			DisableMetadataPointers: fDisableMetadataPtrs,
			Inserter:                mergeStrategy,
		},
	)
	if err != nil {
		return fmt.Errorf("could not create tree: %w", err)
	}

	// prepare input file.
	var inFile *os.File
	if fIn == "" || fIn == "-" {
		inFile = os.Stdin
	} else {
		var err error
		inFile, err = os.Open(fIn)
		if err != nil {
			return fmt.Errorf("invalid input file %v: %w", fIn, err)
		}
		defer inFile.Close()
	}

	entrycnt := 0
	if delim == ',' || delim == '\t' {
		var rdr reader
		if delim == ',' {
			csvrdr := csv.NewReader(inFile)
			csvrdr.Comma = delim
			csvrdr.LazyQuotes = true

			rdr = csvrdr
		} else {
			tsvrdr := NewTsvReader(inFile)

			rdr = tsvrdr
		}

		// read from input, scanning & parsing each line according to delim,
		// then insert that into the tree.
		dataColStart := 1
		hdrSeen := false
		for {
			parts, err := rdr.Read()
			if err == io.EOF {
				break
			} else if err != nil {
				return fmt.Errorf("input scanning failed: %w", err)
			}

			// on header line?
			if !hdrSeen {
				hdrSeen = true

				// check if the header has a multi-column range.
				if len(parts) > 1 && parts[0] == "start_ip" && parts[1] == "end_ip" {
					fRangeMultiCol = true

					// maybe we also have a join key?
					if len(parts) > 2 && parts[2] == "join_key" {
						fJoinKeyCol = true
					}
				}

				if fRangeMultiCol {
					if fJoinKeyCol {
						dataColStart = 3
					} else {
						dataColStart = 2
					}
				}

				// need to get fields from hdr?
				if fFieldsFromHdr {
					// skip all non-data columns.
					fFields = parts[dataColStart:]
				}

				// insert empty values for all fields in 0.0.0.0/0 if requested.
				if fIgnoreEmptyVals {
					_, network, _ := net.ParseCIDR("0.0.0.0/0")
					record := mmdbtype.Map{}
					for _, field := range fFields {
						record[mmdbtype.String(field)] = mmdbtype.String("")
					}
					if err := tree.Insert(network, record); err != nil {
						return errors.New(
							"couldn't insert empty values to 0.0.0.0/0",
						)
					}
				}

				// should we skip this first line now?
				if fFieldsFromHdr {
					continue
				}
			}

			networkStr := parts[0]

			// convert 2 IPs into IP range?
			if fRangeMultiCol {
				networkStr = parts[0] + "-" + parts[1]
			}

			// add network part to single-IP network if it's missing.
			isNetworkRange := strings.Contains(networkStr, "-")
			if !isNetworkRange && !strings.Contains(networkStr, "/") {
				if fIp == 6 && strings.Contains(networkStr, ":") {
					networkStr += "/128"
				} else {
					networkStr += "/32"
				}
			}

			// prep record.
			record := mmdbtype.Map{}
			if !fNoNetwork {
				record["network"] = mmdbtype.String(networkStr)
			}
			for i, field := range fFields {
				record[mmdbtype.String(field)] = mmdbtype.String(parts[i+dataColStart])
			}

			// range insertion or cidr insertion?
			if isNetworkRange {
				networkStrParts := strings.Split(networkStr, "-")
				startIp := net.ParseIP(networkStrParts[0])
				endIp := net.ParseIP(networkStrParts[1])
				if err := tree.InsertRange(startIp, endIp, record); err != nil {
					fmt.Fprintf(
						os.Stderr, "warn: couldn't insert line '%v'\n",
						strings.Join(parts, string(delim)),
					)
				}
			} else {
				_, network, err := net.ParseCIDR(networkStr)
				if err != nil {
					return fmt.Errorf(
						"couldn't parse cidr \"%v\": %w",
						networkStr, err,
					)
				}
				if err := tree.Insert(network, record); err != nil {
					fmt.Fprintf(
						os.Stderr, "warn: couldn't insert line '%v'\n",
						strings.Join(parts, string(delim)),
					)
				}
			}

			entrycnt += 1
		}
	} else if delim == '-' {
		// NOTE: this is not a streaming solution; the whole JSON input is
		// loaded into memory at once. streaming is much harder and we can do
		// it later if really needed for files that can't fit in RAM.
		file, err := ioutil.ReadAll(inFile)
		if err != nil {
			return fmt.Errorf("failed to read from input: %w", err)
		}

		firstSeen := false
		var data map[string]map[string]string
		json.Unmarshal([]byte(file), &data)
		for networkStr, recordMap := range data {
			// on first object?
			if !firstSeen {
				firstSeen = true

				// insert empty values for all fields in 0.0.0.0/0 if requested.
				if fIgnoreEmptyVals {
					_, network, _ := net.ParseCIDR("0.0.0.0/0")
					record := mmdbtype.Map{}
					for k, _ := range recordMap {
						record[mmdbtype.String(k)] = mmdbtype.String("")
					}
					if err := tree.Insert(network, record); err != nil {
						return errors.New(
							"couldn't insert empty values to 0.0.0.0/0",
						)
					}
				}
			}

			// add network part to single-IP network if it's missing.
			isNetworkRange := strings.Contains(networkStr, "-")
			if !isNetworkRange && !strings.Contains(networkStr, "/") {
				if fIp == 6 && strings.Contains(networkStr, ":") {
					networkStr += "/128"
				} else {
					networkStr += "/32"
				}
			}

			// prep record.
			record := mmdbtype.Map{}
			if !fNoNetwork {
				record["network"] = mmdbtype.String(networkStr)
			}
			for k, v := range recordMap {
				record[mmdbtype.String(k)] = mmdbtype.String(v)
			}

			// range insertion or cidr insertion?
			if isNetworkRange {
				networkStrParts := strings.Split(networkStr, "-")
				startIp := net.ParseIP(networkStrParts[0])
				endIp := net.ParseIP(networkStrParts[1])
				if err := tree.InsertRange(startIp, endIp, record); err != nil {
					fmt.Fprintf(
						os.Stderr, "warn: couldn't insert '%v'\n",
						recordMap,
					)
				}
			} else {
				_, network, err := net.ParseCIDR(networkStr)
				if err != nil {
					return fmt.Errorf(
						"couldn't parse cidr \"%v\": %w",
						networkStr, err,
					)
				}
				if err := tree.Insert(network, record); err != nil {
					fmt.Fprintf(
						os.Stderr, "warn: couldn't insert '%v'\n",
						recordMap,
					)
				}
			}

			entrycnt += 1
		}
	}

	if entrycnt == 0 {
		return errors.New("nothing to import")
	}

	// write out mmdb file.
	fmt.Fprintf(os.Stderr, "writing to %s (%v entries)\n", fOut, entrycnt)
	if _, err := tree.WriteTo(outFile); err != nil {
		return fmt.Errorf("writing out to tree failed: %w", err)
	}

	return nil
}
