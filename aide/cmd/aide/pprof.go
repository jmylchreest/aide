//go:build pprof

package main

import (
	"net/http"
	_ "net/http/pprof"
	"os"
	"strings"
)

func initPprof() {
	pprofAddr := os.Getenv("AIDE_PPROF_ADDR")
	if pprofAddr == "" {
		pprofAddr = "localhost:6060"
	}
	if !strings.HasPrefix(pprofAddr, "127.0.0.1:") && !strings.HasPrefix(pprofAddr, "localhost:") {
		mcpLog.Printf("WARNING: pprof binding to %s - this exposes debug endpoints", pprofAddr)
	}
	go func() {
		mcpLog.Printf("pprof server starting on http://%s/debug/pprof/", pprofAddr)
		if err := http.ListenAndServe(pprofAddr, nil); err != nil {
			mcpLog.Printf("pprof server error: %v", err)
		}
	}()
}
