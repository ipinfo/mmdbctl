package lib

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"

	"github.com/oschwald/maxminddb-golang/v2"
)

// jsonExporter exports records in JSON Lines format.
type jsonExporter struct {
	bw    *bufio.Writer
	cache map[uintptr][]byte
}

func newJSONExporter(w io.Writer) *jsonExporter {
	return &jsonExporter{
		bw:    bufio.NewWriter(w),
		cache: make(map[uintptr][]byte),
	}
}

func (e *jsonExporter) WriteRecord(result maxminddb.Result) error {
	offset := result.Offset()
	prefix := result.Prefix()

	var jsonSuffix []byte
	if cached, ok := e.cache[offset]; ok {
		jsonSuffix = cached
	} else {
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
		e.cache[offset] = jsonSuffix
	}

	// Write: {"range":"<prefix>",...}\n
	e.bw.WriteString(`{"range":"`)
	e.bw.WriteString(prefix.String())
	e.bw.WriteString(`"`)
	if len(jsonSuffix) > 1 { // More than just "}"
		e.bw.WriteByte(',')
	}
	e.bw.Write(jsonSuffix)
	e.bw.WriteByte('\n')
	return nil
}

func (e *jsonExporter) Flush() error {
	return e.bw.Flush()
}
