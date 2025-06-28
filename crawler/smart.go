package crawler

import (
    "context"
    "crypto/md5"
    "fmt"
    "io"
    "net/http"
    "net/url"
    "strings"
    "sync"
    "time"

    "github.com/PuerkitoBio/goquery"
    "golang.org/x/time/rate"

    "smart-crawler/database"
    "smart-crawler/models"
    "smart-crawler/utils"
)

type Smart struct {
    db               *database.PostgresDB
    client           *http.Client
    limiter          *rate.Limiter
    workers          int
    contentAnalyzer  *ContentAnalyzer
    duplicateDetector *DuplicateDetector
}

func NewSmart(db *database.PostgresDB, workers int) *Smart {
    return &Smart{
        db: db,
        client: &http.Client{
            Timeout: 30 * time.Second,
            Transport: &http.Transport{
                MaxIdleConns:        100,
                MaxIdleConnsPerHost: 10,
                IdleConnTimeout:     90 * time.Second,
            },
        },
        limiter:           rate.NewLimiter(rate.Limit(15), 30), // Higher rate for smart crawler
        workers:           workers,
        contentAnalyzer:   NewContentAnalyzer(),
        duplicateDetector: NewDuplicateDetector(),
    }
}

func (s *Smart) Crawl(ctx context.Context, startURL string, maxDepth int) (*models.CrawlStats, error) {
    start := time.Now()
    stats := &models.CrawlStats{}

    // Priority queue implementation
    urlQueue := make(chan models.URLPriority, 1000)
    results := make(chan smartCrawlResult, 100)

    // Start workers
    var wg sync.WaitGroup
    for i := 0; i < s.workers; i++ {
        wg.Add(1)
        go s.smartWorker(ctx, &wg, urlQueue, results)
    }

    // Results processor
    go s.processSmartResults(ctx, results, stats, urlQueue)

    // Add initial URL with high priority
    initialURL := models.URLPriority{
        URL:      startURL,
        Priority: 100,
        Depth:    0,
        Context: models.URLContext{
            Importance: 1.0,
        },
    }

    urlQueue <- initialURL
    s.db.AddToQueue([]models.URLPriority{initialURL})

    // Smart crawling with adaptive depth and priority
    ticker := time.NewTicker(500 * time.Millisecond)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            close(urlQueue)
            wg.Wait()
            close(results)
            stats.Duration = time.Since(start)
            return stats, nil
        case <-ticker.C:
            // Get next batch of URLs from database
            nextURLs, err := s.db.GetNextURLs(s.workers * 2)
            if err != nil {
                continue
            }

            if len(nextURLs) == 0 {
                // No more URLs to process
                time.Sleep(1 * time.Second)
                continue
            }

            for _, urlPriority := range nextURLs {
                if urlPriority.Depth <= maxDepth {
                    select {
                    case urlQueue <- urlPriority:
                    case <-ctx.Done():
                        close(urlQueue)
                        wg.Wait()
                        close(results)
                        stats.Duration = time.Since(start)
                        return stats, nil
                    }
                }
            }
        }
    }
}

func (s *Smart) smartWorker(ctx context.Context, wg *sync.WaitGroup, urlQueue <-chan models.URLPriority, results chan<- smartCrawlResult) {
    defer wg.Done()

    for urlPriority := range urlQueue {
        if ctx.Err() != nil {
            return
        }

        // Advanced rate limiting based on priority
        if err := s.limiter.Wait(ctx); err != nil {
            continue
        }

        result := s.smartCrawlPage(ctx, urlPriority)
        select {
        case results <- result:
        case <-ctx.Done():
            return
        }

        s.db.MarkURLProcessed(urlPriority.URL)
    }
}

func (s *Smart) smartCrawlPage(ctx context.Context, urlPriority models.URLPriority) smartCrawlResult {
    start := time.Now()

    // Check if URL is already crawled
    crawled, err := s.db.IsURLCrawled(urlPriority.URL)
    if err == nil && crawled {
        return smartCrawlResult{Skipped: true, Reason: "already_crawled"}
    }

    req, err := http.NewRequestWithContext(ctx, "GET", urlPriority.URL, nil)
    if err != nil {
        return smartCrawlResult{Error: err}
    }

    req.Header.Set("User-Agent", "SmartCrawler/1.0")
    req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

    resp, err := s.client.Do(req)
    if err != nil {
        return smartCrawlResult{Error: err}
    }
    defer resp.Body.Close()

    // Smart content type filtering
    contentType := resp.Header.Get("Content-Type")
    if !s.isRelevantContent(contentType) {
        return smartCrawlResult{Skipped: true, Reason: "irrelevant_content_type"}
    }

    body, err := io.ReadAll(resp.Body)
    if err != nil {
        return smartCrawlResult{Error: err}
    }

    // Duplicate detection
    hash := fmt.Sprintf("%x", md5.Sum(body))
    if s.duplicateDetector.IsDuplicate(hash) {
        return smartCrawlResult{Skipped: true, Reason: "duplicate_content"}
    }

    doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
    if err != nil {
        return smartCrawlResult{Error: err}
    }

    // Content analysis
    context := s.contentAnalyzer.AnalyzeContent(doc, string(body))
    context.LastModified = time.Now()

    page := &models.Page{
        URL:         urlPriority.URL,
        Title:       doc.Find("title").Text(),
        Content:     string(body),
        StatusCode:  resp.StatusCode,
        ContentType: contentType,
        Size:        int64(len(body)),
        LoadTime:    time.Since(start).Milliseconds(),
        Depth:       urlPriority.Depth,
        ParentURL:   urlPriority.Parent,
        Hash:        hash,
     }

    // Extract links with smart prioritization
    links := s.extractSmartLinks(doc, urlPriority.URL, context, urlPriority.Depth)

    return smartCrawlResult{
        Page:  page,
        Links: links,
    }
}

