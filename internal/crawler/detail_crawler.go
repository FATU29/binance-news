package crawler

import (
	"context"
	"sync"
	"time"

	"crawl-news/internal/model"
	"crawl-news/pkg/logger"

	"github.com/gocolly/colly/v2"
)

// crawlDetailPagesWithAI crawls detail pages using AI parser with goroutines for optimization
func (c *Crawler) crawlDetailPagesWithAI(ctx context.Context, newsList []*model.News, sourceName string, maxConcurrent int) {
	if c.aiParser == nil || !c.aiParser.IsEnabled() {
		logger.Warn("AI parser not available, skipping detail page crawl")
		return
	}

	if len(newsList) == 0 {
		return
	}

	logger.Info("Starting AI-based detail page crawl for %d articles (max concurrent: %d)", len(newsList), maxConcurrent)

	// Create a semaphore to limit concurrent requests
	semaphore := make(chan struct{}, maxConcurrent)
	var wg sync.WaitGroup
	var mu sync.Mutex

	successCount := 0
	failCount := 0
	skippedCount := 0

	// Process each news item
	for i := range newsList {
		news := newsList[i]
		
		// Skip if already has sufficient content for analysis (500+ chars)
		if news.Content != "" && len(news.Content) >= 500 {
			logger.Debug("Article %s already has sufficient content (%d chars) for analysis, skipping", news.ID, len(news.Content))
			mu.Lock()
			skippedCount++
			mu.Unlock()
			continue
		}

		if news.SourceURL == "" {
			logger.Warn("Article %s has no source URL, skipping", news.ID)
			mu.Lock()
			skippedCount++
			mu.Unlock()
			continue
		}

		wg.Add(1)
		go func(idx int, article *model.News) {
			defer wg.Done()

			// Acquire semaphore
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			// Create a context with timeout for this request
			reqCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
			defer cancel()

			logger.Info("Crawling detail page %d/%d: %s", idx+1, len(newsList), article.SourceURL)

			// Fetch HTML content using colly
			var htmlContent string
			var fetchErr error
			var fetchMu sync.Mutex

			// Use a separate collector for each request to avoid race conditions
			detailCollector := colly.NewCollector(
				colly.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36"),
			)

			detailCollector.OnHTML("html", func(e *colly.HTMLElement) {
				fetchMu.Lock()
				htmlContent = string(e.Response.Body)
				fetchMu.Unlock()
			})

			detailCollector.OnError(func(r *colly.Response, err error) {
				fetchMu.Lock()
				fetchErr = err
				fetchMu.Unlock()
				logger.Error("Failed to fetch HTML for %s: %v", article.SourceURL, err)
			})

			// Visit the URL
			if err := detailCollector.Visit(article.SourceURL); err != nil {
				logger.Error("Failed to visit %s: %v", article.SourceURL, err)
				mu.Lock()
				failCount++
				mu.Unlock()
				return
			}

			detailCollector.Wait()

			fetchMu.Lock()
			hasError := fetchErr != nil
			hasContent := htmlContent != ""
			fetchMu.Unlock()

			if hasError || !hasContent {
				logger.Warn("Failed to fetch HTML content for %s (error: %v, content length: %d)", 
					article.SourceURL, hasError, len(htmlContent))
				mu.Lock()
				failCount++
				mu.Unlock()
				return
			}

			// Parse with AI
			aiNews, err := c.aiParser.ParseHTMLToNews(reqCtx, htmlContent, article.SourceURL, sourceName)
			if err != nil {
				logger.Error("AI parsing failed for %s: %v", article.SourceURL, err)
				mu.Lock()
				failCount++
				mu.Unlock()
				return
			}

			// Update the news item with AI-parsed data
			// Priority: Always use AI-parsed content if it's substantial (500+ chars) or longer than existing
			mu.Lock()
			if aiNews.Title != "" {
				article.Title = aiNews.Title
			}
			
			// CRITICAL: Always update content if AI parsed substantial content
			// This ensures full content is available for causal analysis
			if aiNews.Content != "" {
				aiContentLen := len(aiNews.Content)
				currentContentLen := len(article.Content)
				
				// Use AI content if:
				// 1. AI content is substantial (500+ chars) - likely full article
				// 2. AI content is longer than current (even if both are short)
				// 3. Current content is empty or very short (< 200 chars)
				if aiContentLen >= 500 || aiContentLen > currentContentLen || currentContentLen < 200 {
					article.Content = aiNews.Content
					logger.Info("Updated content: %d chars (was %d chars)", aiContentLen, currentContentLen)
				} else {
					logger.Debug("Keeping existing content: %d chars (AI: %d chars)", currentContentLen, aiContentLen)
				}
			}
			
			// Update summary if AI provides better one
			if aiNews.Summary != "" && (article.Summary == "" || len(aiNews.Summary) > len(article.Summary)) {
				article.Summary = aiNews.Summary
			}
			
			// Update other fields
			if aiNews.Author != "" {
				article.Author = aiNews.Author
			}
			if aiNews.ImageURL != "" && article.ImageURL == "" {
				article.ImageURL = aiNews.ImageURL
			}
			if len(aiNews.Tags) > 0 {
				article.Tags = aiNews.Tags
			}
			if aiNews.Language != "" {
				article.Language = aiNews.Language
			}
			if !aiNews.PublishedAt.IsZero() {
				article.PublishedAt = aiNews.PublishedAt
			}
			
			// Always update parsing metadata
			article.ParsingMethod = aiNews.ParsingMethod
			article.ParsingConfidence = aiNews.ParsingConfidence
			successCount++
			mu.Unlock()

			logger.Info("✅ Successfully parsed article %d/%d: %s (method: %s, confidence: %.2f, content: %d chars)",
				idx+1, len(newsList), article.Title, aiNews.ParsingMethod, aiNews.ParsingConfidence, len(aiNews.Content))
		}(i, news)
	}

	// Wait for all goroutines to complete
	wg.Wait()

	logger.Info("Detail page crawl completed: %d success, %d failed, %d skipped out of %d total",
		successCount, failCount, skippedCount, len(newsList))
}
