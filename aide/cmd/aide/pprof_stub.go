//go:build !pprof

package main

func initPprof() {
	mcpLog.Printf("WARNING: pprof not available (build without -tags pprof)")
}

func stopPprof() {
	// No-op when pprof is not compiled in.
}

// pprofURL returns empty when pprof is not compiled in.
func pprofURL() string {
	return ""
}
