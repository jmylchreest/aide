//go:build pprof

package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"strconv"
)

var pprofServer *http.Server
var pprofBoundAddr string // actual address the server bound to

func initPprof() {
	pprofAddr := os.Getenv("AIDE_PPROF_ADDR")
	if pprofAddr == "" {
		pprofAddr = "localhost:6060"
	}

	host, portStr, err := net.SplitHostPort(pprofAddr)
	if err != nil {
		mcpLog.Printf("ERROR: invalid pprof address %q: %v", pprofAddr, err)
		return
	}

	if host != "127.0.0.1" && host != "localhost" && host != "::1" {
		mcpLog.Printf("ERROR: refusing to bind pprof to %s — only localhost addresses are allowed (set AIDE_PPROF_ADDR to 127.0.0.1:<port> or localhost:<port>)", pprofAddr)
		return
	}

	basePort, err := strconv.Atoi(portStr)
	if err != nil {
		mcpLog.Printf("ERROR: invalid pprof port %q: %v", portStr, err)
		return
	}

	// Try binding, incrementing port on failure.
	var listener net.Listener
	for i := 0; i < DefaultPprofMaxPortRetries; i++ {
		addr := net.JoinHostPort(host, strconv.Itoa(basePort+i))
		listener, err = net.Listen("tcp", addr)
		if err == nil {
			pprofBoundAddr = addr
			break
		}
		if i == 0 {
			mcpLog.Printf("pprof port %d in use, trying next ports...", basePort)
		}
	}

	if listener == nil {
		mcpLog.Printf("ERROR: could not bind pprof server after %d attempts (ports %d–%d): %v",
			DefaultPprofMaxPortRetries, basePort, basePort+DefaultPprofMaxPortRetries-1, err)
		return
	}

	srv := &http.Server{
		Handler:      nil,
		ReadTimeout:  DefaultPprofReadTimeout,
		WriteTimeout: DefaultPprofWriteTimeout,
		IdleTimeout:  DefaultPprofIdleTimeout,
	}
	pprofServer = srv

	go func() {
		mcpLog.Printf("pprof server listening on http://%s/debug/pprof/", pprofBoundAddr)
		if err := srv.Serve(listener); err != nil && err != http.ErrServerClosed {
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
		pprofBoundAddr = ""
	}
}

// pprofURL returns the full pprof base URL if running, or empty string.
func pprofURL() string {
	if pprofBoundAddr == "" {
		return ""
	}
	host, port, _ := net.SplitHostPort(pprofBoundAddr)
	// Normalise to localhost for display consistency.
	if host == "127.0.0.1" || host == "::1" {
		host = "localhost"
	}
	return fmt.Sprintf("http://%s/debug/pprof/", net.JoinHostPort(host, port))
}
