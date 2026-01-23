package service

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"crawl-news/internal/crawler"
	"crawl-news/internal/model"
	"crawl-news/internal/repository"
	"crawl-news/pkg/logger"
)

type CrawlerService struct {
	newsRepo       *repository.NewsRepository
	crawler        *crawler.Crawler
	qualityService *ContentQualityService
	aiService      *AIService
	aiHTMLParser   *AIHTMLParser
	isRunning      bool
	currentJobID   string
	totalCrawled   int
	filteredCount  int
	mu             sync.RWMutex
	stopChan       chan struct{}
}

func NewCrawlerService(newsRepo *repository.NewsRepository) *CrawlerService {
	return &CrawlerService{
		newsRepo:       newsRepo,
		crawler:        crawler.NewCrawler(),
		qualityService: NewContentQualityService(),
		aiService:      nil, // Will be set via SetAIService
		aiHTMLParser:   nil, // Will be set via SetAIHTMLParser
		isRunning:      false,
		totalCrawled:   0,
		filteredCount:  0,
		stopChan:       make(chan struct{}),
	}
}

// SetAIService sets the AI service for auto-analysis
func (s *CrawlerService) SetAIService(aiService *AIService) {
	s.aiService = aiService
}

// SetAIHTMLParser sets the AI HTML parser for fallback parsing
func (s *CrawlerService) SetAIHTMLParser(parser *AIHTMLParser) {
	s.aiHTMLParser = parser
	if parser != nil {
		s.crawler.SetAIParser(parser)
		logger.Info("AI HTML parser enabled for crawler fallback")
	}
}

func (s *CrawlerService) StartCrawl(ctx context.Context, source string) (string, error) {
	return s.StartCrawlWithOptions(ctx, source, CrawlOptions{})
}

// CrawlOptions configures crawl behavior
type CrawlOptions struct {
	OnlyNew      bool          // Only crawl news newer than last crawl
	MinAge       time.Duration // Minimum age of news to crawl (e.g., only crawl news from last 24h)
	ForceRefresh bool          // Force crawl even if news exists
}

