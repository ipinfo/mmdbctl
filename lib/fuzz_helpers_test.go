package lib

import (
	"os"
	"testing"
)

// redirectStderr points os.Stderr at a discard file for the duration of the
// fuzz target. The import path prints a line per invocation ("writing to ...",
// "warn: couldn't insert ...") which would otherwise flood the fuzzing log
// and slow the run down.
func redirectStderr(t testing.TB) {
	t.Helper()

	devnull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open %s: %v", os.DevNull, err)
	}
	orig := os.Stderr
	os.Stderr = devnull
	t.Cleanup(func() {
		os.Stderr = orig
		_ = devnull.Close()
	})
}
