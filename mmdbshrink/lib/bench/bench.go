// Package bench measures MaxMind DB lookup performance.
package bench

import (
	"runtime"
	"runtime/debug"
)

// Env describes the process a measurement ran in, so saved results from
// different machines or runtime settings can be told apart.
type Env struct {
	GoMaxProcs    int    `json:"gomaxprocs"`
	GoSetMemLimit int64  `json:"gomemlimit_bytes"`
	GoVersion     string `json:"go_version"`
	BinaryArch    string `json:"binary_arch"`
}

// currentEnv reads Env for this process as it is now.
func currentEnv() Env {
	goVer := runtime.Version()
	if bi, ok := debug.ReadBuildInfo(); ok {
		goVer = bi.GoVersion
	}
	return Env{
		GoMaxProcs:    runtime.GOMAXPROCS(0),
		GoSetMemLimit: debug.SetMemoryLimit(-1),
		GoVersion:     goVer,
		BinaryArch:    runtime.GOOS + "/" + runtime.GOARCH,
	}
}
