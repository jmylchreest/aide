//go:build pprof

package main

import (
	"context"
	"net/http"
	_ "net/http/pprof"
	"os"
	"strings"
	"time"
)

var pprofServer *http.Server

func initPprof() {
	pprofAddr := os.Getenv("AIDE_PPROF_ADDR")
	if pprofAddr == "" {
		pprofAddr = "localhost:6060"
	}
	if !strings.HasPrefix(pprofAddr, "127.0.0.1:") && !strings.HasPrefix(pprofAddr, "localhost:") {
		mcpLog.Printf("WARNING: pprof binding to %s - this exposes debug endpoints", pprofAddr)
	}
	srv := &http.Server{
		Addr:         pprofAddr,
		Handler:      nil,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
	pprofServer = srv
	go func() {
		mcpLog.Printf("pprof server starting on http://%s/debug/pprof/", pprofAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			mcpLog.Printf("pprof server error: %v", err)
		}
	}()
}

func stopPprof() {
	if pprofServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := pprofServer.Shutdown(ctx); err != nil {
			mcpLog.Printf("pprof shutdown error: %v", err)
		}
		pprofServer = nil
	}
}
