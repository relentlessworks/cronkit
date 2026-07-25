package main

import (
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/relentlessworks/cronkit/internal/api"
	"github.com/relentlessworks/cronkit/internal/auth"
	"github.com/relentlessworks/cronkit/internal/config"
	"github.com/relentlessworks/cronkit/internal/store"
)

func main() {
	cfg := config.Load()

	// Ensure data directory exists
	if err := os.MkdirAll(cfg.DataDir, 0755); err != nil {
		log.Fatalf("failed to create data directory: %v", err)
	}

	// Create store
	dataFile := filepath.Join(cfg.DataDir, "cronkit.json")
	s, err := store.New(dataFile)
	if err != nil {
		log.Fatalf("failed to create store: %v", err)
	}

	// Create auth
	a := auth.New(s, cfg.Secret, cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUser, cfg.SMTPPass, cfg.FromEmail)

	// Create scheduler
	sched := api.NewScheduler(s)

	// Create handlers
	h := api.NewHandlers(s, a, sched)
	sched.SetHandlers(h)

	// Start scheduler
	sched.Start()
	defer sched.Stop()

	// Set up routes
	mux := http.NewServeMux()
	h.SetupRoutes(mux)

	// Graceful shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Println("shutting down...")
		sched.Stop()
		os.Exit(0)
	}()

	log.Printf("cronkit listening on %s", cfg.Addr)
	if err := http.ListenAndServe(cfg.Addr, mux); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
