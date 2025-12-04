package lib

import (
	"encoding/csv"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCmdExportFlags_Init(t *testing.T) {
	// Test that a newly initialized CmdExportFlags has correct zero values
	// Note: We don't call Init() here because it registers global flags
	// which would cause conflicts when running multiple test iterations
	f := CmdExportFlags{}

	// Test default/zero values
	if f.Help {
		t.Error("expected Help to be false by default")
	}
	if f.NoHdr {
		t.Error("expected NoHdr to be false by default")
	}
	if f.Format != "" {
		t.Errorf("expected Format to be empty by default, got %s", f.Format)
	}
	if f.Out != "" {
		t.Errorf("expected Out to be empty by default, got %s", f.Out)
	}
}

func TestCmdExport_NoInputFile(t *testing.T) {
	f := CmdExportFlags{}

	err := CmdExport(f, []string{}, func() {})
	if err == nil {
		t.Error("expected error for missing input file")
	}
	if !strings.Contains(err.Error(), "input mmdb file required") {
		t.Errorf("expected input file error, got: %s", err.Error())
	}
}

func TestCmdExport_InvalidFormat(t *testing.T) {
	tempDir := t.TempDir()
	mmdbFile := filepath.Join(tempDir, "test.mmdb")

	// Create a simple MMDB file for testing
	createTestMMDB(t, mmdbFile)

	f := CmdExportFlags{
		Format: "xml", // invalid format
		Out:    filepath.Join(tempDir, "output.xml"),
	}

	err := CmdExport(f, []string{mmdbFile}, func() {})
	if err == nil {
		t.Error("expected error for invalid format")
	}
	if !strings.Contains(err.Error(), "format must be") {
		t.Errorf("expected format error, got: %s", err.Error())
	}
}

func TestCmdExport_NonExistentMMDBFile(t *testing.T) {
	tempDir := t.TempDir()
	nonExistentFile := filepath.Join(tempDir, "nonexistent.mmdb")
	outputFile := filepath.Join(tempDir, "output.csv")

	f := CmdExportFlags{
		Out: outputFile,
	}

	err := CmdExport(f, []string{nonExistentFile}, func() {})
	if err == nil {
		t.Error("expected error for non-existent MMDB file")
	}
	if !strings.Contains(err.Error(), "couldn't open mmdb file") {
		t.Errorf("expected MMDB open error, got: %s", err.Error())
	}
}

func TestCmdExport_CSVFormat(t *testing.T) {
	tempDir := t.TempDir()
	mmdbFile := filepath.Join(tempDir, "test.mmdb")
	outputFile := filepath.Join(tempDir, "output.csv")

	createTestMMDB(t, mmdbFile)

	f := CmdExportFlags{
		Format: "csv",
		Out:    outputFile,
	}

	err := CmdExport(f, []string{mmdbFile}, func() {})
	if err != nil {
		t.Fatalf("unexpected error: %s", err.Error())
	}

	data := parseCSV(t, outputFile)

	assertRowCount(t, data, 3)
	assertCSVContains(t, data, map[string]string{
		"range":   "167.153.128.0/17",
		"asn":     "22252",
		"city":    "New York",
		"country": "US",
		"network": "167.153.128.0/17",
	})
	assertCSVContains(t, data, map[string]string{
		"range":   "204.138.232.0/24",
		"asn":     "14836",
		"city":    "Toronto",
		"country": "CA",
		"network": "204.138.232.0/24",
	})
	assertCSVContains(t, data, map[string]string{
		"range":   "5.150.80.0/20",
		"asn":     "60187",
		"city":    "London",
		"country": "UK",
		"network": "5.150.80.0/20",
	})
}

func TestCmdExport_CSVFormatNoHeader(t *testing.T) {
	tempDir := t.TempDir()
	mmdbFile := filepath.Join(tempDir, "test.mmdb")
	outputFile := filepath.Join(tempDir, "output.csv")

	createTestMMDB(t, mmdbFile)

	f := CmdExportFlags{
		Format: "csv",
		Out:    outputFile,
		NoHdr:  true,
	}

	err := CmdExport(f, []string{mmdbFile}, func() {})
	if err != nil {
		t.Fatalf("unexpected error: %s", err.Error())
	}

	file, err := os.Open(outputFile)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("failed to read CSV: %s", err.Error())
	}

	if len(records) != 3 {
		t.Errorf("expected 3 data rows, got %d", len(records))
	}

	assertRowContains(t, records, "167.153.128.0/17", "22252", "New York", "US", "167.153.128.0/17")
	assertRowContains(t, records, "204.138.232.0/24", "14836", "Toronto", "CA", "204.138.232.0/24")
	assertRowContains(t, records, "5.150.80.0/20", "60187", "London", "UK", "5.150.80.0/20")
}

