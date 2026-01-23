package repository

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"crawl-news/internal/config"
	"crawl-news/internal/db"
	"crawl-news/internal/model"
	"crawl-news/pkg/logger"

	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"
)

// NewsRepository handles data persistence for news
type NewsRepository struct {
	db       *gorm.DB
	redis    *redis.Client
	cacheTTL time.Duration
}

// GetDB returns the database connection (for use in services)
func (r *NewsRepository) GetDB() *gorm.DB {
	return r.db
}

func NewNewsRepository(cfg *config.RedisConfig) *NewsRepository {
	return &NewsRepository{
		db:       db.DB,
		redis:    db.RedisClient,
		cacheTTL: cfg.CacheTTL,
	}
}

// getCacheKey generates a cache key for news
func (r *NewsRepository) getCacheKey(id string) string {
	return fmt.Sprintf("news:%s", id)
}

// getListCacheKey generates a cache key for news lists
func (r *NewsRepository) getListCacheKey(source, category string, page, limit int) string {
	return fmt.Sprintf("news:list:%s:%s:%d:%d", source, category, page, limit)
}

// cacheNews stores news in Redis
func (r *NewsRepository) cacheNews(ctx context.Context, news *model.News) error {
	if r.redis == nil {
		return nil
	}

	data, err := json.Marshal(news)
	if err != nil {
		logger.Error("Failed to marshal news for caching: " + err.Error())
		return err
	}

	key := r.getCacheKey(news.ID)
	if err := r.redis.Set(ctx, key, data, r.cacheTTL).Err(); err != nil {
		logger.Error("Failed to cache news: " + err.Error())
		return err
	}

	return nil
}

// getCachedNews retrieves news from Redis
func (r *NewsRepository) getCachedNews(ctx context.Context, id string) (*model.News, error) {
	if r.redis == nil {
		return nil, fmt.Errorf("redis not available")
	}

	key := r.getCacheKey(id)
	data, err := r.redis.Get(ctx, key).Bytes()
	if err != nil {
		return nil, err
	}

	var news model.News
	if err := json.Unmarshal(data, &news); err != nil {
		return nil, err
	}

	return &news, nil
}

// invalidateCache removes cached news
func (r *NewsRepository) invalidateCache(ctx context.Context, id string) {
	if r.redis == nil {
		return
	}

	key := r.getCacheKey(id)
	if err := r.redis.Del(ctx, key).Err(); err != nil {
		logger.Error("Failed to invalidate cache: " + err.Error())
	}
}

// InvalidateListCache removes cached news lists (exported for use in services)
func (r *NewsRepository) InvalidateListCache(ctx context.Context) {
	if r.redis == nil {
		logger.Debug("Redis not available, skipping cache invalidation")
		return
	}

	deletedCount := 0

	// Delete all list cache keys
	iter := r.redis.Scan(ctx, 0, "news:list:*", 0).Iterator()
	for iter.Next(ctx) {
		if err := r.redis.Del(ctx, iter.Val()).Err(); err != nil {
			logger.Error("Failed to invalidate list cache key %s: %v", iter.Val(), err)
		} else {
			deletedCount++
		}
	}

	// Also delete filter cache keys
	iter2 := r.redis.Scan(ctx, 0, "news:filter:*", 0).Iterator()
	for iter2.Next(ctx) {
		if err := r.redis.Del(ctx, iter2.Val()).Err(); err != nil {
			logger.Error("Failed to invalidate filter cache key %s: %v", iter2.Val(), err)
		} else {
			deletedCount++
		}
	}

	// Delete all news item cache keys
	iter3 := r.redis.Scan(ctx, 0, "news:*", 0).Iterator()
	for iter3.Next(ctx) {
		key := iter3.Val()
		// Skip if it's a list or filter key (already deleted)
		if !strings.Contains(key, ":list:") && !strings.Contains(key, ":filter:") {
			r.redis.Del(ctx, key)
			deletedCount++
		}
	}

	if deletedCount > 0 {
		logger.Info("Invalidated %d cache keys (lists, filters, and items)", deletedCount)
	} else {
		logger.Debug("No cache keys found to invalidate")
	}
}

