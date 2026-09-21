//go:build linux || darwin

package bench

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

var errColdDarwin = errors.New("not supported on macOS: madvise cannot evict file-backed pages there")

// checkMemorySupport fails fast on a request this platform can't serve, so
// Memory errors before running any lookups.
func checkMemorySupport(cold bool) error {
	if cold && runtime.GOOS == "darwin" {
		return fmt.Errorf("cold mode: %w", errColdDarwin)
	}
	return nil
}

func readResourceUsage() (resourceUsage, error) {
	var ru syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &ru); err != nil {
		return resourceUsage{}, err
	}
	return resourceUsage{
		maxRSSBytes: maxRSSBytes(&ru),
		minorFaults: int64(ru.Minflt),
		majorFaults: int64(ru.Majflt),
	}, nil
}

// maxRSSBytes normalises ru_maxrss, which macOS reports in bytes and Linux in
// kilobytes. The value alone doesn't say which, so branch on the OS.
func maxRSSBytes(ru *syscall.Rusage) uint64 {
	if runtime.GOOS == "darwin" {
		return uint64(ru.Maxrss)
	}
	return uint64(ru.Maxrss) * 1024
}

// evictMapping drops this process's resident pages of mapped. The pages stay
// in the kernel page cache, so the next touch is a minor fault, not a disk
// read.
//
// Linux only. On macOS no madvise advice evicts file-backed pages:
// MADV_DONTNEED, MADV_FREE and MADV_FREE_REUSABLE all leave every page
// resident, for shared and private mappings alike. Reporting after a no-op
// would label warm numbers as cold, so refuse instead.
func evictMapping(mapped []byte) error {
	if runtime.GOOS == "darwin" {
		return errColdDarwin
	}
	return unix.Madvise(mapped, unix.MADV_DONTNEED)
}

// residentPages counts how many of mapped's pages are in the kernel page
// cache. For a file mapping that is not what this process holds: pages cached
// by anyone count, and Linux reports every page as resident when the caller
// can't open the file for writing.
//
// x/sys/unix has no Mincore wrapper, so this is the raw syscall: one byte per
// page, low bit set if resident.
func residentPages(mapped []byte) (pages, bytes uint64, pageSize int, err error) {
	pageSize = os.Getpagesize()
	if len(mapped) == 0 {
		return 0, 0, pageSize, nil
	}
	pageCount := (len(mapped) + pageSize - 1) / pageSize
	vec := make([]byte, pageCount)
	_, _, errno := unix.Syscall(
		unix.SYS_MINCORE,
		uintptr(unsafe.Pointer(&mapped[0])),
		uintptr(len(mapped)),
		uintptr(unsafe.Pointer(&vec[0])),
	)
	runtime.KeepAlive(mapped)
	if errno != 0 {
		return 0, 0, pageSize, errno
	}
	for _, v := range vec {
		if v&1 != 0 {
			pages++
		}
	}
	return pages, pages * uint64(pageSize), pageSize, nil
}
