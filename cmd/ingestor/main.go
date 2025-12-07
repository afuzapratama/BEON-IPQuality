package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/lfrfrfr/beon-ipquality/internal/config"
	"github.com/lfrfrfr/beon-ipquality/internal/database"
	"github.com/lfrfrfr/beon-ipquality/internal/ingestor"
	pkglogger "github.com/lfrfrfr/beon-ipquality/pkg/logger"
)

func main() {
	// Parse command line flags
	configPath := flag.String("config", "./configs/config.yaml", "Path to configuration file")
	feedsPath := flag.String("feeds", "./configs/feeds.yaml", "Path to feeds configuration file")
	flag.Parse()

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Printf("Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// Load feeds configuration
	feedsCfg, err := config.LoadFeeds(*feedsPath)
	if err != nil {
		fmt.Printf("Failed to load feeds configuration: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	if err := pkglogger.Init(cfg.Logging.Level, cfg.Logging.Format, cfg.Logging.Output, cfg.Logging.FilePath); err != nil {
		fmt.Printf("Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer pkglogger.Sync()

	pkglogger.Info("Starting BEON-IPQuality Ingestor Service")

	// Connect to database
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
		pkglogger.Fatal(fmt.Sprintf("Failed to connect to database: %v", err))
	}
	defer db.Close()

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create ingestor service
	ing, err := ingestor.New(cfg, feedsCfg, db)
	if err != nil {
		pkglogger.Fatal(fmt.Sprintf("Failed to create ingestor: %v", err))
	}

	// Start ingestor
	go func() {
		if err := ing.Start(ctx); err != nil {
			pkglogger.Error(fmt.Sprintf("Ingestor error: %v", err))
		}
	}()

	pkglogger.Info("Ingestor service started")

	// Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	pkglogger.Info("Shutting down ingestor...")
	cancel()

	// Wait for ingestor to stop
	ing.Stop()

	pkglogger.Info("Ingestor stopped gracefully")
}
