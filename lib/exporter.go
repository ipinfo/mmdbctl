package lib

import (
	"fmt"

	"github.com/oschwald/maxminddb-golang/v2"
)

// exporter defines the interface for exporting MMDB records.
type exporter interface {
	WriteRecord(result maxminddb.Result) error
	Flush() error
}

// exportNetworks iterates over all networks in the database and writes them using the exporter.
func exportNetworks(db *maxminddb.Reader, exp exporter) error {
	for result := range db.Networks() {
		if err := result.Err(); err != nil {
			return fmt.Errorf("failed networks traversal: %w", err)
		}
		if err := exp.WriteRecord(result); err != nil {
			return err
		}
	}
	return exp.Flush()
}
