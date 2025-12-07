package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lfrfrfr/beon-ipquality/internal/config"
	"github.com/lfrfrfr/beon-ipquality/internal/judge"
	pkglogger "github.com/lfrfrfr/beon-ipquality/pkg/logger"
)

func main() {
	// Parse command line flags
	configPath := flag.String("config", "./configs/config.yaml", "Path to configuration file")
	flag.Parse()

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Printf("Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	if err := pkglogger.Init(cfg.Logging.Level, cfg.Logging.Format, cfg.Logging.Output, cfg.Logging.FilePath); err != nil {
		fmt.Printf("Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer pkglogger.Sync()

	pkglogger.Info("Starting BEON-IPQuality Judge Node")

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create judge node
	node, err := judge.New(cfg)
	if err != nil {
		pkglogger.Fatal(fmt.Sprintf("Failed to create judge node: %v", err))
	}
	defer node.Close()

	// Start judge node
	go func() {
		if err := node.Start(ctx); err != nil {
			pkglogger.Error(fmt.Sprintf("Judge node error: %v", err))
		}
	}()

	pkglogger.Info(fmt.Sprintf("Judge node started, listening on %s:%d", cfg.Server.Host, cfg.Judge.Port))

	// Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	pkglogger.Info("Shutting down judge node...")
	cancel()

	// Give time for graceful shutdown
	time.Sleep(2 * time.Second)

	pkglogger.Info("Judge node stopped gracefully")
}
