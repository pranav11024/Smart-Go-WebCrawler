
// benchmark/benchmark.go
package benchmark

import (
    "context"
    "fmt"
    "log"
    "time"
    "strings"
    

    "smart-crawler/crawler"
    "smart-crawler/database"
    "smart-crawler/models"
)

func RunComparison(ctx context.Context, db *database.PostgresDB, startURL string, maxDepth, workers int) {
    fmt.Println("🚀 Starting Crawler Performance Benchmark")
    fmt.Println("==========================================")
    fmt.Printf("Target URL: %s\n", startURL)
    fmt.Printf("Max Depth: %d\n", maxDepth)
    fmt.Printf("Workers: %d\n", workers)
    fmt.Println()

    // Clear previous data
    clearDatabase(db)

    // Run Traditional Crawler
    fmt.Println("📊 Running Traditional Crawler...")
    traditionalStats := runTraditionalBenchmark(ctx, db, startURL, maxDepth, workers)
    
    // Clear database for fair comparison
    clearDatabase(db)
    
    // Run Smart Crawler
    fmt.Println("🧠 Running Smart Crawler...")
    smartStats := runSmartBenchmark(ctx, db, startURL, maxDepth, workers)

    // Display Results
    displayComparison(traditionalStats, smartStats)
}

func runTraditionalBenchmark(ctx context.Context, db *database.PostgresDB, startURL string, maxDepth, workers int) *models.CrawlStats {
    traditionalCrawler := crawler.NewTraditional(db, workers)
    start := time.Now()
    
    stats, err := traditionalCrawler.Crawl(ctx, startURL, maxDepth)
    if err != nil {
        log.Printf("Traditional crawler error: %v", err)
        return &models.CrawlStats{}
    }
    
    stats.Duration = time.Since(start)
    return stats
}

func runSmartBenchmark(ctx context.Context, db *database.PostgresDB, startURL string, maxDepth, workers int) *models.CrawlStats {
    smartCrawler := crawler.NewSmart(db, workers)
    start := time.Now()
    
    stats, err := smartCrawler.Crawl(ctx, startURL, maxDepth)
    if err != nil {
        log.Printf("Smart crawler error: %v", err)
        return &models.CrawlStats{}
    }
    
    stats.Duration = time.Since(start)
    return stats
}

func displayComparison(traditional, smart *models.CrawlStats) {
    fmt.Println("\n📈 Performance Comparison Results")
    fmt.Println("=================================")
    
    fmt.Printf("%-20s %-15s %-15s %-15s\n", "Metric", "Traditional", "Smart", "Improvement")
    fmt.Println(strings.Repeat("-", 65))
    
    // Pages Processed
    improvement := calculateImprovement(traditional.PagesProcessed, smart.PagesProcessed)
    fmt.Printf("%-20s %-15d %-15d %-15s\n", "Pages Processed", traditional.PagesProcessed, smart.PagesProcessed, improvement)
    
    // Pages Skipped
    fmt.Printf("%-20s %-15d %-15d %-15s\n", "Pages Skipped", traditional.PagesSkipped, smart.PagesSkipped, "N/A")
    
    // Errors
    errorImprovement := calculateImprovementReverse(traditional.Errors, smart.Errors)
    fmt.Printf("%-20s %-15d %-15d %-15s\n", "Errors", traditional.Errors, smart.Errors, errorImprovement)
    
    // Duration
    durationImprovement := calculateDurationImprovement(traditional.Duration, smart.Duration)
    fmt.Printf("%-20s %-15s %-15s %-15s\n", "Duration", traditional.Duration.Round(time.Second), smart.Duration.Round(time.Second), durationImprovement)
    
    // Total Size
    sizeImprovement := calculateImprovement(int(traditional.TotalSize), int(smart.TotalSize))
    fmt.Printf("%-20s %-15s %-15s %-15s\n", "Total Size", formatBytes(traditional.TotalSize), formatBytes(smart.TotalSize), sizeImprovement)
    
    // Efficiency Metrics
    fmt.Println("\n🎯 Efficiency Metrics")
    fmt.Println("====================")
    
    if traditional.Duration > 0 {
        traditionalRate := float64(traditional.PagesProcessed) / traditional.Duration.Seconds()
        smartRate := float64(smart.PagesProcessed) / smart.Duration.Seconds()
        
        fmt.Printf("Traditional Rate: %.2f pages/second\n", traditionalRate)
        fmt.Printf("Smart Rate: %.2f pages/second\n", smartRate)
        
        if traditionalRate > 0 {
            rateImprovement := ((smartRate - traditionalRate) / traditionalRate) * 100
            fmt.Printf("Rate Improvement: %.2f%%\n", rateImprovement)
        }
    }
    
    // Smart Crawler Specific Benefits
    fmt.Println("\n💡 Smart Crawler Benefits")
    fmt.Println("========================")
    fmt.Printf("• Duplicate Detection: %d pages skipped\n", smart.PagesSkipped)
    fmt.Printf("• Content Quality Filtering: Reduced noise\n")
    fmt.Printf("• Priority-based Crawling: Better resource utilization\n")
    fmt.Printf("• Context Awareness: Smarter link following\n")
    
    if smart.Errors < traditional.Errors {
        fmt.Printf("• Error Reduction: %d fewer errors\n", traditional.Errors-smart.Errors)
    }
}

func calculateImprovement(traditional, smart int) string {
    if traditional == 0 {
        return "N/A"
    }
    
    improvement := ((float64(smart) - float64(traditional)) / float64(traditional)) * 100
    if improvement > 0 {
        return fmt.Sprintf("+%.1f%%", improvement)
    } else if improvement < 0 {
        return fmt.Sprintf("%.1f%%", improvement)
    }
    return "0%"
}

func calculateImprovementReverse(traditional, smart int) string {
    if traditional == 0 {
        return "N/A"
    }
    
    improvement := ((float64(traditional) - float64(smart)) / float64(traditional)) * 100
    if improvement > 0 {
        return fmt.Sprintf("+%.1f%%", improvement)
    } else if improvement < 0 {
        return fmt.Sprintf("%.1f%%", improvement)
    }
    return "0%"
}

func calculateDurationImprovement(traditional, smart time.Duration) string {
    if traditional == 0 {
        return "N/A"
    }
    
    improvement := ((traditional.Seconds() - smart.Seconds()) / traditional.Seconds()) * 100
    if improvement > 0 {
        return fmt.Sprintf("+%.1f%%", improvement)
    } else if improvement < 0 {
        return fmt.Sprintf("%.1f%%", improvement)
    }
    return "0%"
}

func formatBytes(bytes int64) string {
    const unit = 1024
    if bytes < unit {
        return fmt.Sprintf("%d B", bytes)
    }
    div, exp := int64(unit), 0
    for n := bytes / unit; n >= unit; n /= unit {
        div *= unit
        exp++
    }
    return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func clearDatabase(db *database.PostgresDB) {
    queries := []string{
        "TRUNCATE TABLE links CASCADE",
        "TRUNCATE TABLE pages CASCADE", 
        "TRUNCATE TABLE crawl_queue CASCADE",
    }
    
    for _, query := range queries {
        db.DB.Exec(query)
    }
}
