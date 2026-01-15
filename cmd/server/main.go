package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"postal-inspection-service/internal/config"
	"postal-inspection-service/internal/db"
	"postal-inspection-service/internal/imap"
	"postal-inspection-service/internal/poller"
	"postal-inspection-service/internal/web"
)

// Set at build time via -ldflags
var (
	Version   = "dev"
	CommitSHA = "unknown"
)

func main() {
	log.Println("Starting Postal Inspection Service...")

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	log.Printf("Configuration loaded: IMAP=%s:%d, Poll=%v, Web=:%d",
		cfg.IMAPServer, cfg.IMAPPort, cfg.PollInterval, cfg.WebPort)

	// Initialize database
	database, err := db.New(cfg.DBPath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer database.Close()
	log.Printf("Database initialized at %s", cfg.DBPath)

	// Create IMAP client
	imapClient := imap.NewClient(cfg.IMAPServer, cfg.IMAPPort, cfg.Email, cfg.AppPassword)

	// Create poller
	emailPoller := poller.New(imapClient, database, cfg.PollInterval)

	// Create web server
	repoURL := "https://github.com/BrandonKowalski/postal-inspection-service"
	webServer, err := web.NewServer(database, cfg.WebPort, CommitSHA, repoURL)
	if err != nil {
		log.Fatalf("Failed to create web server: %v", err)
	}

	// Setup graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start poller in background
	go emailPoller.Start(ctx)

	// Start web server in background
	go func() {
		if err := webServer.Start(); err != nil {
			log.Printf("Web server error: %v", err)
			cancel()
		}
	}()

	log.Println("Service started successfully")

	// Wait for shutdown signal
	<-sigChan
	log.Println("Shutting down...")
	cancel()
	log.Println("Goodbye!")
}
