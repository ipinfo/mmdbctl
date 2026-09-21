// Package shrink shrinks MaxMind DB files by deduplicating identical subtrees of the search trie,
// producing a smaller file with identical lookup semantics that any reader can open.
package shrink

import (
	"bytes"
	"fmt"
	"github.com/ipinfo/mmdbctl/mmdbshrink/lib/format"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

const (
	dataSectionSeparatorSize = 16
)

// Options configures the Shrink function
type Options struct {
	// DisableCompact skips the data section compaction step
	DisableCompact bool

	// LogMemorySnapshots logs a memory snapshot after certain steps.
	// This is expensive and recommended for debugging only as it calls
	// runtime.ReadMemStats that stops the world.
	LogMemorySnapshots bool
}

// Result is the data returned by the Shrink function
type Result struct {
	// InputBytes is the input file size
	InputBytes uint64
	// OutputBytes is the output file size
	OutputBytes uint64
	// InputNodeCount is the number of nodes in the input file
	InputNodeCount uint32
	// OutputNodeCount is the number of nodes in the output file
	OutputNodeCount uint32
	// InputTreeBytes is the size in bytes of the search tree in the input file
	InputTreeBytes uint64
	// OutputTreeBytes is the size in bytes of the search tree in the output file
	OutputTreeBytes uint64
	// InputDataBytes is the size in bytes of the data section in the input file
	InputDataBytes uint64
	// OutputDataBytes is the size in bytes of the data section in the output file
	OutputDataBytes uint64
	// MetadataBytes is size in bytes of the metadata section, we don't add or remove
	// any key so the size is identical in input and output
	MetadataBytes uint64
	// Elapsed time from Shrink start to finish
	Elapsed time.Duration
}

// Shrink reads the mmdb file at inputPath, deduplicates identical subtrees of
// its search trie, and writes the smaller result to outputPath.
// The input file is left untouched.
func Shrink(logger *slog.Logger, inputPath, outputPath string, opts Options) (Result, error) {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}

	startTime := time.Now()
	result := Result{}

	logger.Debug(fmt.Sprintf("read: %s", inputPath))
	timeRead := time.Now()
	input, err := os.ReadFile(inputPath)
	if err != nil {
		return result, fmt.Errorf("failure reading input file: %w", err)
	}
	result.InputBytes = uint64(len(input))
	logger.Debug(fmt.Sprintf("read: %s loaded in %s",
		format.DecimalBytes(result.InputBytes),
		time.Since(timeRead).Round(time.Millisecond),
	))

	if opts.LogMemorySnapshots {
		logMemory(logger, "after-read")
	}

	metaStart, metaBytes, err := findMetadata(input)
	if err != nil {
		return result, fmt.Errorf("failure finding metadata: %w", err)
	}

	meta, err := decodeMetadata(metaBytes)
	if err != nil {
		return result, fmt.Errorf("failure decoding metadata: %w", err)
	}

	nodeCount, ok := meta["node_count"].(uint32)
	if !ok {
		return result, fmt.Errorf("metadata.node_count missing or wrong type (got %T)", meta["node_count"])
	}

	recordSize, ok := meta["record_size"].(uint16)
	if !ok {
		return result, fmt.Errorf("metadata.record_size missing or wrong type (got %T)", meta["record_size"])
	}
	if recordSize != 24 && recordSize != 28 && recordSize != 32 {
		return result, fmt.Errorf("unsupported record_size %d", recordSize)
	}
	result.InputNodeCount = nodeCount
	nodeBytes := uint64(recordSize) / 4
	treeBytes := uint64(nodeCount) * nodeBytes
	result.InputTreeBytes = treeBytes

	if metaStart < treeBytes+dataSectionSeparatorSize {
		return result, fmt.Errorf("metadata starts before end of tree+separator")
	}
	dataStart := treeBytes + dataSectionSeparatorSize
	dataEnd := metaStart
	result.InputDataBytes = dataEnd - dataStart
	result.MetadataBytes = result.InputBytes - metaStart
	logger.Debug(fmt.Sprintf("layout: nodes=%d record_size=%d tree=%s data=%s meta=%s",
		nodeCount,
		recordSize,
		format.DecimalBytes(result.InputTreeBytes),
		format.DecimalBytes(result.InputDataBytes),
		format.DecimalBytes(result.MetadataBytes),
	))

	tree := input[:treeBytes]
	dataSection := input[dataStart:dataEnd]

	// Phase 1: bottom-up canonical-ID assignment.
	logger.Debug(fmt.Sprintf("phase 1: canonicalize starting (%d input nodes)", nodeCount))
	canonicalizeStartTime := time.Now()
	canon, err := canonicalize(logger, tree, nodeCount, uint64(recordSize))
	if err != nil {
		return result, fmt.Errorf("failure during canonicalization: %w", err)
	}
	result.OutputNodeCount = uint32(len(canon))
	result.OutputTreeBytes = uint64(result.OutputNodeCount) * nodeBytes
	logger.Debug(fmt.Sprintf("phase1: canonicalize done in %s — %d -> %d nodes (%+.2f%%)",
		time.Since(canonicalizeStartTime).Round(time.Millisecond),
		nodeCount,
		result.OutputNodeCount,
		100.0*float64(int64(result.OutputNodeCount)-int64(nodeCount))/float64(nodeCount),
	))
	if opts.LogMemorySnapshots {
		logMemory(logger, "after-canon")
	}

	// Phase 1.5: data-section compaction. Find every record reachable from
	// the canonical tree's leaf pointers (transitively, including through
	// MMDB pointers/kind 1), emit them contiguously into a new data section,
	// rewrite tree leaf pointers with the new offsets.
	emittedData := dataSection
	if !opts.DisableCompact {
		emittedData, err = compact(logger, canon, dataSection)
		if err != nil {
			// We don't format error as the error we receive is already formatted
			return result, err
		}
		if opts.LogMemorySnapshots {
			logMemory(logger, "after-compact")
		}
	}

	// Phase 2: emit the new tree with rewritten pointers. Output node indices
	// are assigned in canonicalization order, with the root pinned at index 0.
	logger.Debug(fmt.Sprintf("phase2: emit tree (%s)", format.DecimalBytes(result.OutputTreeBytes)))
	buildTreeStartTime := time.Now()
	searchTree, err := buildSearchTree(canon, uint64(recordSize))
	if err != nil {
		return result, err
	}
	if uint64(len(searchTree)) != result.OutputTreeBytes {
		return result, fmt.Errorf("tree size mismatch: got %d want %d", len(searchTree), result.OutputTreeBytes)
	}
	logger.Debug(fmt.Sprintf("phase2: emit tree done in %s", time.Since(buildTreeStartTime).Round(time.Millisecond)))

	// Phase 3: rewrite metadata's node_count. Other fields are preserved.
	logger.Debug("phase3: rewrite metadata")
	// It's fine reusing the old meta data structure, other than changing the node count
	// the data is identical and we don't use it after this phase other than to write it
	// to file.
	meta["node_count"] = result.OutputNodeCount
	encodedMeta, err := encodeMetadata(meta)
	if err != nil {
		return result, fmt.Errorf("encode metadata: %w", err)
	}

	// Phase 4: assemble the output file.
	logger.Debug(fmt.Sprintf("phase4: write %s", outputPath))
	writeStartTime := time.Now()
	outputBytes, err := writeOutputFile(outputPath, searchTree, emittedData, encodedMeta)
	// We return these in the result in any case, even if there's an error so let's set them before
	// checking for the error
	result.OutputBytes = outputBytes
	result.OutputDataBytes = uint64(len(emittedData))
	if err != nil {
		return result, err
	}
	logger.Debug(fmt.Sprintf("phase4: wrote %s in %s",
		format.DecimalBytes(result.OutputBytes),
		time.Since(writeStartTime).Round(time.Millisecond),
	))
	result.Elapsed = time.Since(startTime)

	return result, nil
}

