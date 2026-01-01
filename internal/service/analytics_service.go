package service

import (
	"context"
	"time"

	"crawl-news/internal/model"
	"crawl-news/internal/repository"
)

type AnalyticsService struct {
	newsRepo *repository.NewsRepository
}

func NewAnalyticsService(newsRepo *repository.NewsRepository) *AnalyticsService {
	return &AnalyticsService{
		newsRepo: newsRepo,
	}
}

// CrawlStats represents crawling statistics
type CrawlStats struct {
	TotalNews         int                    `json:"total_news"`
	TotalSources      int                    `json:"total_sources"`
	NewsBySource      map[string]int         `json:"news_by_source"`
	NewsByCategory    map[string]int         `json:"news_by_category"`
	LatestCrawl       *time.Time             `json:"latest_crawl,omitempty"`
	OldestCrawl       *time.Time             `json:"oldest_crawl,omitempty"`
	NewsWithContent   int                    `json:"news_with_content"`
	NewsWithoutContent int                   `json:"news_without_content"`
	AvgContentLength  float64                `json:"avg_content_length"`
	AIAnalyzedCount   int                    `json:"ai_analyzed_count"`
	SentimentBreakdown map[string]int        `json:"sentiment_breakdown"`
}

// PageStats represents page-specific statistics
type PageStats struct {
	TotalPages        int                    `json:"total_pages"`
	AvgNewsPerPage    float64                `json:"avg_news_per_page"`
	PopularSources    []SourceCount          `json:"popular_sources"`
	RecentActivity    []DailyActivity        `json:"recent_activity"`
}

type SourceCount struct {
	Source string `json:"source"`
	Count  int    `json:"count"`
}

type DailyActivity struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
}

// GetCrawlStats returns comprehensive crawling statistics
func (s *AnalyticsService) GetCrawlStats(ctx context.Context) (*CrawlStats, error) {
	// Get all news
	allNews, err := s.newsRepo.FindWithFilter(ctx, &model.NewsFilter{})
	if err != nil {
		return nil, err
	}

	stats := &CrawlStats{
		TotalNews:         len(allNews),
		NewsBySource:      make(map[string]int),
		NewsByCategory:    make(map[string]int),
		SentimentBreakdown: make(map[string]int),
	}

	if len(allNews) == 0 {
		return stats, nil
	}

	// Analyze news
	sources := make(map[string]bool)
	totalContentLength := 0
	latestCrawl := allNews[0].CrawledAt
	oldestCrawl := allNews[0].CrawledAt

	for _, news := range allNews {
		// Count by source
		stats.NewsBySource[news.Source]++
		sources[news.Source] = true

		// Count by category
		if news.Category != "" {
			stats.NewsByCategory[news.Category]++
		}

		// Content analysis
		if len(news.Content) > 100 {
			stats.NewsWithContent++
			totalContentLength += len(news.Content)
		} else {
			stats.NewsWithoutContent++
		}

		// AI analysis
		if news.AIAnalyzed {
			stats.AIAnalyzedCount++
			if news.Sentiment != nil {
				stats.SentimentBreakdown[news.Sentiment.Label]++
			}
		}

		// Time tracking
		if news.CrawledAt.After(latestCrawl) {
			latestCrawl = news.CrawledAt
		}
		if news.CrawledAt.Before(oldestCrawl) {
			oldestCrawl = news.CrawledAt
		}
	}

	stats.TotalSources = len(sources)
	stats.LatestCrawl = &latestCrawl
	stats.OldestCrawl = &oldestCrawl

	if stats.NewsWithContent > 0 {
		stats.AvgContentLength = float64(totalContentLength) / float64(stats.NewsWithContent)
	}

	return stats, nil
}

