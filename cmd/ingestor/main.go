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
	"github.com/lfrfrfr/beon-ipquality/internal/database"
	"github.com/lfrfrfr/beon-ipquality/internal/ingestor"
	pkglogger "github.com/lfrfrfr/beon-ipquality/pkg/logger"
)

// Version info
var (
	Version   = "1.0.0"
	BuildTime = "unknown"
)

func main() {
	// Parse command line flags
	configPath := flag.String("config", "./configs/config.yaml", "Path to configuration file")
	feedsPath := flag.String("feeds", "./configs/feeds.yaml", "Path to feeds configuration file")
	runOnce := flag.Bool("once", false, "Run once and exit (don't start daemon)")
	verbose := flag.Bool("verbose", false, "Enable verbose output to stdout")
	showVersion := flag.Bool("version", false, "Show version information")
	flag.Parse()

	// Show version
	if *showVersion {
		fmt.Printf("BEON-IPQuality Ingestor v%s (built: %s)\n", Version, BuildTime)
		os.Exit(0)
	}

	// Print startup banner if verbose or once mode
	if *verbose || *runOnce {
		printBanner()
	}

	// Load configuration
	printProgress("Loading configuration...")
	cfg, err := config.Load(*configPath)
	if err != nil {
		printError("Failed to load configuration: %v", err)
		os.Exit(1)
	}
	printSuccess("Configuration loaded")

	// Load feeds configuration
	printProgress("Loading feeds configuration...")
	feedsCfg, err := config.LoadFeeds(*feedsPath)
	if err != nil {
		printError("Failed to load feeds configuration: %v", err)
		os.Exit(1)
	}

	enabledFeeds := feedsCfg.GetEnabledFeeds()
	printSuccess("Loaded %d enabled feeds", len(enabledFeeds))

	// Initialize logger - force stdout if verbose/once mode
	logOutput := cfg.Logging.Output
	if *verbose || *runOnce {
		logOutput = "stdout"
	}

	if err := pkglogger.Init(cfg.Logging.Level, cfg.Logging.Format, logOutput, cfg.Logging.FilePath); err != nil {
		printError("Failed to initialize logger: %v", err)
		os.Exit(1)
	}
	defer pkglogger.Sync()

	if !(*verbose || *runOnce) {
		pkglogger.Info("Starting BEON-IPQuality Ingestor Service")
	}

	// Connect to database
	printProgress("Connecting to PostgreSQL database...")
	dsn := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		cfg.Database.Postgres.Username,
		cfg.Database.Postgres.Password,
		cfg.Database.Postgres.Host,
		cfg.Database.Postgres.Port,
		cfg.Database.Postgres.Database,
		cfg.Database.Postgres.SSLMode,
	)

	db, err := database.NewPostgresDB(dsn, cfg.Database.Postgres.MaxConnections, cfg.Database.Postgres.MinConnections)
	if err != nil {
		printError("Failed to connect to database: %v", err)
		os.Exit(1)
	}
	defer db.Close()
	printSuccess("Connected to PostgreSQL")

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create ingestor service
	printProgress("Initializing ingestor service...")
	ing, err := ingestor.New(cfg, feedsCfg, db)
	if err != nil {
		printError("Failed to create ingestor: %v", err)
		os.Exit(1)
	}
	printSuccess("Ingestor initialized")

	// Run once mode - just fetch feeds and exit
	if *runOnce {
		fmt.Println()
		printHeader("FETCHING THREAT FEEDS")
		fmt.Println()

		startTime := time.Now()

		// Run ingestor with progress
		totalFeeds, totalEntries, totalStored, err := ing.RunOnce(ctx)

		elapsed := time.Since(startTime)

		fmt.Println()
		printHeader("INGESTION COMPLETE")
		fmt.Println()
		fmt.Printf("  📊 Feeds processed:    %d\n", totalFeeds)
		fmt.Printf("  📥 Entries fetched:    %d\n", totalEntries)
		fmt.Printf("  💾 Entries stored:     %d\n", totalStored)
		fmt.Printf("  ⏱️  Time elapsed:       %v\n", elapsed.Round(time.Millisecond))
		fmt.Println()

		if err != nil {
			printError("Ingestion completed with errors: %v", err)
			os.Exit(1)
		}

		if totalStored == 0 && totalEntries > 0 {
			printWarning("Warning: Entries were fetched but none stored to database")
			printWarning("Check database connection and table schema")
		}

		printSuccess("Ingestion completed successfully!")
		os.Exit(0)
	}

	// Daemon mode
	go func() {
		if err := ing.Start(ctx); err != nil {
			pkglogger.Error(fmt.Sprintf("Ingestor error: %v", err))
		}
	}()

	if *verbose {
		printSuccess("Ingestor daemon started")
		fmt.Println("Press Ctrl+C to stop...")
	} else {
		pkglogger.Info("Ingestor service started")
	}

	// Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	if *verbose {
		fmt.Println()
		printProgress("Shutting down ingestor...")
	} else {
		pkglogger.Info("Shutting down ingestor...")
	}

	cancel()

	// Wait for ingestor to stop
	ing.Stop()

	if *verbose {
		printSuccess("Ingestor stopped gracefully")
	} else {
		pkglogger.Info("Ingestor stopped gracefully")
	}
}

// Console output helpers
func printBanner() {
	fmt.Println()
	fmt.Println("╔═══════════════════════════════════════════════════════════╗")
	fmt.Println("║       BEON-IPQuality Threat Feed Ingestor                 ║")
	fmt.Printf("║       Version: %-42s ║\n", Version)
	fmt.Println("╚═══════════════════════════════════════════════════════════╝")
	fmt.Println()
}

func printHeader(msg string) {
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Printf("  %s\n", msg)
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
}

func printProgress(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Printf("\033[0;34m[*]\033[0m %s\n", msg)
}

func printSuccess(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Printf("\033[0;32m[✓]\033[0m %s\n", msg)
}

func printWarning(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Printf("\033[1;33m[!]\033[0m %s\n", msg)
}

func printError(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Printf("\033[0;31m[✗]\033[0m %s\n", msg)
}