// writeOutputFile creates a new MMDB file at outputPath using the provided searchTree, data, and metadata.
// If the file exists it's overwritten.
// If the directories don't exist they are created.
// Returns the number of bytes written to file.
// Returns error if there's a failures creating the output directories or writing the file.
func writeOutputFile(
	outputPath string,
	searchTree []byte,
	data []byte,
	metadata []byte,
) (uint64, error) {
	var out bytes.Buffer
	out.Grow(len(searchTree) + dataSectionSeparatorSize + len(data) + len(metadataStartMarker) + len(metadata))
	out.Write(searchTree)
	out.Write(make([]byte, dataSectionSeparatorSize))
	out.Write(data)
	out.Write(metadataStartMarker)
	out.Write(metadata)
	outputBytes := uint64(out.Len())

	if dir := filepath.Dir(outputPath); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return outputBytes, fmt.Errorf("create output dir: %w", err)
		}
	}
	if err := os.WriteFile(outputPath, out.Bytes(), 0o644); err != nil {
		return outputBytes, fmt.Errorf("write output: %w", err)
	}
	return outputBytes, nil
}

func logMemory(logger *slog.Logger, label string) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	logger.Debug(fmt.Sprintf("mem[%s]: alloc=%s sys=%s heap_in_use=%s",
		label, format.Bytes(m.Alloc), format.Bytes(m.Sys), format.Bytes(m.HeapInuse)))
}