func (s *CrawlerService) StartCrawlWithOptions(ctx context.Context, source string, options CrawlOptions) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.isRunning {
		return "", fmt.Errorf("crawler is already running")
	}

	jobID := fmt.Sprintf("job-%d-%d", time.Now().Unix(), len(source))
	s.currentJobID = jobID
	s.isRunning = true

	// Create background context that won't be cancelled when request ends
	// Use context.Background() with timeout instead of request context
	bgCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// Start crawling in a goroutine
	go func() {
		defer func() {
			s.mu.Lock()
			s.isRunning = false
			s.mu.Unlock()
			cancel() // Clean up context
		}()

		logger.Info("Starting crawl job %s for source: %s (options: onlyNew=%v, minAge=%v, forceRefresh=%v)",
			jobID, source, options.OnlyNew, options.MinAge, options.ForceRefresh)

		// Create crawl job
		job := &model.CrawlJob{
			ID:     jobID,
			Source: source,
			Status: "running",
		}

		// Execute crawler with background context
		results, err := s.crawler.Crawl(bgCtx, source)
		if err != nil {
			logger.Error("Crawl job %s failed: %v", jobID, err)
			job.Status = "failed"
			job.Error = err.Error()
			return
		}

		if len(results) == 0 {
			logger.Warn("Crawl job %s completed but found 0 results", jobID)
			job.Status = "completed"
			job.ItemsFound = 0
			return
		}

		// Filter results by quality and process
		savedCount := 0
		filteredCount := 0
		analyzedCount := 0
		errorCount := 0

		logger.Info("Processing %d crawled results for job %s", len(results), jobID)

		// Get cutoff time for filtering old news
		var cutoffTime time.Time
		if options.MinAge > 0 {
			cutoffTime = time.Now().Add(-options.MinAge)
			logger.Info("Filtering news older than: %s", cutoffTime.Format(time.RFC3339))
		} else if options.OnlyNew {
			// Get last crawl time for this source
			cutoffTime = s.getLastCrawlTimeForSource(bgCtx, source)
			if cutoffTime.IsZero() {
				// If no previous crawl, use last 24 hours
				cutoffTime = time.Now().Add(-24 * time.Hour)
			}
			logger.Info("Only crawling news newer than last crawl: %s", cutoffTime.Format(time.RFC3339))
		}

		for i, news := range results {
			// Ensure news has required fields
			if news.Title == "" {
				logger.Warn("Skipping news item %d: empty title", i)
				filteredCount++
				continue
			}

			// Ensure SourceURL is set (required for unique constraint)
			if news.SourceURL == "" {
				logger.Warn("Skipping news item %d: empty source_url (title: %s)", i, news.Title)
				filteredCount++
				continue
			}

			// Filter by age if specified
			if !cutoffTime.IsZero() && !news.PublishedAt.IsZero() {
				if news.PublishedAt.Before(cutoffTime) {
					logger.Debug("Skipping old news: %s (published: %s, cutoff: %s)",
						news.Title, news.PublishedAt.Format(time.RFC3339), cutoffTime.Format(time.RFC3339))
					filteredCount++
					continue
				}
			}

			// Generate ID if not set
			if news.ID == "" {
				hash := md5.Sum([]byte(news.SourceURL))
				news.ID = hex.EncodeToString(hash[:])
			}

			// Set timestamps if not set
			now := time.Now()
			if news.CrawledAt.IsZero() {
				news.CrawledAt = now
			}
			if news.PublishedAt.IsZero() {
				news.PublishedAt = now
			}
			if news.CreatedAt.IsZero() {
				news.CreatedAt = now
			}
			if news.UpdatedAt.IsZero() {
				news.UpdatedAt = now
			}

			// Ensure Source is set
			if news.Source == "" {
				news.Source = source
			}

			quality := s.qualityService.AssessQuality(news)

			if !quality.ShouldShow {
				filteredCount++
				logger.Debug("Filtered out low quality news: %s (score: %.2f)", news.Title, quality.Score)
				continue
			}

			// Extract trading pairs before saving
			if s.aiService != nil {
				news.RelatedPairs = s.aiService.extractTradingPairs(news)
			}

			// Note: Create() will handle checking if news exists and update if needed
			// If ForceRefresh is false, Create() will update existing news
			// We don't need to check here since Create() already does it efficiently

			// Save high quality news with background context
			// Create method will handle update if news already exists
			if err := s.newsRepo.Create(bgCtx, news); err != nil {
				errorCount++
				logger.Error("Failed to save news '%s' (ID: %s, URL: %s): %v", news.Title, news.ID, news.SourceURL, err)
				// Continue with next news instead of stopping
			} else {
				savedCount++
				logger.Info("✅ Successfully saved/updated news: %s (ID: %s, URL: %s)", news.Title, news.ID, news.SourceURL)

				// Auto-analyze sentiment if AI service is available and enabled
				if s.aiService != nil && s.aiService.cfg != nil && s.aiService.cfg.EnableAutoAnalysis {
					go func(newsID string) {
						// Use background context for AI analysis too
						_, err := s.aiService.AnalyzeSentiment(bgCtx, newsID)
						if err != nil {
							logger.Warn("Failed to auto-analyze sentiment for news %s: %v", newsID, err)
						} else {
							analyzedCount++
							logger.Debug("Auto-analyzed sentiment for news: %s", newsID)
						}
					}(news.ID)
				}
			}
		}

		// Update total crawled count
		s.mu.Lock()
		s.totalCrawled += savedCount
		s.filteredCount += filteredCount
		s.mu.Unlock()

		job.Status = "completed"
		job.ItemsFound = len(results)

		// Log detailed summary
		logger.Info("═══════════════════════════════════════════════════════════")
		logger.Info("Crawl job %s COMPLETED for source: %s", jobID, source)
		logger.Info("  📊 Found:        %d items", len(results))
		logger.Info("  ✅ Saved/Updated: %d items", savedCount)
		logger.Info("  🚫 Filtered:     %d items (low quality)", filteredCount)
		logger.Info("  ❌ Errors:       %d items", errorCount)
		logger.Info("  🤖 Analyzed:     %d items", analyzedCount)
		logger.Info("═══════════════════════════════════════════════════════════")

		// Force invalidate all caches to ensure fresh data
		if savedCount > 0 {
			// Invalidate all list caches
			s.newsRepo.InvalidateListCache(bgCtx)
			logger.Info("Cache invalidated - %d news items saved/updated", savedCount)
		}
	}()

	return jobID, nil
}