func (r *NewsRepository) FindAll(ctx context.Context, page, limit int, source, category string) ([]model.News, int, error) {
	// Try to get from cache first
	cacheKey := r.getListCacheKey(source, category, page, limit)
	if r.redis != nil {
		cachedData, err := r.redis.Get(ctx, cacheKey).Bytes()
		if err == nil {
			var results []model.News
			if err := json.Unmarshal(cachedData, &results); err == nil {
				logger.Info("Cache hit for news list")
				// Get total count
				var total int64
				query := r.db.Model(&model.News{})
				if source != "" {
					query = query.Where("source = ?", source)
				}
				if category != "" {
					query = query.Where("category = ?", category)
				}
				query.Count(&total)
				return results, int(total), nil
			}
		}
	}

	// Query from database
	var results []model.News
	var total int64

	query := r.db.Model(&model.News{})

	// Apply filters
	if source != "" {
		query = query.Where("source = ?", source)
	}
	if category != "" {
		query = query.Where("category = ?", category)
	}

	// Get total count
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Apply pagination and fetch results
	offset := (page - 1) * limit

	// Use Select to explicitly handle JSONB fields
	// GORM's AfterFind hook will automatically parse JSONB fields
	if err := query.Select("*").
		Order("published_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&results).Error; err != nil {
		logger.Error("Failed to fetch news: %v", err)
		return nil, 0, err
	}

	// Cache the results
	if r.redis != nil {
		data, err := json.Marshal(results)
		if err == nil {
			r.redis.Set(ctx, cacheKey, data, r.cacheTTL)
		}
	}

	return results, int(total), nil
}

func (r *NewsRepository) FindByID(ctx context.Context, id string) (*model.News, error) {
	// Try cache first
	if cachedNews, err := r.getCachedNews(ctx, id); err == nil {
		logger.Info("Cache hit for news: " + id)
		return cachedNews, nil
	}

	// Query from database
	var news model.News
	if err := r.db.Where("id = ?", id).First(&news).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("news not found")
		}
		return nil, err
	}

	// Cache the result
	r.cacheNews(ctx, &news)

	return &news, nil
}

func (r *NewsRepository) Create(ctx context.Context, news *model.News) error {
	// Check if already exists by source_url (unique constraint)
	var existing model.News
	err := r.db.Where("source_url = ?", news.SourceURL).First(&existing).Error
	if err == nil {
		// News already exists, merge and update it instead
		logger.Debug("News already exists with source_url: %s (ID: %s), merging and updating", news.SourceURL, existing.ID)
		
		// Merge content: prefer longer/more complete content
		if news.Content != "" {
			existingContentLen := len(existing.Content)
			newContentLen := len(news.Content)
			
			// Use new content if:
			// 1. New content is substantial (500+ chars) - likely full article
			// 2. New content is longer than existing
			// 3. Existing content is empty or very short
			if newContentLen >= 500 || newContentLen > existingContentLen || existingContentLen < 200 {
				existing.Content = news.Content
				logger.Debug("Merged content: updated to %d chars (was %d chars)", newContentLen, existingContentLen)
			}
		}
		
		// Merge other fields (prefer new if better)
		if news.Title != "" {
			existing.Title = news.Title
		}
		if news.Summary != "" && (existing.Summary == "" || len(news.Summary) > len(existing.Summary)) {
			existing.Summary = news.Summary
		}
		if news.Author != "" {
			existing.Author = news.Author
		}
		if news.ImageURL != "" && existing.ImageURL == "" {
			existing.ImageURL = news.ImageURL
		}
		if len(news.Tags) > 0 {
			existing.Tags = news.Tags
		}
		if news.Language != "" {
			existing.Language = news.Language
		}
		if !news.PublishedAt.IsZero() {
			existing.PublishedAt = news.PublishedAt
		}
		// Update parsing metadata
		if news.ParsingMethod != "" {
			existing.ParsingMethod = news.ParsingMethod
		}
		if news.ParsingConfidence > 0 {
			existing.ParsingConfidence = news.ParsingConfidence
		}
		
		existing.UpdatedAt = time.Now()
		existing.CrawledAt = time.Now()
		
		return r.Update(ctx, &existing)
	}
	if err != gorm.ErrRecordNotFound {
		logger.Error("Error checking for existing news: %v", err)
		return err
	}

	// Ensure required fields are set
	if news.ID == "" {
		// Generate ID from source_url if not set
		hash := md5.Sum([]byte(news.SourceURL))
		news.ID = hex.EncodeToString(hash[:])
	}
	if news.CrawledAt.IsZero() {
		news.CrawledAt = time.Now()
	}
	if news.PublishedAt.IsZero() {
		news.PublishedAt = time.Now()
	}

	// Create in database
	if err := r.db.Create(news).Error; err != nil {
		logger.Error("Failed to create news in database: %v, news: %+v", err, news)
		return fmt.Errorf("failed to create news: %w", err)
	}

	logger.Info("Successfully created news: %s (ID: %s)", news.Title, news.ID)

	// Cache the news
	r.cacheNews(ctx, news)

	// Invalidate list cache
	r.InvalidateListCache(ctx)

	return nil
}

