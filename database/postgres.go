package database

import (
    "database/sql"
    "fmt"

    _ "github.com/lib/pq"
    "smart-crawler/models"
)

type PostgresDB struct {
    DB *sql.DB
}

func NewPostgresDB(databaseURL string) (*PostgresDB, error) {
    db, err := sql.Open("postgres", databaseURL)
    if err != nil {
        return nil, fmt.Errorf("failed to open database: %w", err)
    }

    if err := db.Ping(); err != nil {
        return nil, fmt.Errorf("failed to ping database: %w", err)
    }

    pgDB := &PostgresDB{DB: db}
    if err := pgDB.createTables(); err != nil {
        return nil, fmt.Errorf("failed to create tables: %w", err)
    }

    return pgDB, nil
}

func (p *PostgresDB) createTables() error {
    queries := []string{
        `CREATE TABLE IF NOT EXISTS pages (
            id SERIAL PRIMARY KEY,
            url TEXT UNIQUE NOT NULL,
            title TEXT,
            content TEXT,
            status_code INTEGER,
            content_type TEXT,
            size BIGINT,
            load_time_ms BIGINT,
            depth INTEGER,
            parent_url TEXT,
            crawled_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
            hash TEXT,
            importance_score FLOAT DEFAULT 0,
            content_quality FLOAT DEFAULT 0,
            link_density FLOAT DEFAULT 0
        )`,
        `CREATE TABLE IF NOT EXISTS links (
            id SERIAL PRIMARY KEY,
            source_id BIGINT REFERENCES pages(id),
            target_id BIGINT REFERENCES pages(id),
            url TEXT NOT NULL,
            anchor TEXT,
            rel TEXT
        )`,
        `CREATE TABLE IF NOT EXISTS crawl_queue (
            id SERIAL PRIMARY KEY,
            url TEXT UNIQUE NOT NULL,
            priority INTEGER DEFAULT 0,
            depth INTEGER,
            parent_url TEXT,
            scheduled_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
            attempts INTEGER DEFAULT 0,
            last_attempt TIMESTAMP,
            status TEXT DEFAULT 'pending'
        )`,
        `CREATE INDEX IF NOT EXISTS idx_pages_url ON pages(url)`,
        `CREATE INDEX IF NOT EXISTS idx_pages_hash ON pages(hash)`,
        `CREATE INDEX IF NOT EXISTS idx_crawl_queue_priority ON crawl_queue(priority DESC, scheduled_at)`,
        `CREATE INDEX IF NOT EXISTS idx_crawl_queue_status ON crawl_queue(status)`,
    }

    for _, query := range queries {
        if _, err := p.DB.Exec(query); err != nil {
            return fmt.Errorf("failed to execute query %s: %w", query, err)
        }
    }

    return nil
}

func (p *PostgresDB) SavePage(page *models.Page) error {
    query := `
        INSERT INTO pages (url, title, content, status_code, content_type, size, load_time_ms, depth, parent_url, hash, importance_score, content_quality, link_density)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
        ON CONFLICT (url) DO UPDATE SET
            title = EXCLUDED.title,
            content = EXCLUDED.content,
            status_code = EXCLUDED.status_code,
            content_type = EXCLUDED.content_type,
            size = EXCLUDED.size,
            load_time_ms = EXCLUDED.load_time_ms,
            crawled_at = CURRENT_TIMESTAMP,
            hash = EXCLUDED.hash,
            importance_score = EXCLUDED.importance_score,
            content_quality = EXCLUDED.content_quality,
            link_density = EXCLUDED.link_density
        RETURNING id`

    err := p.DB.QueryRow(query,
        page.URL, page.Title, page.Content, page.StatusCode, page.ContentType,
        page.Size, page.LoadTime, page.Depth, page.ParentURL, page.Hash,
        page.Importance, page.ContentQuality, page.LinkDensity, // <-- use directly
    ).Scan(&page.ID)

    return err
}

func (p *PostgresDB) IsURLCrawled(url string) (bool, error) {
    var count int
    err := p.DB.QueryRow("SELECT COUNT(*) FROM pages WHERE url = $1", url).Scan(&count)
    return count > 0, err
}

func (p *PostgresDB) AddToQueue(urls []models.URLPriority) error {
    tx, err := p.DB.Begin()
    if err != nil {
        return err
    }
    defer tx.Rollback()

    stmt, err := tx.Prepare(`
        INSERT INTO crawl_queue (url, priority, depth, parent_url)
        VALUES ($1, $2, $3, $4)
        ON CONFLICT (url) DO UPDATE SET
            priority = GREATEST(crawl_queue.priority, EXCLUDED.priority)
    `)
    if err != nil {
        return err
    }
    defer stmt.Close()

    for _, urlPriority := range urls {
        _, err := stmt.Exec(urlPriority.URL, urlPriority.Priority, urlPriority.Depth, urlPriority.Parent)
        if err != nil {
            return err
        }
    }

    return tx.Commit()
}

func (p *PostgresDB) GetNextURLs(limit int) ([]models.URLPriority, error) {
    query := `
        SELECT url, priority, depth, parent_url
        FROM crawl_queue
        WHERE status = 'pending'
        ORDER BY priority DESC, scheduled_at ASC
        LIMIT $1
    `

    rows, err := p.DB.Query(query, limit)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var urls []models.URLPriority
    for rows.Next() {
        var url models.URLPriority
        err := rows.Scan(&url.URL, &url.Priority, &url.Depth, &url.Parent)
        if err != nil {
            return nil, err
        }
        urls = append(urls, url)
    }

    return urls, nil
}

func (p *PostgresDB) MarkURLProcessed(url string) error {
    _, err := p.DB.Exec("UPDATE crawl_queue SET status = 'completed' WHERE url = $1", url)
    return err
}

func (p *PostgresDB) GetSimilarContent(hash string, threshold float64) ([]models.Page, error) {
    // Simplified similarity check - in production, use more sophisticated algorithms
    query := `SELECT id, url, title, hash FROM pages WHERE hash = $1 LIMIT 5`
    
    rows, err := p.DB.Query(query, hash)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var pages []models.Page
    for rows.Next() {
        var page models.Page
        err := rows.Scan(&page.ID, &page.URL, &page.Title, &page.Hash)
        if err != nil {
            return nil, err
        }
        pages = append(pages, page)
    }

    return pages, nil
}

func (p *PostgresDB) Close() error {
    return p.DB.Close()
}
