package lib

import (
	"os"
	"path/filepath"
	"testing"
)

// FuzzCmdImportJSON drives the whole JSON import path with arbitrary bytes.
// The JSON branch of CmdImport does unchecked type assertions on the decoded
// record, so malformed documents are the interesting input here.
func FuzzCmdImportJSON(f *testing.F) {
	redirectStderr(f)

	seeds := [][]byte{
		[]byte(`{"range":"1.2.3.0/24","country":"US"}`),
		[]byte(`{"start_ip":"1.2.3.4","end_ip":"1.2.3.255","country":"US"}`),
		[]byte(`{"start_ip":"1.2.3.4"}`),
		[]byte(`{"start_ip":"1.2.3.4","end_ip":7}`),
		[]byte(`[1,2,3]`),
		[]byte(`"a string"`),
		[]byte(`null`),
		[]byte(`{"range":"1.2.3.4","nested":{"a":[1,2,{"b":true}]}}`),
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		dir := t.TempDir()
		in := filepath.Join(dir, "in.json")
		out := filepath.Join(dir, "out.mmdb")
		if err := os.WriteFile(in, data, 0o644); err != nil {
			t.Fatalf("write input: %v", err)
		}

		// Errors are expected and fine; the point is that no input shape
		// makes the importer panic.
		_ = CmdImport(CmdImportFlags{
			Ip:    6,
			Size:  32,
			Merge: "none",
			In:    in,
			Out:   out,
			Json:  true,
		}, []string{}, func() {})
	})
}
