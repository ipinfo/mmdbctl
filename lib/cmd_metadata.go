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

const (
	MetadataStartMarker = "\xAB\xCD\xEFMaxMind.com"
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
	dataSectionEndOffset := 0
	dataSectionSize := 0
	metadataSectionStartOffset := 0

	// Offset of this separator is used to determine the metadata start section, data section end and data section size.
	offset, err := findSectionSeparator(args[0], MetadataStartMarker)
	if err != nil {
		return fmt.Errorf("couldn't process the mmdb file: %w", err)
	}

	if offset == -1 {
		return errors.New("input valid mmdb file required as first argument")
	}
	dataSectionEndOffset = int(offset)
	dataSectionSize = int(offset) - treeSize - 16
	typeSizes, err := traverseDataSection(args[0], int64(dataSectionStartOffset), int64(dataSectionEndOffset))
	if err != nil {
		return fmt.Errorf("couldn't process the mmdb file: %w", err)
	}
	fmt.Println(typeSizes)
	metadataSectionStartOffset = int(offset) + len(MetadataStartMarker)

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

		typeSizePrintlineGen := func(entryLen string) func(string, string) {
			return func(name string, val string) {
				fmt.Printf(
					"    - %v %v\n",
					fmtEntry.Sprintf("%-"+entryLen+"s", name),
					fmtVal.Sprintf("%v", val),
				)
			}
		}

		printline := printlineGen("13")
		typeSizePrintline := typeSizePrintlineGen("13")
		printline("Binary Format", binaryFmt)
		printline("Database Type", mdFromLib.DatabaseType)
		printline("IP Version", strconv.Itoa(int(mdFromLib.IPVersion)))
		printline("Record Size", strconv.Itoa(int(mdFromLib.RecordSize)))
		printline("Node Count", strconv.Itoa(int(mdFromLib.NodeCount)))
		printline("Tree Size", strconv.Itoa(treeSize))
		printline("Data Section Size", strconv.Itoa(dataSectionSize))
		typeSizePrintline("Pointer Size:", strconv.Itoa(int(typeSizes.PointerSize)))
		typeSizePrintline("UTF-8 String Size:", strconv.Itoa(int(typeSizes.Utf8StringSize)))
		typeSizePrintline("Double Size:", strconv.Itoa(int(typeSizes.DoubleSize)))
		typeSizePrintline("Bytes Size:", strconv.Itoa(int(typeSizes.BytesSize)))
		typeSizePrintline("Unsigned 16-bit Integer Size:", strconv.Itoa(int(typeSizes.Unsigned16bitIntSize)))
		typeSizePrintline("Unsigned 32-bit Integer Size:", strconv.Itoa(int(typeSizes.Unsigned32bitIntSize)))
		typeSizePrintline("Signed 32-bit Integer Size:", strconv.Itoa(int(typeSizes.Signed32bitIntSize)))
		typeSizePrintline("Unsigned 64-bit Integer Size:", strconv.Itoa(int(typeSizes.Unsigned64bitIntSize)))
		typeSizePrintline("Unsigned 128-bit Integer Size:", strconv.Itoa(int(typeSizes.Unsigned128bitIntSize)))
		typeSizePrintline("Map Key-Value Pair Count:", strconv.Itoa(int(typeSizes.MapKeyValueCount)))
		typeSizePrintline("Array Length:", strconv.Itoa(int(typeSizes.ArrayLength)))
		typeSizePrintline("Float Size:", strconv.Itoa(int(typeSizes.FloatSize)))
		printline("Data Section Start Offset", strconv.Itoa(dataSectionStartOffset))
		printline("Data Section End Offset", strconv.Itoa(dataSectionEndOffset))
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
			BinaryFormatVsn string `json:"binary_format"`
			DatabaseType    string `json:"db_type"`
			IPVersion       uint   `json:"ip"`
			RecordSize      uint   `json:"record_size"`
			NodeCount       uint   `json:"node_count"`
			TreeSize        uint   `json:"tree_size"`
			DataSectionSize uint   `json:"data_section_size"`
			TypeSize        struct {
				PointerSize           int64 `json:"pointer_size"`
				Utf8StringSize        int64 `json:"utf8_string_size"`
				DoubleSize            int64 `json:"double_size"`
				BytesSize             int64 `json:"bytes_size"`
				Unsigned16bitIntSize  int64 `json:"unsigned_16-bit_int_size"`
				Unsigned32bitIntSize  int64 `json:"unsigned_32-bit_int_size"`
				Signed32bitIntSize    int64 `json:"signed_32-bit_int_size"`
				Unsigned64bitIntSize  int64 `json:"unsigned_64-bit_int_size"`
				Unsigned128bitIntSize int64 `json:"unsigned_128-bit_int_size"`
				MapKeyValueCount      int64 `json:"map_key_value_pair_count"`
				ArrayLength           int64 `json:"array_length"`
				FloatSize             int64 `json:"float_size"`
			} `json:"type_size"`
			DataSectionStartOffset uint              `json:"data_section_start_offset"`
			DataSectionEndOffset   uint              `json:"data_section_end_offset"`
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
			uint(dataSectionSize),
			struct {
				PointerSize           int64 "json:\"pointer_size\""
				Utf8StringSize        int64 "json:\"utf8_string_size\""
				DoubleSize            int64 "json:\"double_size\""
				BytesSize             int64 "json:\"bytes_size\""
				Unsigned16bitIntSize  int64 "json:\"unsigned_16-bit_int_size\""
				Unsigned32bitIntSize  int64 "json:\"unsigned_32-bit_int_size\""
				Signed32bitIntSize    int64 "json:\"signed_32-bit_int_size\""
				Unsigned64bitIntSize  int64 "json:\"unsigned_64-bit_int_size\""
				Unsigned128bitIntSize int64 "json:\"unsigned_128-bit_int_size\""
				MapKeyValueCount      int64 "json:\"map_key_value_pair_count\""
				ArrayLength           int64 "json:\"array_length\""
				FloatSize             int64 "json:\"float_size\""
			}{
				typeSizes.PointerSize,
				typeSizes.Utf8StringSize,
				typeSizes.DoubleSize,
				typeSizes.BytesSize,
				typeSizes.Unsigned16bitIntSize,
				typeSizes.Unsigned32bitIntSize,
				typeSizes.Signed32bitIntSize,
				typeSizes.Unsigned64bitIntSize,
				typeSizes.Unsigned128bitIntSize,
				typeSizes.MapKeyValueCount,
				typeSizes.ArrayLength,
				typeSizes.FloatSize,
			},
			uint(dataSectionStartOffset),
			uint(dataSectionEndOffset),
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