// getLastCrawlTimeForSource gets the most recent crawl time for a source
func (s *CrawlerService) getLastCrawlTimeForSource(ctx context.Context, source string) time.Time {
	// Use newsRepo to query DB
	var lastNews model.News
	err := s.newsRepo.GetDB().Where("source = ?", source).
		Order("crawled_at DESC").
		First(&lastNews).Error

	if err != nil {
		return time.Time{} // Return zero time if no previous crawl
	}

	return lastNews.CrawledAt
}

// StartCrawlMultipleSources crawls multiple sources in parallel
func (s *CrawlerService) StartCrawlMultipleSources(ctx context.Context, sources []string, options CrawlOptions) ([]string, error) {
	jobIDs := make([]string, 0, len(sources))
	var wg sync.WaitGroup
	var mu sync.Mutex

	// Start all crawls in parallel
	for _, source := range sources {
		wg.Add(1)
		go func(src string) {
			defer wg.Done()
			jobID, err := s.startCrawlJob(ctx, src, options)
			if err != nil {
				logger.Error("Failed to start crawl for source %s: %v", src, err)
				return
			}
			mu.Lock()
			jobIDs = append(jobIDs, jobID)
			mu.Unlock()
		}(source)
	}

	// Wait for all crawls to start (not to complete)
	wg.Wait()

	logger.Info("Started %d crawl jobs for %d sources", len(jobIDs), len(sources))
	return jobIDs, nil
}

// startCrawlJob starts a crawl job without checking isRunning (for parallel crawls)
func (s *CrawlerService) startCrawlJob(ctx context.Context, source string, options CrawlOptions) (string, error) {
	jobID := fmt.Sprintf("job-%d-%s-%d", time.Now().Unix(), source, len(source))

	// Create background context that won't be cancelled when request ends
	bgCtx, cancel := context.WithTimeout(context.Background(), 15*time.Minute) // Increased timeout for AI parsing

	// Start crawling in a goroutine
	go func() {
		defer cancel()

		logger.Info("Starting crawl job %s for source: %s (options: onlyNew=%v, minAge=%v, forceRefresh=%v)",
			jobID, source, options.OnlyNew, options.MinAge, options.ForceRefresh)

		// Create crawl job
		job := &model.CrawlJob{
			ID:     jobID,
			Source: source,
			Status: "running",
		}

		// Execute crawler with background context
		results, err := s.crawler.Crawl(bgCtx, source)
		if err != nil {
			logger.Error("Crawl job %s failed: %v", jobID, err)
			job.Status = "failed"
			job.Error = err.Error()
			return
		}

		if len(results) == 0 {
			logger.Warn("Crawl job %s completed but found 0 results", jobID)
			job.Status = "completed"
			job.ItemsFound = 0
			return
		}

		// Filter results by quality and process
		savedCount := 0
		filteredCount := 0
		analyzedCount := 0
		errorCount := 0

		logger.Info("Processing %d crawled results for job %s", len(results), jobID)

		// Get cutoff time for filtering old news
		var cutoffTime time.Time
		if options.MinAge > 0 {
			cutoffTime = time.Now().Add(-options.MinAge)
		}

		// Process each result
		for i, news := range results {
			// Check if we should stop
			select {
			case <-s.stopChan:
				logger.Info("Crawl job %s stopped by user", jobID)
				return
			default:
			}

			// Basic validation
			if news == nil {
				logger.Warn("Skipping nil news item at index %d", i)
				filteredCount++
				continue
			}

			if news.Title == "" {
				logger.Warn("Skipping news item %d: empty title", i)
				filteredCount++
				continue
			}

			// Ensure SourceURL is set (required for unique constraint)
			if news.SourceURL == "" {
				logger.Warn("Skipping news item %d: empty source_url (title: %s)", i, news.Title)
				filteredCount++
				continue
			}

			// Filter by age if specified
			if !cutoffTime.IsZero() && !news.PublishedAt.IsZero() {
				if news.PublishedAt.Before(cutoffTime) {
					logger.Debug("Skipping old news: %s (published: %s, cutoff: %s)",
						news.Title, news.PublishedAt.Format(time.RFC3339), cutoffTime.Format(time.RFC3339))
					filteredCount++
					continue
				}
			}

			// Generate ID if not set
			if news.ID == "" {
				hash := md5.Sum([]byte(news.SourceURL))
				news.ID = hex.EncodeToString(hash[:])
			}

			// Set timestamps if not set
			now := time.Now()
			if news.CrawledAt.IsZero() {
				news.CrawledAt = now
			}
			if news.PublishedAt.IsZero() {
				news.PublishedAt = now
			}
			if news.CreatedAt.IsZero() {
				news.CreatedAt = now
			}
			if news.UpdatedAt.IsZero() {
				news.UpdatedAt = now
			}

			// Ensure Source is set
			if news.Source == "" {
				news.Source = source
			}

			quality := s.qualityService.AssessQuality(news)

			if !quality.ShouldShow {
				filteredCount++
				logger.Debug("Filtered low quality news: %s (score: %.2f)", news.Title, quality.Score)
				continue
			}

			// Extract trading pairs before saving
			if s.aiService != nil {
				news.RelatedPairs = s.aiService.extractTradingPairs(news)
			}

			// Save high quality news with background context
			// Create method will handle update if news already exists
			if err := s.newsRepo.Create(bgCtx, news); err != nil {
				errorCount++
				logger.Error("Failed to save news '%s' (ID: %s, URL: %s): %v", news.Title, news.ID, news.SourceURL, err)
				// Continue with next news instead of stopping
			} else {
				savedCount++
				logger.Info("✅ Successfully saved/updated news: %s (ID: %s, URL: %s)", news.Title, news.ID, news.SourceURL)

				// Auto-analyze sentiment if AI service is available and enabled
				if s.aiService != nil && s.aiService.cfg != nil && s.aiService.cfg.EnableAutoAnalysis {
					go func(newsID string) {
						// Use background context for AI analysis too
						_, err := s.aiService.AnalyzeSentiment(bgCtx, newsID)
						if err != nil {
							logger.Warn("Failed to auto-analyze sentiment for news %s: %v", newsID, err)
						} else {
							analyzedCount++
							logger.Debug("Auto-analyzed sentiment for news: %s", newsID)
						}
					}(news.ID)
				}
			}
		}

		job.Status = "completed"
		job.ItemsFound = len(results)
		job.CompletedAt = time.Now()

		// Update statistics
		s.mu.Lock()
		s.totalCrawled += savedCount
		s.filteredCount += filteredCount
		s.mu.Unlock()

		// Log detailed summary
		logger.Info("═══════════════════════════════════════════════════════════")
		logger.Info("Crawl job %s COMPLETED for source: %s", jobID, source)
		logger.Info("  📊 Found:        %d items", len(results))
		logger.Info("  ✅ Saved/Updated: %d items", savedCount)
		logger.Info("  🚫 Filtered:     %d items (low quality)", filteredCount)
		logger.Info("  ❌ Errors:       %d items", errorCount)
		logger.Info("  🤖 Analyzed:     %d items", analyzedCount)
		logger.Info("═══════════════════════════════════════════════════════════")

		// Force invalidate all caches to ensure fresh data
		if savedCount > 0 {
			// Invalidate all list caches
			s.newsRepo.InvalidateListCache(bgCtx)
			logger.Info("Cache invalidated - %d news items saved/updated", savedCount)
		}
	}()

	return jobID, nil
}

