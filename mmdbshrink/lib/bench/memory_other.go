//go:build !(linux || darwin)

package bench

import (
	"errors"
	"os"
	"runtime"
)

var errMemoryUnsupported = errors.New("measuring memory is only supported on Linux and macOS, not " + runtime.GOOS)

func checkMemorySupport(cold bool) error {
	return errMemoryUnsupported
}

func readResourceUsage() (resourceUsage, error) {
	return resourceUsage{}, errMemoryUnsupported
}

func evictMapping(mapped []byte) error {
	return errMemoryUnsupported
}

func residentPages(mapped []byte) (pages, bytes uint64, pageSize int, err error) {
	return 0, 0, os.Getpagesize(), errMemoryUnsupported
}
