package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lfrfrfr/beon-ipquality/internal/compiler"
	"github.com/lfrfrfr/beon-ipquality/internal/config"
	internalmetrics "github.com/lfrfrfr/beon-ipquality/internal/metrics"
	"github.com/lfrfrfr/beon-ipquality/internal/mmdb"
	pkglogger "github.com/lfrfrfr/beon-ipquality/pkg/logger"
)

func main() {
	// Parse command line flags
	verifyDirectory := flag.String("verify-mmdb", "", "Validate an MMDB directory offline and exit")
	configPath := flag.String("config", "./configs/config.yaml", "Path to configuration file")
	skipInitial := flag.Bool("skip-initial", false, "Skip initial compilation after validating an existing MMDB")
	oneshot := flag.Bool("oneshot", false, "Run compilation once and exit")
	flag.Parse()
	if *verifyDirectory != "" {
		reader, err := mmdb.VerifyDirectory(*verifyDirectory)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		defer reader.Close()
		if err := json.NewEncoder(os.Stdout).Encode(reader.Stats()); err != nil {
			os.Exit(1)
		}
		return
	}

	// Load configuration
	cfg, err := config.LoadFromEnv(*configPath)
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

	if *skipInitial {
		reader, err := mmdb.NewReader(cfg.MMDB.OutputPath, "", "")
		if err != nil {
			pkglogger.Fatal(fmt.Sprintf("Cannot skip initial compilation without valid snapshot: %v", err))
		}
		reader.Close()
	}
	if cfg.Metrics.Enabled {
		if err := internalmetrics.Serve(ctx, cfg.Metrics.Host, cfg.Metrics.Port, cfg.Metrics.Path); err != nil {
			pkglogger.Fatal(fmt.Sprintf("Cannot start pipeline metrics: %v", err))
		}
	}

	// Start periodic compilation
	go func() {
		ticker := time.NewTicker(compileInterval)
		defer ticker.Stop()

		// Run initial compilation
		if !*skipInitial {
			if err := comp.Compile(ctx); err != nil {
				pkglogger.Error(fmt.Sprintf("Initial compilation failed: %v", err))
			}
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
