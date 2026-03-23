package lib

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/oschwald/maxminddb-golang/v2"
)

const rangePlaceholder = "__RANGE__"

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

	cached, ok := e.cache[offset]
	if !ok {
		record := make(map[string]any)
		if err := result.Decode(&record); err != nil {
			return fmt.Errorf("failed to decode record: %w", err)
		}
		record["range"] = rangePlaceholder

		encoded, err := json.Marshal(record)
		if err != nil {
			return fmt.Errorf("failed to encode record: %w", err)
		}
		cached = encoded
		e.cache[offset] = cached
	}

	line := bytes.Replace(cached, []byte(rangePlaceholder), []byte(prefix.String()), 1)
	e.bw.Write(line)
	e.bw.WriteByte('\n')
	return nil
}

func (e *jsonExporter) Flush() error {
	return e.bw.Flush()
}
