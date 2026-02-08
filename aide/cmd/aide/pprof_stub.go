//go:build !pprof

package main

func initPprof() {
	mcpLog.Printf("WARNING: pprof not available (build without -tags pprof)")
}