// GetPageStats returns page-specific statistics
func (s *AnalyticsService) GetPageStats(ctx context.Context) (*PageStats, error) {
	allNews, err := s.newsRepo.FindWithFilter(ctx, &model.NewsFilter{})
	if err != nil {
		return nil, err
	}

	stats := &PageStats{
		PopularSources:  make([]SourceCount, 0),
		RecentActivity: make([]DailyActivity, 0),
	}

	if len(allNews) == 0 {
		return stats, nil
	}

	// Calculate pages (assuming 20 items per page)
	itemsPerPage := 20
	stats.TotalPages = (len(allNews) + itemsPerPage - 1) / itemsPerPage
	stats.AvgNewsPerPage = float64(len(allNews)) / float64(stats.TotalPages)

	// Popular sources
	sourceCounts := make(map[string]int)
	for _, news := range allNews {
		sourceCounts[news.Source]++
	}

	for source, count := range sourceCounts {
		stats.PopularSources = append(stats.PopularSources, SourceCount{
			Source: source,
			Count:  count,
		})
	}

	// Recent activity (last 7 days)
	dailyCounts := make(map[string]int)
	now := time.Now()
	for i := 6; i >= 0; i-- {
		date := now.AddDate(0, 0, -i).Format("2006-01-02")
		dailyCounts[date] = 0
	}

	for _, news := range allNews {
		date := news.CrawledAt.Format("2006-01-02")
		if _, exists := dailyCounts[date]; exists {
			dailyCounts[date]++
		}
	}

	for date, count := range dailyCounts {
		stats.RecentActivity = append(stats.RecentActivity, DailyActivity{
			Date:  date,
			Count: count,
		})
	}

	return stats, nil
}

// GetSourceAnalytics returns detailed analytics for a specific source
func (s *AnalyticsService) GetSourceAnalytics(ctx context.Context, source string) (map[string]interface{}, error) {
	filter := &model.NewsFilter{
		Sources: []string{source},
	}

	news, err := s.newsRepo.FindWithFilter(ctx, filter)
	if err != nil {
		return nil, err
	}

	analytics := map[string]interface{}{
		"source":       source,
		"total_news":   len(news),
		"with_content": 0,
		"with_author":  0,
		"with_images":  0,
		"avg_length":   0,
	}

	if len(news) == 0 {
		return analytics, nil
	}

	totalLength := 0
	withContent := 0
	withAuthor := 0
	withImages := 0

	for _, n := range news {
		if len(n.Content) > 100 {
			withContent++
			totalLength += len(n.Content)
		}
		if n.Author != "" {
			withAuthor++
		}
		if n.ImageURL != "" {
			withImages++
		}
	}

	analytics["with_content"] = withContent
	analytics["with_author"] = withAuthor
	analytics["with_images"] = withImages
	
	if withContent > 0 {
		analytics["avg_length"] = totalLength / withContent
	}

	return analytics, nil
}

// SentimentTrend represents sentiment data over time
type SentimentTrend struct {
	Time      string  `json:"time"`       // ISO format: "2025-12-30T10:00:00Z"
	Positive  int     `json:"positive"`  // Count of positive news
	Negative  int     `json:"negative"`    // Count of negative news
	Neutral   int     `json:"neutral"`    // Count of neutral news
	AvgScore  float64 `json:"avg_score"`  // Average sentiment score
	Total     int     `json:"total"`      // Total analyzed news
}

// SentimentTrendRequest represents request parameters for sentiment trends
type SentimentTrendRequest struct {
	Timeframe string   `json:"timeframe"` // "hour", "day", "week", "month"
	StartDate *time.Time `json:"start_date,omitempty"`
	EndDate   *time.Time `json:"end_date,omitempty"`
	Sources   []string `json:"sources,omitempty"`
	TradingPairs []string `json:"trading_pairs,omitempty"`
}