func (r *NewsRepository) Update(ctx context.Context, news *model.News) error {
	// Update in database
	if err := r.db.Save(news).Error; err != nil {
		logger.Error("Failed to update news in database: %v, news: %+v", err, news)
		return fmt.Errorf("failed to update news: %w", err)
	}

	logger.Info("Successfully updated news: %s (ID: %s)", news.Title, news.ID)

	// Invalidate cache
	r.invalidateCache(ctx, news.ID)
	r.InvalidateListCache(ctx)

	return nil
}

func (r *NewsRepository) Delete(ctx context.Context, id string) error {
	// Delete from database
	if err := r.db.Where("id = ?", id).Delete(&model.News{}).Error; err != nil {
		return err
	}

	// Invalidate cache
	r.invalidateCache(ctx, id)
	r.InvalidateListCache(ctx)

	return nil
}

// FindWithFilter searches news with advanced filters
func (r *NewsRepository) FindWithFilter(ctx context.Context, filter *model.NewsFilter) ([]model.News, error) {
	query := r.db.Model(&model.News{})

	// Apply date range filter
	if filter.StartDate != nil {
		query = query.Where("published_at >= ?", *filter.StartDate)
	}
	if filter.EndDate != nil {
		query = query.Where("published_at <= ?", *filter.EndDate)
	}

	// Apply source filter
	if len(filter.Sources) > 0 {
		query = query.Where("source IN ?", filter.Sources)
	}

	// Apply category filter
	if len(filter.Categories) > 0 {
		query = query.Where("category IN ?", filter.Categories)
	}

	// Apply sentiment filter
	if filter.Sentiment != "" {
		query = query.Where("sentiment->>'label' = ?", filter.Sentiment)
	}

	// Apply min score filter
	if filter.MinScore != nil {
		query = query.Where("(sentiment->>'score')::float >= ?", *filter.MinScore)
	}

	// Apply AI analyzed filter
	if filter.AIAnalyzed != nil {
		query = query.Where("ai_analyzed = ?", *filter.AIAnalyzed)
	}

	// Apply language filter
	if filter.Language != "" {
		query = query.Where("language = ?", filter.Language)
	}

	// Apply parsing method filter
	if filter.ParsingMethod != "" {
		query = query.Where("parsing_method = ?", filter.ParsingMethod)
	}

	// Apply trading pairs filter (JSONB array contains check)
	if len(filter.TradingPairs) > 0 {
		for _, pair := range filter.TradingPairs {
			query = query.Where("related_pairs @> ?", fmt.Sprintf(`["%s"]`, pair))
		}
	}

	var results []model.News
	if err := query.Order("published_at DESC").Find(&results).Error; err != nil {
		return nil, err
	}

	return results, nil
}

