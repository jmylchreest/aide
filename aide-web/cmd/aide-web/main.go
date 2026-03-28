package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/jmylchreest/aide/aide-web/internal/instance"
	"github.com/jmylchreest/aide/aide-web/internal/server"
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

	go func() {
		log.Printf("aide-web listening on %s", httpServer.Addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	if cfg.Open {
		openBrowser(fmt.Sprintf("http://localhost:%d", cfg.Port))
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

func openBrowser(url string) {
	for _, name := range []string{"xdg-open", "open", "start"} {
		if err := exec.Command(name, url).Start(); err == nil {
			return
		}
	}
}
