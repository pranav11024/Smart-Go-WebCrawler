package config

import (
    "os"

    "github.com/joho/godotenv"
)

type Config struct {
    DatabaseURL    string
    UserAgent      string
    RequestTimeout int
    RateLimit      int
}

func Load() *Config {
    // Load .env file if it exists
    godotenv.Load()

    return &Config{
        DatabaseURL:    getEnv("DATABASE_URL", "postgres://postgres:password@localhost/smart_crawler?sslmode=disable"),
        UserAgent:      getEnv("USER_AGENT", "SmartCrawler/1.0"),
        RequestTimeout: getEnvInt("REQUEST_TIMEOUT", 30),
        RateLimit:      getEnvInt("RATE_LIMIT", 100),
    }
}

func getEnv(key, defaultVal string) string {
    if val := os.Getenv(key); val != "" {
        return val
    }
    return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
    // Simple implementation - in production, add proper error handling
    return defaultVal
}