func (s *Smart) extractSmartLinks(doc *goquery.Document, baseURL string, pageContext models.URLContext, parentDepth int) []models.URLPriority {
    var links []models.URLPriority

    doc.Find("a[href]").Each(func(i int, sel *goquery.Selection) {
        href, exists := sel.Attr("href")
        if !exists {
            return
        }

        absoluteURL := s.makeAbsoluteURL(baseURL, href)
        if absoluteURL == "" || !utils.IsValidURL(absoluteURL) {
            return
        }

        // Smart link prioritization
        priority := s.calculateLinkPriority(sel, pageContext)
        
        linkContext := models.URLContext{
            Importance:     float64(priority) / 100.0,
            ContentType:    s.guessContentType(absoluteURL),
            LinkDensity:    pageContext.LinkDensity,
        }

        links = append(links, models.URLPriority{
            URL:      absoluteURL,
            Priority: priority,
            Depth:    parentDepth + 1,
            Parent:   baseURL,
            Context:  linkContext,
        })
    })

    return links
}

func (s *Smart) calculateLinkPriority(sel *goquery.Selection, pageContext models.URLContext) int {
    priority := 50 // Base priority

    // Analyze anchor text
    anchorText := strings.TrimSpace(sel.Text())
    
    // High priority keywords
    highPriorityKeywords := []string{"article", "news", "blog", "content", "post", "story", "research", "documentation"}
    for _, keyword := range highPriorityKeywords {
        if strings.Contains(strings.ToLower(anchorText), keyword) {
            priority += 20
            break
        }
    }

    // Low priority keywords (navigation, etc.)
    lowPriorityKeywords := []string{"login", "register", "contact", "about", "terms", "privacy", "sitemap"}
    for _, keyword := range lowPriorityKeywords {
        if strings.Contains(strings.ToLower(anchorText), keyword) {
            priority -= 15
            break
        }
    }

    // Check rel attribute
    if rel, exists := sel.Attr("rel"); exists {
        if strings.Contains(rel, "nofollow") {
            priority -= 30
        }
    }

    // Check class attribute for semantic hints
    if class, exists := sel.Attr("class"); exists {
        if strings.Contains(class, "nav") || strings.Contains(class, "menu") {
            priority -= 10
        }
        if strings.Contains(class, "content") || strings.Contains(class, "article") {
            priority += 15
        }
    }

    // Boost priority based on page importance
    priority += int(pageContext.Importance * 20)

    // Ensure priority is within bounds
    if priority < 1 {
        priority = 1
    }
    if priority > 100 {
        priority = 100
    }

    return priority
}

func (s *Smart) isRelevantContent(contentType string) bool {
    relevantTypes := []string{
        "text/html",
        "application/xhtml+xml",
        "text/plain",
    }

    for _, relevantType := range relevantTypes {
        if strings.Contains(contentType, relevantType) {
            return true
        }
    }
    return false
}

func (s *Smart) guessContentType(url string) string {
    lower := strings.ToLower(url)
    
    if strings.Contains(lower, "/blog/") || strings.Contains(lower, "/article/") {
        return "article"
    }
    if strings.Contains(lower, "/news/") {
        return "news"
    }
    if strings.Contains(lower, "/doc") || strings.Contains(lower, "/help/") {
        return "documentation"
    }
    
    return "general"
}

func (s *Smart) makeAbsoluteURL(baseURL, href string) string {
    base, err := url.Parse(baseURL)
    if err != nil {
        return ""
    }

    link, err := url.Parse(href)
    if err != nil {
        return ""
    }

    resolved := base.ResolveReference(link)
    
    // Filter out unwanted URLs
    if resolved.Fragment != "" && resolved.RawQuery == "" && resolved.Path == base.Path {
        return "" // Skip anchor-only links on same page
    }

    return resolved.String()
}

