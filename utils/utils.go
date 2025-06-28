package utils

import (
    "net/url"
    "strings"
)

func IsValidURL(rawURL string) bool {
    if rawURL == "" {
        return false
    }

    u, err := url.Parse(rawURL)
    if err != nil {
        return false
    }

    // Must have scheme and host
    if u.Scheme == "" || u.Host == "" {
        return false
    }

    // Only allow HTTP and HTTPS
    if u.Scheme != "http" && u.Scheme != "https" {
        return false
    }

    // Filter out common non-content URLs
    excludePatterns := []string{
        ".css", ".js", ".png", ".jpg", ".jpeg", ".gif", ".svg", ".ico",
        ".pdf", ".zip", ".exe", ".dmg", "mailto:", "tel:",
    }

    lowerURL := strings.ToLower(rawURL)
    for _, pattern := range excludePatterns {
        if strings.Contains(lowerURL, pattern) {
            return false
        }
    }

    return true
}

func NormalizeURL(rawURL string) string {
    u, err := url.Parse(rawURL)
    if err != nil {
        return rawURL
    }

    // Remove fragment
    u.Fragment = ""
    
    // Normalize path
    if u.Path == "" {
        u.Path = "/"
    }

    return u.String()
}
