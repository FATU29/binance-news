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
	Redis    RedisConfig
	AIService AIServiceConfig
	CronJob  CronJobConfig
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

type RedisConfig struct {
	Host     string
	Port     int
	Password string
	DB       int
	CacheTTL time.Duration
}

type AIServiceConfig struct {
	BaseURL     string
	Timeout     time.Duration
	EnableAutoAnalysis bool
	BatchSize   int
}

type CronJobConfig struct {
	Enabled  bool
	Interval string // Cron expression, e.g., "0 */1 * * * *" for every hour
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
		Redis: RedisConfig{
			Host:     getEnv("REDIS_HOST", "localhost"),
			Port:     getEnvAsInt("REDIS_PORT", 6379),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       getEnvAsInt("REDIS_DB", 0),
			CacheTTL: time.Duration(getEnvAsInt("CACHE_TTL", 3600)) * time.Second,
		},
		AIService: AIServiceConfig{
			BaseURL:          getEnv("AI_SERVICE_URL", "http://localhost:8000"),
			Timeout:          time.Duration(getEnvAsInt("AI_SERVICE_TIMEOUT", 30)) * time.Second,
			EnableAutoAnalysis: getEnv("AI_AUTO_ANALYZE", "true") == "true",
			BatchSize:        getEnvAsInt("AI_BATCH_SIZE", 10),
		},
		CronJob: CronJobConfig{
			Enabled:  getEnv("CRONJOB_ENABLED", "true") == "true",
			Interval: getEnv("CRONJOB_INTERVAL", "0 */1 * * * *"), // Default: every hour
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
