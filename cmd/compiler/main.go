package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lfrfrfr/beon-ipquality/internal/compiler"
	"github.com/lfrfrfr/beon-ipquality/internal/config"
	pkglogger "github.com/lfrfrfr/beon-ipquality/pkg/logger"
)

func main() {
	// Parse command line flags
	configPath := flag.String("config", "./configs/config.yaml", "Path to configuration file")
	oneshot := flag.Bool("oneshot", false, "Run compilation once and exit")
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

	pkglogger.Info("Starting BEON-IPQuality MMDB Compiler")

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create compiler
	comp, err := compiler.New(cfg)
	if err != nil {
		pkglogger.Fatal(fmt.Sprintf("Failed to create compiler: %v", err))
	}
	defer comp.Close()

	if *oneshot {
		// One-shot mode: compile once and exit
		pkglogger.Info("Running in one-shot mode")
		if err := comp.Compile(ctx); err != nil {
			pkglogger.Fatal(fmt.Sprintf("Compilation failed: %v", err))
		}
		pkglogger.Info("Compilation complete")
		return
	}

	// Get compile interval, default to 1 hour if not set
	compileInterval := cfg.MMDB.CompileInterval
	if compileInterval <= 0 {
		compileInterval = 1 * time.Hour
		pkglogger.Info(fmt.Sprintf("Using default compile interval: %v", compileInterval))
	} else {
		pkglogger.Info(fmt.Sprintf("Compile interval: %v", compileInterval))
	}

	// Start periodic compilation
	go func() {
		ticker := time.NewTicker(compileInterval)
		defer ticker.Stop()

		// Run initial compilation
		if err := comp.Compile(ctx); err != nil {
			pkglogger.Error(fmt.Sprintf("Initial compilation failed: %v", err))
		}

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := comp.Compile(ctx); err != nil {
					pkglogger.Error(fmt.Sprintf("Periodic compilation failed: %v", err))
				}
			}
		}
	}()

	pkglogger.Info(fmt.Sprintf("MMDB Compiler started (interval: %v)", cfg.MMDB.CompileInterval))

	// Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	pkglogger.Info("Shutting down compiler...")
	cancel()

	pkglogger.Info("Compiler stopped gracefully")
}
