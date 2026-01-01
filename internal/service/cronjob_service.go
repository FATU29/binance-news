package service

import (
	"context"
	"fmt"
	"time"

	"crawl-news/internal/model"
	"crawl-news/internal/repository"
	"crawl-news/pkg/logger"

	"github.com/robfig/cron/v3"
)

type CronJobService struct {
	crawlerService *CrawlerService
	newsRepo       *repository.NewsRepository
	cron           *cron.Cron
	enabled        bool
	interval       string // Cron expression, e.g., "0 */1 * * *" for every hour
	lastRunTime    *time.Time
	nextRunTime    *time.Time
}

func NewCronJobService(crawlerService *CrawlerService, newsRepo *repository.NewsRepository) *CronJobService {
	return &CronJobService{
		crawlerService: crawlerService,
		newsRepo:       newsRepo,
		cron:           cron.New(cron.WithSeconds()),
		enabled:        false,
		interval:       "0 */1 * * * *", // Default: every hour
	}
}

// Start starts the cron job service
func (s *CronJobService) Start(ctx context.Context) error {
	if s.enabled {
		return fmt.Errorf("cron job service is already running")
	}

	logger.Info("Starting cron job service with interval: %s", s.interval)

	// Add cron job
	_, err := s.cron.AddFunc(s.interval, func() {
		s.runCrawlJob(ctx)
	})
	if err != nil {
		return fmt.Errorf("failed to add cron job: %w", err)
	}

	s.cron.Start()
	s.enabled = true

	logger.Info("Cron job service started successfully")
	return nil
}

// Stop stops the cron job service
func (s *CronJobService) Stop() {
	if !s.enabled {
		return
	}

	logger.Info("Stopping cron job service")
	s.cron.Stop()
	s.enabled = false
	logger.Info("Cron job service stopped")
}

// SetInterval sets the cron interval
func (s *CronJobService) SetInterval(interval string) error {
	if s.enabled {
		return fmt.Errorf("cannot change interval while cron job is running")
	}

	// Validate cron expression
	_, err := cron.ParseStandard(interval)
	if err != nil {
		return fmt.Errorf("invalid cron expression: %w", err)
	}

	s.interval = interval
	logger.Info("Cron interval updated to: %s", interval)
	return nil
}

// runCrawlJob executes the crawl job
func (s *CronJobService) runCrawlJob(ctx context.Context) {
	logger.Info("Starting scheduled crawl job")

	// Check if we need to crawl (if no recent data)
	shouldCrawl, err := s.shouldCrawl(ctx)
	if err != nil {
		logger.Error("Failed to check if crawl is needed: %v", err)
		return
	}

	if !shouldCrawl {
		logger.Info("Skipping crawl - recent data exists")
		return
	}

	// Get all available sources
	sources := s.getAvailableSources()

	// Crawl all sources
	for _, source := range sources {
		// Check if crawler is already running
		status := s.crawlerService.GetStatus(ctx)
		if isRunning, ok := status["is_running"].(bool); ok && isRunning {
			logger.Warn("Crawler is already running, skipping source: %s", source)
			continue
		}

		logger.Info("Crawling source: %s", source)
		_, err := s.crawlerService.StartCrawl(ctx, source)
		if err != nil {
			logger.Error("Failed to start crawl for source %s: %v", source, err)
			continue
		}

		// Wait a bit between sources to avoid overwhelming
		time.Sleep(5 * time.Second)
	}

	now := time.Now()
	s.lastRunTime = &now

	// Calculate next run time
	entries := s.cron.Entries()
	if len(entries) > 0 {
		next := entries[0].Next
		s.nextRunTime = &next
	}

	logger.Info("Scheduled crawl job completed")
}

// shouldCrawl checks if we should crawl based on recent data
func (s *CronJobService) shouldCrawl(ctx context.Context) (bool, error) {
	// Check if there's any news in the last hour
	oneHourAgo := time.Now().Add(-1 * time.Hour)

	filter := &model.NewsFilter{
		StartDate: &oneHourAgo,
	}

	news, err := s.newsRepo.FindWithFilter(ctx, filter)
	if err != nil {
		return true, err // If error, assume we should crawl
	}

	// If no recent news, we should crawl
	if len(news) == 0 {
		logger.Info("No recent news found, will crawl")
		return true, nil
	}

	// If we have recent news, check if it's enough
	// For now, if we have at least 5 news items in the last hour, skip
	if len(news) >= 5 {
		logger.Info("Found %d recent news items, skipping crawl", len(news))
		return false, nil
	}

	// If we have some but not many, still crawl to get more
	logger.Info("Found %d recent news items, will crawl for more", len(news))
	return true, nil
}

// getAvailableSources returns list of available sources to crawl
func (s *CronJobService) getAvailableSources() []string {
	return []string{
		"cointelegraph",
		"coindesk",
		"cryptonews",
		"binance",
		"coinmarketcap",
		"bitcoincom",
		"theblock",
		"decrypt",
		"utoday",
		"cryptoslate",
	}
}

// GetStatus returns the status of the cron job service
func (s *CronJobService) GetStatus() map[string]interface{} {
	entries := s.cron.Entries()
	nextRun := time.Time{}
	if len(entries) > 0 {
		nextRun = entries[0].Next
	}

	return map[string]interface{}{
		"enabled":    s.enabled,
		"interval":   s.interval,
		"last_run":   s.lastRunTime,
		"next_run":   nextRun,
		"is_running": s.enabled,
	}
}

// TriggerNow manually triggers a crawl job
func (s *CronJobService) TriggerNow(ctx context.Context) error {
	logger.Info("Manually triggering crawl job")
	// Use background context so crawl continues even if request ends
	bgCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	go func() {
		defer cancel()
		s.runCrawlJob(bgCtx)
	}()
	return nil
}
