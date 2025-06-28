// crawler/traditional.go
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

type Traditional struct {
    db      *database.PostgresDB
    client  *http.Client
    limiter *rate.Limiter
    workers int
}

func NewTraditional(db *database.PostgresDB, workers int) *Traditional {
    return &Traditional{
        db: db,
        client: &http.Client{
            Timeout: 30 * time.Second,
            Transport: &http.Transport{
                MaxIdleConns:        100,
                MaxIdleConnsPerHost: 10,
                IdleConnTimeout:     90 * time.Second,
            },
        },
        limiter: rate.NewLimiter(rate.Limit(10), 20), // 10 requests per second, burst of 20
        workers: workers,
    }
}

func (t *Traditional) Crawl(ctx context.Context, startURL string, maxDepth int) (*models.CrawlStats, error) {
    start := time.Now()
    stats := &models.CrawlStats{}

    // Simple queue implementation
    urlQueue := make(chan models.URLPriority, 1000)
    results := make(chan crawlResult, 100)

    // Start workers
    var wg sync.WaitGroup
    for i := 0; i < t.workers; i++ {
        wg.Add(1)
        go t.worker(ctx, &wg, urlQueue, results)
    }

    // Results processor
    go t.processResults(ctx, results, stats)

    // Add initial URL
    urlQueue <- models.URLPriority{
        URL:   startURL,
        Depth: 0,
    }

    visited := make(map[string]bool)
    visited[startURL] = true

    // Simple BFS crawling
    for depth := 0; depth <= maxDepth; depth++ {
        if ctx.Err() != nil {
            break
        }

        // Process current level
        levelURLs := []string{}
        select {
        case urlPriority := <-urlQueue:
            if urlPriority.Depth == depth {
                levelURLs = append(levelURLs, urlPriority.URL)
            }
        case <-time.After(100 * time.Millisecond):
            // No more URLs at this level
            continue
        }

        for _, currentURL := range levelURLs {
            links, err := t.extractLinks(ctx, currentURL)
            if err != nil {
                stats.Errors++
                continue
            }

            // Add new URLs to queue
            for _, link := range links {
                if !visited[link] {
                    visited[link] = true
                    if utils.IsValidURL(link) {
                        urlQueue <- models.URLPriority{
                            URL:    link,
                            Depth:  depth + 1,
                            Parent: currentURL,
                        }
                    }
                }
            }
        }
    }

    close(urlQueue)
    wg.Wait()
    close(results)

    stats.Duration = time.Since(start)
    return stats, nil
}

func (t *Traditional) worker(ctx context.Context, wg *sync.WaitGroup, urlQueue <-chan models.URLPriority, results chan<- crawlResult) {
    defer wg.Done()

    for urlPriority := range urlQueue {
        if ctx.Err() != nil {
            return
        }

        // Rate limiting
        if err := t.limiter.Wait(ctx); err != nil {
            continue
        }

        result := t.crawlPage(ctx, urlPriority)
        select {
        case results <- result:
        case <-ctx.Done():
            return
        }
    }
}

func (t *Traditional) crawlPage(ctx context.Context, urlPriority models.URLPriority) crawlResult {
    start := time.Now()

    req, err := http.NewRequestWithContext(ctx, "GET", urlPriority.URL, nil)
    if err != nil {
        return crawlResult{Error: err}
    }

    req.Header.Set("User-Agent", "SmartCrawler/1.0")

    resp, err := t.client.Do(req)
    if err != nil {
        return crawlResult{Error: err}
    }
    defer resp.Body.Close()

    body, err := io.ReadAll(resp.Body)
    if err != nil {
        return crawlResult{Error: err}
    }

    doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
    if err != nil {
        return crawlResult{Error: err}
    }

    page := &models.Page{
        URL:         urlPriority.URL,
        Title:       doc.Find("title").Text(),
        Content:     string(body),
        StatusCode:  resp.StatusCode,
        ContentType: resp.Header.Get("Content-Type"),
        Size:        int64(len(body)),
        LoadTime:    time.Since(start).Milliseconds(),
        Depth:       urlPriority.Depth,
        ParentURL:   urlPriority.Parent,
        Hash:        fmt.Sprintf("%x", md5.Sum(body)),
    }

    return crawlResult{Page: page}
}

func (t *Traditional) extractLinks(ctx context.Context, pageURL string) ([]string, error) {
    req, err := http.NewRequestWithContext(ctx, "GET", pageURL, nil)
    if err != nil {
        return nil, err
    }

    resp, err := t.client.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()

    doc, err := goquery.NewDocumentFromReader(resp.Body)
    if err != nil {
        return nil, err
    }

    var links []string
    doc.Find("a[href]").Each(func(i int, s *goquery.Selection) {
        href, exists := s.Attr("href")
        if exists {
            if absoluteURL := t.makeAbsoluteURL(pageURL, href); absoluteURL != "" {
                links = append(links, absoluteURL)
            }
        }
    })

    return links, nil
}

func (t *Traditional) makeAbsoluteURL(baseURL, href string) string {
    base, err := url.Parse(baseURL)
    if err != nil {
        return ""
    }

    link, err := url.Parse(href)
    if err != nil {
        return ""
    }

    return base.ResolveReference(link).String()
}

func (t *Traditional) processResults(ctx context.Context, results <-chan crawlResult, stats *models.CrawlStats) {
    for result := range results {
        if result.Error != nil {
            stats.Errors++
            continue
        }

        if err := t.db.SavePage(result.Page); err != nil {
            stats.Errors++
            continue
        }

        stats.PagesProcessed++
        stats.TotalSize += result.Page.Size
    }
}

type crawlResult struct {
    Page  *models.Page
    Error error
}
