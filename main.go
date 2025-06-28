package main

import (
    "context"
    "flag"
    "log"
    "os"
    "os/signal"
    "syscall"
    "time"

    "smart-crawler/benchmark"
    "smart-crawler/config"
    "smart-crawler/crawler"
    "smart-crawler/database"
)

func main() {
    // Command line flags
    var (
        mode = flag.String("mode", "smart", "Crawler mode: 'traditional', 'smart', or 'benchmark'")
        url  = flag.String("url", "https://example.com", "Starting URL to crawl")
        depth = flag.Int("depth", 3, "Maximum crawl depth")
        workers = flag.Int("workers", 10, "Number of concurrent workers")
    )
    flag.Parse()

    // Load configuration
    cfg := config.Load()
    
    // Initialize database
    db, err := database.NewPostgresDB(cfg.DatabaseURL)
    if err != nil {
        log.Fatalf("Failed to connect to database: %v", err)
    }
    defer db.Close()

    // Setup graceful shutdown
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    go func() {
        sigChan := make(chan os.Signal, 1)
        signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
        <-sigChan
        log.Println("Shutting down gracefully...")
        cancel()
    }()

    switch *mode {
    case "traditional":
        runTraditionalCrawler(ctx, db, *url, *depth, *workers)
    case "smart":
        runSmartCrawler(ctx, db, *url, *depth, *workers)
    case "benchmark":
        benchmark.RunComparison(ctx, db, *url, *depth, *workers)
    default:
        log.Fatalf("Invalid mode: %s. Use 'traditional', 'smart', or 'benchmark'", *mode)
    }
}

func runTraditionalCrawler(ctx context.Context, db *database.PostgresDB, startURL string, maxDepth, workers int) {
    log.Printf("Starting traditional crawler on %s with depth %d and %d workers", startURL, maxDepth, workers)
    
    traditionalCrawler := crawler.NewTraditional(db, workers)
    start := time.Now()
    
    stats, err := traditionalCrawler.Crawl(ctx, startURL, maxDepth)
    if err != nil {
        log.Fatalf("Traditional crawler failed: %v", err)
    }
    
    duration := time.Since(start)
    log.Printf("Traditional crawler completed in %v", duration)
    log.Printf("Stats: %+v", stats)
}

func runSmartCrawler(ctx context.Context, db *database.PostgresDB, startURL string, maxDepth, workers int) {
    log.Printf("Starting smart crawler on %s with depth %d and %d workers", startURL, maxDepth, workers)
    
    smartCrawler := crawler.NewSmart(db, workers)
    start := time.Now()
    
    stats, err := smartCrawler.Crawl(ctx, startURL, maxDepth)
    if err != nil {
        log.Fatalf("Smart crawler failed: %v", err)
    }
    
    duration := time.Since(start)
    log.Printf("Smart crawler completed in %v", duration)
    log.Printf("Stats: %+v", stats)
}
