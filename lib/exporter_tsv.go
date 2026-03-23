package lib

import (
	"fmt"
	"io"

	"github.com/oschwald/maxminddb-golang/v2"
)

// tsvExporter exports records in TSV format.
type tsvExporter struct {
	wr      *TsvWriter
	cache   map[uintptr]map[string]string
	hdrKeys []string
	noHdr   bool
}

func newTSVExporter(w io.Writer, noHdr bool) *tsvExporter {
	return &tsvExporter{
		wr:    NewTsvWriter(w),
		cache: make(map[uintptr]map[string]string),
		noHdr: noHdr,
	}
}

func (e *tsvExporter) WriteRecord(result maxminddb.Result) error {
	offset := result.Offset()
	prefix := result.Prefix()

	var recordStr map[string]string
	if cached, ok := e.cache[offset]; ok {
		recordStr = cached
	} else {
		record := make(map[string]any)
		if err := result.Decode(&record); err != nil {
			return fmt.Errorf("failed to decode record: %w", err)
		}
		recordStr = mapInterfaceToStr(record)
		e.cache[offset] = recordStr
	}

	// Write header on first record.
	if e.hdrKeys == nil {
		e.hdrKeys = sortedMapKeys(recordStr)
		if !e.noHdr {
			hdr := append([]string{"range"}, e.hdrKeys...)
			if err := e.wr.Write(hdr); err != nil {
				return fmt.Errorf("failed to write header %v: %w", hdr, err)
			}
		}
	}

	// Build values in header key order.
	vals := make([]string, len(e.hdrKeys))
	for i, k := range e.hdrKeys {
		vals[i] = recordStr[k]
	}

	line := append([]string{prefix.String()}, vals...)
	if err := e.wr.Write(line); err != nil {
		return fmt.Errorf("failed to write line %v: %w", line, err)
	}
	return nil
}

func (e *tsvExporter) Flush() error {
	e.wr.Flush()
	return e.wr.Error()
}