func TestCmdExport_TSVFormat(t *testing.T) {
	tempDir := t.TempDir()
	mmdbFile := filepath.Join(tempDir, "test.mmdb")
	outputFile := filepath.Join(tempDir, "output.tsv")

	createTestMMDB(t, mmdbFile)

	f := CmdExportFlags{
		Format: "tsv",
		Out:    outputFile,
	}

	err := CmdExport(f, []string{mmdbFile}, func() {})
	if err != nil {
		t.Fatalf("unexpected error: %s", err.Error())
	}

	data := parseTSV(t, outputFile)

	assertRowCount(t, data, 3)
	assertCSVContains(t, data, map[string]string{
		"range":   "167.153.128.0/17",
		"asn":     "22252",
		"city":    "New York",
		"country": "US",
		"network": "167.153.128.0/17",
	})
	assertCSVContains(t, data, map[string]string{
		"range":   "204.138.232.0/24",
		"asn":     "14836",
		"city":    "Toronto",
		"country": "CA",
		"network": "204.138.232.0/24",
	})
	assertCSVContains(t, data, map[string]string{
		"range":   "5.150.80.0/20",
		"asn":     "60187",
		"city":    "London",
		"country": "UK",
		"network": "5.150.80.0/20",
	})
}

func TestCmdExport_JSONFormat(t *testing.T) {
	tempDir := t.TempDir()
	mmdbFile := filepath.Join(tempDir, "test.mmdb")
	outputFile := filepath.Join(tempDir, "output.json")

	createTestMMDB(t, mmdbFile)

	f := CmdExportFlags{
		Format: "json",
		Out:    outputFile,
	}

	err := CmdExport(f, []string{mmdbFile}, func() {})
	if err != nil {
		t.Fatalf("unexpected error: %s", err.Error())
	}

	records := parseJSONLines(t, outputFile)

	if len(records) != 3 {
		t.Errorf("expected 3 JSON records, got %d", len(records))
	}

	assertJSONContains(t, records, map[string]interface{}{
		"range":   "167.153.128.0/17",
		"asn":     "22252",
		"city":    "New York",
		"country": "US",
		"network": "167.153.128.0/17",
	})
	assertJSONContains(t, records, map[string]interface{}{
		"range":   "204.138.232.0/24",
		"asn":     "14836",
		"city":    "Toronto",
		"country": "CA",
		"network": "204.138.232.0/24",
	})
	assertJSONContains(t, records, map[string]interface{}{
		"range":   "5.150.80.0/20",
		"asn":     "60187",
		"city":    "London",
		"country": "UK",
		"network": "5.150.80.0/20",
	})
}

func TestCmdExport_FormatInferredFromFilename(t *testing.T) {
	tests := []struct {
		name           string
		filename       string
		expectedFormat string
	}{
		{"CSV extension", "output.csv", "csv"},
		{"TSV extension", "output.tsv", "tsv"},
		{"JSON extension", "output.json", "json"},
		{"Unknown extension defaults to CSV", "output.txt", "csv"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			mmdbFile := filepath.Join(tempDir, "test.mmdb")
			outputFile := filepath.Join(tempDir, tt.filename)

			createTestMMDB(t, mmdbFile)

			f := CmdExportFlags{
				Out: outputFile,
				// Format is intentionally empty to test inference
			}

			err := CmdExport(f, []string{mmdbFile}, func() {})
			if err != nil {
				t.Errorf("unexpected error: %s", err.Error())
			}

			if _, err := os.Stat(outputFile); os.IsNotExist(err) {
				t.Error("expected output file to be created")
			}
		})
	}
}

func TestCmdExport_OutputViaArgument(t *testing.T) {
	tempDir := t.TempDir()
	mmdbFile := filepath.Join(tempDir, "test.mmdb")
	outputFile := filepath.Join(tempDir, "output.csv")

	createTestMMDB(t, mmdbFile)

	// Use argument instead of flag for output
	f := CmdExportFlags{
		Format: "csv",
	}

	err := CmdExport(f, []string{mmdbFile, outputFile}, func() {})
	if err != nil {
		t.Errorf("unexpected error: %s", err.Error())
	}

	// Check if output file was created
	if _, err := os.Stat(outputFile); os.IsNotExist(err) {
		t.Error("expected output file to be created via argument")
	}
}