func (s *CrawlerService) StopCrawl(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.isRunning {
		return fmt.Errorf("no crawler is currently running")
	}

	close(s.stopChan)
	s.stopChan = make(chan struct{})
	s.isRunning = false

	logger.Info("Crawler stopped")
	return nil
}

func (s *CrawlerService) GetStatus(ctx context.Context) map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	activeJobs := 0
	if s.isRunning {
		activeJobs = 1
	}

	// Get available sources
	sources := []string{
		"cointelegraph", "coindesk", "cryptonews",
		"binance", "coinmarketcap", "bitcoincom",
		"theblock", "decrypt", "utoday", "cryptoslate",
	}

	return map[string]interface{}{
		"is_running":      s.isRunning,
		"active_jobs":     activeJobs,
		"total_crawled":   s.totalCrawled,
		"total_filtered":  s.filteredCount,
		"last_crawl_time": nil, // TODO: Track last crawl time
		"sources":         sources,
	}
}

// FetchDetailFromURL crawls a specific URL to get full article details
func (s *CrawlerService) FetchDetailFromURL(ctx context.Context, url string) (*model.News, error) {
	logger.Info("Fetching detail from URL: %s", url)

	// Use the crawler to fetch detail page
	news, err := s.crawler.CrawlDetailPage(ctx, url)
	if err != nil {
		logger.Error("Failed to fetch detail from URL %s: %v", url, err)
		return nil, err
	}

	return news, nil
}
