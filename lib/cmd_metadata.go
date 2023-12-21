package lib

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/fatih/color"
	"github.com/oschwald/maxminddb-golang"
	"github.com/spf13/pflag"
)

var (
	metadataStartMarker = "\xAB\xCD\xEFMaxMind.com"
)

// CmdMetadataFlags are flags expected by CmdMetadata.
type CmdMetadataFlags struct {
	Help    bool
	NoColor bool
	Format  string
}

// Init initializes the common flags available to CmdMetadata with sensible
// defaults.
//
// pflag.Parse() must be called to actually use the final flag values.
func (f *CmdMetadataFlags) Init() {
	_h := "see description in --help"
	pflag.BoolVarP(
		&f.Help,
		"help", "h", false,
		"show help.",
	)
	pflag.BoolVar(
		&f.NoColor,
		"nocolor", false,
		_h,
	)
	pflag.StringVarP(
		&f.Format,
		"format", "f", "",
		_h,
	)
}

func CmdMetadata(f CmdMetadataFlags, args []string, printHelp func()) error {
	if f.NoColor {
		color.NoColor = true
	}

	// help?
	if f.Help || (pflag.NArg() == 1 && pflag.NFlag() == 0) {
		printHelp()
		return nil
	}

	// validate input file.
	if len(args) == 0 {
		return errors.New("input mmdb file required as first argument")
	}

	// validate format
	if f.Format == "" {
		f.Format = "pretty"
	}
	if f.Format != "pretty" && f.Format != "json" {
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
	treeSize := ((int(mdFromLib.RecordSize) * 2) / 8) * int(mdFromLib.NodeCount)
	dataSectionStartOffset := treeSize + 16
	var (
		dataSectionEndOffset       int
		dataSectionSize            int
		metadataSectionStartOffset int
	)

	// Offset of this separator is used to determine the metadata start section, data section end and data section size.
	offset, err := findSectionSeparator(args[0], metadataStartMarker)
	if err != nil {
		return fmt.Errorf("couldn't process the mmdb file: %w", err)
	}

	if offset != -1 {
		dataSectionEndOffset = int(offset)
		dataSectionSize = int(offset) - treeSize - 16
		metadataSectionStartOffset = int(offset) + len(metadataStartMarker)
	} else {
		return errors.New("input valid mmdb file required as first argument")
	}
	if f.Format == "pretty" {
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
		printline("Tree Size", strconv.Itoa(treeSize))
		printline("Data Section Start Offset", strconv.Itoa(dataSectionStartOffset))
		printline("Data Section End Offset", strconv.Itoa(dataSectionEndOffset))
		printline("Data Section Size", strconv.Itoa(dataSectionSize))
		printline("Metadata Section Start Offset", strconv.Itoa(metadataSectionStartOffset))
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
			BinaryFormatVsn        string            `json:"binary_format"`
			DatabaseType           string            `json:"db_type"`
			IPVersion              uint              `json:"ip"`
			RecordSize             uint              `json:"record_size"`
			NodeCount              uint              `json:"node_count"`
			TreeSize               uint              `json:"tree_size"`
			DataSectionStartOffset uint              `json:"data_section_start_offset"`
			DataSectionEndOffset   uint              `json:"data_section_end_offset"`
			DataSectionSize        uint              `json:"data_section_size"`
			MetadataStartOffset    uint              `json:"metadata_section_start_offset"`
			Description            map[string]string `json:"description"`
			Languages              []string          `json:"languages"`
			BuildEpoch             uint              `json:"build_epoch"`
		}{
			binaryFmt,
			mdFromLib.DatabaseType,
			mdFromLib.IPVersion,
			mdFromLib.RecordSize,
			mdFromLib.NodeCount,
			uint(treeSize),
			uint(dataSectionStartOffset),
			uint(dataSectionEndOffset),
			uint(dataSectionSize),
			uint(metadataSectionStartOffset),
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
