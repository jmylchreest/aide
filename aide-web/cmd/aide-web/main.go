package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/jmylchreest/aide/aide-web/internal/instance"
	"github.com/jmylchreest/aide/aide-web/internal/server"
	"github.com/pkg/browser"
)

func main() {
	cfg := parseFlags()

	mgr, err := instance.NewManager()
	if err != nil {
		log.Fatalf("failed to create instance manager: %v", err)
	}
	defer mgr.Close()

	srv, err := server.New(mgr, cfg.Dev)
	if err != nil {
		log.Fatalf("failed to create server: %v", err)
	}

	httpServer := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Addr, cfg.Port),
		Handler:      srv.Handler(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	url := displayURL(cfg.Addr, cfg.Port)
	go func() {
		// Print the full scheme+host+port so the line is click-to-open in most
		// terminals.
		log.Printf("aide-web listening on %s", url)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	if cfg.Open {
		// Best-effort: open the dashboard in the user's browser. pkg/browser
		// dispatches per-OS via build tags (xdg-open / open / rundll32), so this
		// works on Linux, macOS and Windows. Run it detached and ignore the
		// error — a missing/failed launcher (headless, SSH, no browser) must
		// never block or fail dashboard startup. Discard launcher output so it
		// doesn't clutter the server log.
		browser.Stdout, browser.Stderr = io.Discard, io.Discard
		go func() { _ = browser.OpenURL(url) }()
	}

	<-ctx.Done()
	log.Println("shutting down...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown error: %v", err)
	}
}

type config struct {
	Port int
	Addr string
	Dev  bool
	Open bool
}

func parseFlags() config {
	cfg := config{}
	flag.IntVar(&cfg.Port, "port", 8080, "HTTP port")
	flag.StringVar(&cfg.Addr, "addr", "127.0.0.1", "Listen address")
	flag.BoolVar(&cfg.Dev, "dev", false, "Development mode (load templates from disk)")
	flag.BoolVar(&cfg.Open, "open", false, "Open browser on startup")
	flag.Parse()
	return cfg
}

// displayURL builds a clickable http URL from the listen address and port.
// Wildcard binds (0.0.0.0, ::) and an empty address are shown as localhost so
// the printed link is openable when clicked; an explicit host is kept as-is.
func displayURL(addr string, port int) string {
	host := addr
	switch host {
	case "", "0.0.0.0", "::", "[::]":
		host = "localhost"
	}
	return fmt.Sprintf("http://%s:%d", host, port)
}
