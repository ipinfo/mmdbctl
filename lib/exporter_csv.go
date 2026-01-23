package lib

import (
	"encoding/csv"
	"fmt"
	"io"

	"github.com/oschwald/maxminddb-golang/v2"
)

// csvExporter exports records in CSV format.
type csvExporter struct {
	wr      *csv.Writer
	cache   map[uintptr]map[string]string
	hdrKeys []string
	noHdr   bool
}

func newCSVExporter(w io.Writer, noHdr bool) *csvExporter {
	return &csvExporter{
		wr:    csv.NewWriter(w),
		cache: make(map[uintptr]map[string]string),
		noHdr: noHdr,
	}
}

func (e *csvExporter) WriteRecord(result maxminddb.Result) error {
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

func (e *csvExporter) Flush() error {
	e.wr.Flush()
	return e.wr.Error()
}
