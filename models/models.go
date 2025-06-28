// models/models.go
package models

import (
    "time"
)
type Page struct {
    ID             int64   `json:"id"`
    URL            string  `json:"url"`
    Title          string  `json:"title"`
    Content        string  `json:"content"`
    StatusCode     int     `json:"status_code"`
    ContentType    string  `json:"content_type"`
    Size           int64   `json:"size"`
    LoadTime       int64   `json:"load_time"`
    Depth          int     `json:"depth"`
    ParentURL      string  `json:"parent_url"`
    Hash           string  `json:"hash"`
    Importance     float64 `json:"importance"`
    ContentQuality float64 `json:"content_quality"`
    LinkDensity    float64 `json:"link_density"`
}

type Link struct {
    ID       int64  `json:"id"`
    SourceID int64  `json:"source_id"`
    TargetID int64  `json:"target_id"`
    URL      string `json:"url"`
    Anchor   string `json:"anchor"`
    Rel      string `json:"rel"`
}

type CrawlStats struct {
    PagesProcessed int           `json:"pages_processed"`
    PagesSkipped   int           `json:"pages_skipped"`
    Errors         int           `json:"errors"`
    Duration       time.Duration `json:"duration"`
    AvgLoadTime    time.Duration `json:"avg_load_time"`
    TotalSize      int64         `json:"total_size"`
}

type URLPriority struct {
    URL      string
    Priority int
    Depth    int
    Parent   string
    Context  URLContext
}

type URLContext struct {
    ContentType     string
    Importance      float64
    LastModified    time.Time
    LinkDensity     float64
    ContentQuality  float64
    SimilarityScore float64
}