// GetSentimentTrends returns sentiment trends over time
func (s *AnalyticsService) GetSentimentTrends(ctx context.Context, req *SentimentTrendRequest) ([]SentimentTrend, error) {
	// Build filter
	filter := &model.NewsFilter{
		AIAnalyzed: func() *bool { b := true; return &b }(),
	}
	
	if req.StartDate != nil {
		filter.StartDate = req.StartDate
	}
	if req.EndDate != nil {
		filter.EndDate = req.EndDate
	}
	if len(req.Sources) > 0 {
		filter.Sources = req.Sources
	}
	if len(req.TradingPairs) > 0 {
		filter.TradingPairs = req.TradingPairs
	}

	// Get analyzed news
	news, err := s.newsRepo.FindWithFilter(ctx, filter)
	if err != nil {
		return nil, err
	}

	// Default timeframe
	timeframe := req.Timeframe
	if timeframe == "" {
		timeframe = "day"
	}

	// Group by time period
	trendsMap := make(map[string]*SentimentTrend)
	
	// Determine time format based on timeframe
	var timeFormat string
	var timeTruncate func(time.Time) time.Time
	
	switch timeframe {
	case "hour":
		timeFormat = "2006-01-02T15:04:05Z"
		timeTruncate = func(t time.Time) time.Time {
			return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), 0, 0, 0, t.Location())
		}
	case "day":
		timeFormat = "2006-01-02"
		timeTruncate = func(t time.Time) time.Time {
			return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
		}
	case "week":
		timeFormat = "2006-W01" // ISO week format
		timeTruncate = func(t time.Time) time.Time {
			// Get start of week (Monday)
			weekday := int(t.Weekday())
			if weekday == 0 {
				weekday = 7 // Sunday = 7
			}
			daysFromMonday := weekday - 1
			startOfWeek := t.AddDate(0, 0, -daysFromMonday)
			return time.Date(startOfWeek.Year(), startOfWeek.Month(), startOfWeek.Day(), 0, 0, 0, 0, startOfWeek.Location())
		}
	case "month":
		timeFormat = "2006-01"
		timeTruncate = func(t time.Time) time.Time {
			return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
		}
	default:
		timeFormat = "2006-01-02"
		timeTruncate = func(t time.Time) time.Time {
			return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
		}
	}

	// Aggregate sentiment data
	for _, n := range news {
		if n.Sentiment == nil {
			continue
		}

		// Use analyzed_at if available, otherwise use published_at
		timeKey := n.PublishedAt
		if n.AnalyzedAt != nil {
			timeKey = *n.AnalyzedAt
		}

		truncatedTime := timeTruncate(timeKey)
		timeKeyStr := truncatedTime.Format(timeFormat)

		// Initialize trend if not exists
		if trendsMap[timeKeyStr] == nil {
			trendsMap[timeKeyStr] = &SentimentTrend{
				Time: truncatedTime.Format(time.RFC3339),
			}
		}

		trend := trendsMap[timeKeyStr]
		trend.Total++

		// Count by sentiment label
		switch n.Sentiment.Label {
		case "positive":
			trend.Positive++
		case "negative":
			trend.Negative++
		case "neutral":
			trend.Neutral++
		}

		// Accumulate score for average
		trend.AvgScore += n.Sentiment.Score
	}

	// Calculate averages and convert to slice
	trends := make([]SentimentTrend, 0, len(trendsMap))
	for _, trend := range trendsMap {
		if trend.Total > 0 {
			trend.AvgScore = trend.AvgScore / float64(trend.Total)
		}
		trends = append(trends, *trend)
	}

	// Sort by time
	for i := 0; i < len(trends)-1; i++ {
		for j := i + 1; j < len(trends); j++ {
			if trends[i].Time > trends[j].Time {
				trends[i], trends[j] = trends[j], trends[i]
			}
		}
	}

	return trends, nil
}

// GetSentimentByTradingPair returns sentiment breakdown by trading pair
func (s *AnalyticsService) GetSentimentByTradingPair(ctx context.Context, pair string) (map[string]interface{}, error) {
	filter := &model.NewsFilter{
		TradingPairs: []string{pair},
		AIAnalyzed:  func() *bool { b := true; return &b }(),
	}

	news, err := s.newsRepo.FindWithFilter(ctx, filter)
	if err != nil {
		return nil, err
	}

	result := map[string]interface{}{
		"trading_pair": pair,
		"total_news":   len(news),
		"positive":     0,
		"negative":     0,
		"neutral":      0,
		"avg_score":    0.0,
		"avg_confidence": 0.0,
	}

	if len(news) == 0 {
		return result, nil
	}

	positive := 0
	negative := 0
	neutral := 0
	totalScore := 0.0
	totalConfidence := 0.0

	for _, n := range news {
		if n.Sentiment == nil {
			continue
		}

		switch n.Sentiment.Label {
		case "positive":
			positive++
		case "negative":
			negative++
		case "neutral":
			neutral++
		}

		totalScore += n.Sentiment.Score
		totalConfidence += n.Sentiment.Confidence
	}

	result["positive"] = positive
	result["negative"] = negative
	result["neutral"] = neutral
	result["avg_score"] = totalScore / float64(len(news))
	result["avg_confidence"] = totalConfidence / float64(len(news))

	return result, nil
}