// Helper function to create an MMDB file with multiple records
func createTestMMDB(t *testing.T, outputPath string) {
	t.Helper()

	// Create CSV with multiple records
	tempDir := filepath.Dir(outputPath)
	inputCSV := filepath.Join(tempDir, "multi_input.csv")

	csvData := `network,country,city,asn
167.153.128.0/17,US,New York,22252
204.138.232.0/24,CA,Toronto,14836
5.150.80.0/20,UK,London,60187
`
	if err := os.WriteFile(inputCSV, []byte(csvData), 0644); err != nil {
		t.Fatal(err)
	}

	// Import it to create the MMDB
	f := CmdImportFlags{
		Ip:    6,
		Size:  32,
		Merge: "none",
		In:    inputCSV,
		Out:   outputPath,
		Csv:   true,
	}

	if err := CmdImport(f, []string{}, func() {}); err != nil {
		t.Fatalf("failed to create multi-record MMDB: %s", err.Error())
	}
}

// csvData represents parsed CSV/TSV data with header and rows
type csvData struct {
	header []string
	rows   []map[string]string
}

// parseCSV reads and parses a CSV file into structured data
func parseCSV(t *testing.T, path string) csvData {
	t.Helper()

	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("failed to open CSV: %s", err.Error())
	}
	defer file.Close()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("failed to read CSV: %s", err.Error())
	}

	if len(records) < 1 {
		t.Fatal("CSV has no data")
	}

	header := records[0]
	var rows []map[string]string
	for _, record := range records[1:] {
		row := make(map[string]string)
		for i, value := range record {
			if i < len(header) {
				row[header[i]] = value
			}
		}
		rows = append(rows, row)
	}

	return csvData{header: header, rows: rows}
}

// parseTSV reads and parses a TSV file into structured data
func parseTSV(t *testing.T, path string) csvData {
	t.Helper()

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read TSV: %s", err.Error())
	}

	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	if len(lines) < 1 {
		t.Fatal("TSV has no data")
	}

	header := strings.Split(lines[0], "\t")
	var rows []map[string]string
	for _, line := range lines[1:] {
		fields := strings.Split(line, "\t")
		row := make(map[string]string)
		for i, value := range fields {
			if i < len(header) {
				row[header[i]] = value
			}
		}
		rows = append(rows, row)
	}

	return csvData{header: header, rows: rows}
}

// parseJSONLines reads and parses JSON lines file
func parseJSONLines(t *testing.T, path string) []map[string]interface{} {
	t.Helper()

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read JSON: %s", err.Error())
	}

	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	var records []map[string]interface{}
	for _, line := range lines {
		if line == "" {
			continue
		}
		var record map[string]interface{}
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("failed to parse JSON line: %s", err.Error())
		}
		records = append(records, record)
	}

	return records
}

// assertRowCount checks if CSV data has expected number of rows
func assertRowCount(t *testing.T, data csvData, expected int) {
	t.Helper()
	if len(data.rows) != expected {
		t.Errorf("expected %d rows, got %d", expected, len(data.rows))
	}
}

// assertCSVContains checks if CSV data contains a row with expected values and no extra fields
func assertCSVContains(t *testing.T, data csvData, expected map[string]string) {
	t.Helper()

	for _, row := range data.rows {
		// Check if row has exactly the expected fields (no more, no less)
		if len(row) != len(expected) {
			continue
		}

		matches := true
		for key, expectedValue := range expected {
			if actualValue, ok := row[key]; !ok || actualValue != expectedValue {
				matches = false
				break
			}
		}
		if matches {
			return // Found matching row
		}
	}

	t.Errorf("expected to find row with exactly these values %v in CSV data", expected)
}

// assertJSONContains checks if JSON records contain an entry with expected values and no extra fields
func assertJSONContains(t *testing.T, records []map[string]interface{}, expected map[string]interface{}) {
	t.Helper()

	for _, record := range records {
		// Check if record has exactly the expected fields (no more, no less)
		if len(record) != len(expected) {
			continue
		}

		matches := true
		for key, expectedValue := range expected {
			if actualValue, ok := record[key]; !ok || actualValue != expectedValue {
				matches = false
				break
			}
		}
		if matches {
			return // Found matching record
		}
	}

	t.Errorf("expected to find JSON record with exactly these values %v", expected)
}

// assertRowContains checks if any row in the raw records contains exactly the expected values
func assertRowContains(t *testing.T, records [][]string, expectedValues ...string) {
	t.Helper()

	for _, record := range records {
		// Check if record has exactly the expected number of fields
		if len(record) != len(expectedValues) {
			continue
		}

		allFound := true
		for _, expected := range expectedValues {
			found := false
			for _, value := range record {
				if value == expected {
					found = true
					break
				}
			}
			if !found {
				allFound = false
				break
			}
		}
		if allFound {
			return // Found matching row
		}
	}

	t.Errorf("expected to find row with exactly these values: %v", expectedValues)
}
