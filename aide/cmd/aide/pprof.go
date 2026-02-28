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
	if !strings.HasPrefix(pprofAddr, "127.0.0.1:") && !strings.HasPrefix(pprofAddr, "localhost:") && !strings.HasPrefix(pprofAddr, "[::1]:") {
		mcpLog.Printf("ERROR: refusing to bind pprof to %s — only localhost addresses are allowed (set AIDE_PPROF_ADDR to 127.0.0.1:<port> or localhost:<port>)", pprofAddr)
		return
	}
	srv := &http.Server{
		Addr:         pprofAddr,
		Handler:      nil,
		ReadTimeout:  DefaultPprofReadTimeout,
		WriteTimeout: DefaultPprofWriteTimeout,
		IdleTimeout:  DefaultPprofIdleTimeout,
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
		ctx, cancel := context.WithTimeout(context.Background(), DefaultPprofShutdownTimeout)
		defer cancel()
		if err := pprofServer.Shutdown(ctx); err != nil {
			mcpLog.Printf("pprof shutdown error: %v", err)
		}
		pprofServer = nil
	}
}