func (s *Smart) processSmartResults(ctx context.Context, results <-chan smartCrawlResult, stats *models.CrawlStats, urlQueue chan<- models.URLPriority) {
    for result := range results {
        if result.Error != nil {
            stats.Errors++
            continue
        }

        if result.Skipped {
            stats.PagesSkipped++
            continue
        }

        if err := s.db.SavePage(result.Page); err != nil {
            stats.Errors++
            continue
        }

        // Add discovered links to queue
        if len(result.Links) > 0 {
            if err := s.db.AddToQueue(result.Links); err != nil {
                // Log error but continue
            }
        }

        stats.PagesProcessed++
        stats.TotalSize += result.Page.Size
        
        if stats.PagesProcessed > 0 {
            stats.AvgLoadTime = time.Duration(stats.TotalSize/int64(stats.PagesProcessed)) * time.Millisecond
        }
    }
}

type smartCrawlResult struct {
    Page    *models.Page
    Links   []models.URLPriority
    Skipped bool
    Reason  string
    Error   error
}

// Content Analyzer
type ContentAnalyzer struct {
    stopWords map[string]bool
}

func NewContentAnalyzer() *ContentAnalyzer {
    stopWords := map[string]bool{
        "a": true, "an": true, "and": true, "are": true, "as": true, "at": true, "be": true, "by": true,
        "for": true, "from": true, "has": true, "he": true, "in": true, "is": true, "it": true, "its": true,
        "of": true, "on": true, "that": true, "the": true, "to": true, "was": true, "will": true, "with": true,
    }

    return &ContentAnalyzer{stopWords: stopWords}
}

func (ca *ContentAnalyzer) AnalyzeContent(doc *goquery.Document, content string) models.URLContext {
    context := models.URLContext{}

    // Calculate content quality based on various factors
    context.ContentQuality = ca.calculateContentQuality(doc, content)
    
    // Calculate link density
    context.LinkDensity = ca.calculateLinkDensity(doc)
    
    // Calculate importance score
    context.Importance = ca.calculateImportance(doc, content)

    return context
}

func (ca *ContentAnalyzer) calculateContentQuality(doc *goquery.Document, content string) float64 {
    score := 0.0

    // Text length factor
    textLength := len(strings.TrimSpace(doc.Find("body").Text()))
    if textLength > 500 {
        score += 0.3
    }
    if textLength > 2000 {
        score += 0.2
    }

    // Presence of structured content
    if doc.Find("h1, h2, h3").Length() > 0 {
        score += 0.2
    }

    // Presence of paragraphs
    if doc.Find("p").Length() > 3 {
        score += 0.2
    }

    // Meta description
    if doc.Find("meta[name='description']").Length() > 0 {
        score += 0.1
    }

    // Ensure score is between 0 and 1
    if score > 1.0 {
        score = 1.0
    }

    return score
}

func (ca *ContentAnalyzer) calculateLinkDensity(doc *goquery.Document) float64 {
    textLength := len(doc.Find("body").Text())
    linkTextLength := len(doc.Find("a").Text())

    if textLength == 0 {
        return 0.0
    }

    density := float64(linkTextLength) / float64(textLength)
    if density > 1.0 {
        density = 1.0
    }

    return density
}

func (ca *ContentAnalyzer) calculateImportance(doc *goquery.Document, content string) float64 {
    importance := 0.5 // Base importance

    // Title analysis
    title := doc.Find("title").Text()
    if len(title) > 10 && len(title) < 70 {
        importance += 0.1
    }

    // Content depth indicators
    if doc.Find("article").Length() > 0 {
        importance += 0.2
    }

    // Navigation breadcrumbs suggest deeper content
    if doc.Find("nav, .breadcrumb").Length() > 0 {
        importance += 0.1
    }

    // Social sharing buttons suggest valuable content
    if doc.Find("[class*='share'], [class*='social']").Length() > 0 {
        importance += 0.1
    }

    // Ensure importance is between 0 and 1
    if importance > 1.0 {
        importance = 1.0
    }

    return importance
}

// Duplicate Detector
type DuplicateDetector struct {
    seenHashes map[string]bool
    mutex      sync.RWMutex
}

func NewDuplicateDetector() *DuplicateDetector {
    return &DuplicateDetector{
        seenHashes: make(map[string]bool),
    }
}

func (dd *DuplicateDetector) IsDuplicate(hash string) bool {
    dd.mutex.RLock()
    defer dd.mutex.RUnlock()
    
    if dd.seenHashes[hash] {
        return true
    }
    
    dd.mutex.RUnlock()
    dd.mutex.Lock()
    dd.seenHashes[hash] = true
    dd.mutex.Unlock()
    dd.mutex.RLock()
    
    return false
}
