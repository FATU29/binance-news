#!/bin/bash

# Model
cat > internal/model/news.go << 'EOF'
package model

import "time"

type News struct {
ID          string    `json:"id"`
Title       string    `json:"title"`
Content     string    `json:"content"`
Summary     string    `json:"summary"`
Author      string    `json:"author"`
Source      string    `json:"source"`
SourceURL   string    `json:"source_url"`
ImageURL    string    `json:"image_url"`
Category    string    `json:"category"`
Tags        []string  `json:"tags"`
PublishedAt time.Time `json:"published_at"`
CrawledAt   time.Time `json:"crawled_at"`
Sentiment   string    `json:"sentiment,omitempty"`
Language    string    `json:"language"`
}

type CrawlJob struct {
ID          string    `json:"id"`
Source      string    `json:"source"`
URL         string    `json:"url"`
Status      string    `json:"status"`
StartedAt   time.Time `json:"started_at,omitempty"`
CompletedAt time.Time `json:"completed_at,omitempty"`
ItemsFound  int       `json:"items_found"`
Error       string    `json:"error,omitempty"`
}

type CrawlSource struct {
Name     string   `json:"name"`
BaseURL  string   `json:"base_url"`
Enabled  bool     `json:"enabled"`
Selectors Selector `json:"selectors"`
}

type Selector struct {
Title       string `json:"title"`
Content     string `json:"content"`
Summary     string `json:"summary"`
Author      string `json:"author"`
PublishedAt string `json:"published_at"`
ImageURL    string `json:"image_url"`
ArticleList string `json:"article_list"`
ArticleLink string `json:"article_link"`
}
EOF

# Config
cat > internal/config/config.go << 'EOF'
package config

import (
"fmt"
"os"
"strconv"
"time"

"github.com/joho/godotenv"
)

type Config struct {
Server   ServerConfig
Crawler  CrawlerConfig
Database DatabaseConfig
}

type ServerConfig struct {
Port         int
Environment  string
ReadTimeout  time.Duration
WriteTimeout time.Duration
}

type CrawlerConfig struct {
Workers           int
RequestTimeout    time.Duration
RateLimitPerMin   int
UserAgent         string
MaxRetries        int
RetryDelay        time.Duration
CrawlInterval     time.Duration
MaxConcurrentJobs int
}

type DatabaseConfig struct {
Host     string
Port     int
User     string
Password string
DBName   string
SSLMode  string
}

func Load() (*Config, error) {
_ = godotenv.Load()

cfg := &Config{
Server: ServerConfig{
Port:         getEnvAsInt("SERVER_PORT", 9000),
Environment:  getEnv("ENVIRONMENT", "development"),
ReadTimeout:  time.Duration(getEnvAsInt("READ_TIMEOUT", 10)) * time.Second,
WriteTimeout: time.Duration(getEnvAsInt("WRITE_TIMEOUT", 10)) * time.Second,
},
Crawler: CrawlerConfig{
Workers:           getEnvAsInt("CRAWLER_WORKERS", 5),
RequestTimeout:    time.Duration(getEnvAsInt("REQUEST_TIMEOUT", 30)) * time.Second,
RateLimitPerMin:   getEnvAsInt("RATE_LIMIT_PER_MIN", 60),
UserAgent:         getEnv("USER_AGENT", "CryptoNewsCrawler/1.0"),
MaxRetries:        getEnvAsInt("MAX_RETRIES", 3),
RetryDelay:        time.Duration(getEnvAsInt("RETRY_DELAY", 2)) * time.Second,
CrawlInterval:     time.Duration(getEnvAsInt("CRAWL_INTERVAL", 300)) * time.Second,
MaxConcurrentJobs: getEnvAsInt("MAX_CONCURRENT_JOBS", 10),
},
Database: DatabaseConfig{
Host:     getEnv("DB_HOST", "localhost"),
Port:     getEnvAsInt("DB_PORT", 5432),
User:     getEnv("DB_USER", "postgres"),
Password: getEnv("DB_PASSWORD", ""),
DBName:   getEnv("DB_NAME", "crypto_news"),
SSLMode:  getEnv("DB_SSL_MODE", "disable"),
},
}

return cfg, nil
}

func getEnv(key, defaultValue string) string {
if value := os.Getenv(key); value != "" {
return value
}
return defaultValue
}

func getEnvAsInt(key string, defaultValue int) int {
valueStr := os.Getenv(key)
if valueStr == "" {
return defaultValue
}

value, err := strconv.Atoi(valueStr)
if err != nil {
fmt.Printf("Warning: Invalid value for %s, using default %d\n", key, defaultValue)
return defaultValue
}

return value
}
EOF

echo "Created model and config files successfully"