// FindByTradingPair finds news related to a specific trading pair
func (r *NewsRepository) FindByTradingPair(ctx context.Context, pair string) ([]model.News, error) {
	var results []model.News

	// JSONB array contains check
	if err := r.db.Where("related_pairs @> ?", fmt.Sprintf(`["%s"]`, pair)).
		Order("published_at DESC").
		Find(&results).Error; err != nil {
		return nil, err
	}

	return results, nil
}

// GetUnanalyzedNews returns news that haven't been analyzed by AI
func (r *NewsRepository) GetUnanalyzedNews(ctx context.Context, limit int) ([]model.News, error) {
	var results []model.News

	if err := r.db.Where("ai_analyzed = ?", false).
		Order("published_at DESC").
		Limit(limit).
		Find(&results).Error; err != nil {
		return nil, err
	}

	return results, nil
}

// GetNewsSummaries returns simplified news data for listing
func (r *NewsRepository) GetNewsSummaries(ctx context.Context, filter *model.NewsFilter, limit int) ([]model.NewsSummary, error) {
	news, err := r.FindWithFilter(ctx, filter)
	if err != nil {
		return nil, err
	}

	summaries := make([]model.NewsSummary, 0, len(news))
	count := 0

	for _, n := range news {
		if count >= limit {
			break
		}

		summary := model.NewsSummary{
			ID:           n.ID,
			Title:        n.Title,
			Summary:      n.Summary,
			Source:       n.Source,
			ImageURL:     n.ImageURL,
			PublishedAt:  n.PublishedAt,
			Sentiment:    n.Sentiment,
			RelatedPairs: n.RelatedPairs,
			PriceImpact:  n.PriceImpact,
		}
		summaries = append(summaries, summary)
		count++
	}

	return summaries, nil
}

// FindWithFilterPaginated searches news with filters and pagination
func (r *NewsRepository) FindWithFilterPaginated(ctx context.Context, filter *model.NewsFilter, page, limit int) ([]model.News, int, error) {
	query := r.db.Model(&model.News{})

	// Apply date range filter
	if filter.StartDate != nil {
		query = query.Where("published_at >= ?", *filter.StartDate)
	}
	if filter.EndDate != nil {
		query = query.Where("published_at <= ?", *filter.EndDate)
	}

	// Apply source filter
	if len(filter.Sources) > 0 {
		query = query.Where("source IN ?", filter.Sources)
	}

	// Apply category filter
	if len(filter.Categories) > 0 {
		query = query.Where("category IN ?", filter.Categories)
	}

	// Apply sentiment filter
	if filter.Sentiment != "" {
		query = query.Where("sentiment->>'label' = ?", filter.Sentiment)
	}

	// Apply min score filter
	if filter.MinScore != nil {
		query = query.Where("(sentiment->>'score')::float >= ?", *filter.MinScore)
	}

	// Apply AI analyzed filter
	if filter.AIAnalyzed != nil {
		query = query.Where("ai_analyzed = ?", *filter.AIAnalyzed)
	}

	// Apply language filter
	if filter.Language != "" {
		query = query.Where("language = ?", filter.Language)
	}

	// Apply trading pairs filter (JSONB array contains check)
	if len(filter.TradingPairs) > 0 {
		for _, pair := range filter.TradingPairs {
			query = query.Where("related_pairs @> ?", fmt.Sprintf(`["%s"]`, pair))
		}
	}

	// Get total count
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Apply pagination
	offset := (page - 1) * limit
	var results []model.News
	if err := query.Order("published_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&results).Error; err != nil {
		return nil, 0, err
	}

	return results, int(total), nil
}
