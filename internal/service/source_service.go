package service

import (
	"context"
	"fmt"
	"time"

	"crawl-news/internal/crawler"
	"crawl-news/internal/model"
	"crawl-news/pkg/logger"

	"gorm.io/gorm"
)

type SourceService struct {
	db               *gorm.DB
	crawler          *crawler.Crawler
	structureMonitor *StructureMonitor
}

func NewSourceService(db *gorm.DB, crawlerInstance *crawler.Crawler, monitor *StructureMonitor) *SourceService {
	return &SourceService{
		db:               db,
		crawler:          crawlerInstance,
		structureMonitor: monitor,
	}
}

// CrawlSourceConfig represents a source configuration in database
type CrawlSourceConfig struct {
	ID          string         `gorm:"primaryKey;size:100"`
	Name        string         `gorm:"size:100;uniqueIndex;not null"`
	BaseURL     string         `gorm:"size:500;not null"`
	Enabled     bool           `gorm:"default:true;index"`
	Priority    int            `gorm:"default:0"`
	CrawlFreq   int            `gorm:"default:60"` // Minutes
	LastCrawled time.Time      `gorm:"index"`
	Selectors   string         `gorm:"type:jsonb"` // JSON serialized Selector
	Categories  string         `gorm:"type:jsonb"` // JSON serialized []string
	Language    string         `gorm:"size:10;default:'en'"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   gorm.DeletedAt `gorm:"index"`
}

func (CrawlSourceConfig) TableName() string {
	return "crawl_sources"
}

// GetAllSources returns all sources, optionally filtered by enabled status
func (s *SourceService) GetAllSources(enabled *bool) ([]model.CrawlSource, error) {
	var configs []CrawlSourceConfig
	query := s.db

	if enabled != nil {
		query = query.Where("enabled = ?", *enabled)
	}

	if err := query.Find(&configs).Error; err != nil {
		return nil, err
	}

	// Convert to model.CrawlSource
	sources := make([]model.CrawlSource, len(configs))
	for i, cfg := range configs {
		sources[i] = s.configToModel(&cfg)
	}

	return sources, nil
}

// GetSourceByName returns a specific source by name
func (s *SourceService) GetSourceByName(name string) (*model.CrawlSource, error) {
	var config CrawlSourceConfig
	
	if err := s.db.Where("name = ?", name).First(&config).Error; err != nil {
		return nil, err
	}

	source := s.configToModel(&config)
	return &source, nil
}

// CreateSource creates a new crawl source
func (s *SourceService) CreateSource(source *model.CrawlSource) error {
	// Check if source already exists
	var existing CrawlSourceConfig
	if err := s.db.Where("name = ?", source.Name).First(&existing).Error; err == nil {
		return fmt.Errorf("source with name '%s' already exists", source.Name)
	}

	// Generate ID if not provided
	if source.ID == "" {
		source.ID = fmt.Sprintf("src-%s-%d", source.Name, time.Now().Unix())
	}

	config := s.modelToConfig(source)
	
	if err := s.db.Create(&config).Error; err != nil {
		return fmt.Errorf("failed to create source: %w", err)
	}

	logger.Info("Created new crawl source: %s", source.Name)
	return nil
}

// UpdateSource updates an existing source
func (s *SourceService) UpdateSource(name string, updates *model.CrawlSource) error {
	var config CrawlSourceConfig
	
	if err := s.db.Where("name = ?", name).First(&config).Error; err != nil {
		return fmt.Errorf("source not found: %s", name)
	}

	// Update fields
	updatedConfig := s.modelToConfig(updates)
	updatedConfig.ID = config.ID // Keep original ID
	updatedConfig.CreatedAt = config.CreatedAt // Keep original creation time

	if err := s.db.Model(&config).Updates(&updatedConfig).Error; err != nil {
		return fmt.Errorf("failed to update source: %w", err)
	}

	logger.Info("Updated crawl source: %s", name)
	return nil
}

// DeleteSource deletes a source
func (s *SourceService) DeleteSource(name string) error {
	result := s.db.Where("name = ?", name).Delete(&CrawlSourceConfig{})
	
	if result.Error != nil {
		return fmt.Errorf("failed to delete source: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("source not found: %s", name)
	}

	logger.Info("Deleted crawl source: %s", name)
	return nil
}

// UpdateSelectors updates selectors for a source
func (s *SourceService) UpdateSelectors(name string, selectors *model.Selector) error {
	var config CrawlSourceConfig
	
	if err := s.db.Where("name = ?", name).First(&config).Error; err != nil {
		return fmt.Errorf("source not found: %s", name)
	}

	// Serialize selectors to JSON
	selectorsJSON, err := serializeSelectors(selectors)
	if err != nil {
		return fmt.Errorf("failed to serialize selectors: %w", err)
	}

	if err := s.db.Model(&config).Update("selectors", selectorsJSON).Error; err != nil {
		return fmt.Errorf("failed to update selectors: %w", err)
	}

	logger.Info("Updated selectors for source: %s", name)
	return nil
}

// SetSourceEnabled enables or disables a source
func (s *SourceService) SetSourceEnabled(name string, enabled bool) error {
	result := s.db.Model(&CrawlSourceConfig{}).
		Where("name = ?", name).
		Update("enabled", enabled)

	if result.Error != nil {
		return fmt.Errorf("failed to update source status: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("source not found: %s", name)
	}

	status := "disabled"
	if enabled {
		status = "enabled"
	}
	logger.Info("Source %s %s", name, status)

	return nil
}

// TestSource tests if a source is working correctly
func (s *SourceService) TestSource(ctx context.Context, name string) (map[string]interface{}, error) {
	source, err := s.GetSourceByName(name)
	if err != nil {
		return nil, err
	}

	startTime := time.Now()
	
	// Try to crawl
	results, err := s.crawler.Crawl(ctx, name)
	
	duration := time.Since(startTime)
	
	testResult := map[string]interface{}{
		"source":        name,
		"duration_ms":   duration.Milliseconds(),
		"items_found":   len(results),
		"success":       err == nil,
		"tested_at":     time.Now(),
		"base_url":      source.BaseURL,
	}

	if err != nil {
		testResult["error"] = err.Error()
		testResult["success"] = false
	} else {
		testResult["sample_titles"] = extractSampleTitles(results, 3)
	}

	return testResult, nil
}

// GetSourceHealth returns health metrics for a source
func (s *SourceService) GetSourceHealth(name string) (map[string]interface{}, error) {
	// Get structure monitor health
	health, err := s.structureMonitor.GetSourceHealth(name)
	if err != nil {
		return nil, err
	}

	// Get source info
	source, err := s.GetSourceByName(name)
	if err != nil {
		return health, nil // Return structure health even if source not in DB
	}

	// Add source-specific info
	health["enabled"] = source.Enabled
	health["priority"] = source.Priority
	health["crawl_frequency_minutes"] = source.CrawlFreq
	health["last_crawled"] = source.LastCrawled

	// Calculate time since last crawl
	if !source.LastCrawled.IsZero() {
		minutesSinceLastCrawl := int(time.Since(source.LastCrawled).Minutes())
		health["minutes_since_last_crawl"] = minutesSinceLastCrawl
		
		if minutesSinceLastCrawl > source.CrawlFreq*2 {
			health["crawl_status"] = "overdue"
		} else if minutesSinceLastCrawl > source.CrawlFreq {
			health["crawl_status"] = "due"
		} else {
			health["crawl_status"] = "recent"
		}
	}

	return health, nil
}

// Helper functions

func (s *SourceService) configToModel(config *CrawlSourceConfig) model.CrawlSource {
	selectors := deserializeSelectors(config.Selectors)
	categories := deserializeStringArray(config.Categories)

	return model.CrawlSource{
		ID:          config.ID,
		Name:        config.Name,
		BaseURL:     config.BaseURL,
		Enabled:     config.Enabled,
		Priority:    config.Priority,
		CrawlFreq:   config.CrawlFreq,
		LastCrawled: config.LastCrawled,
		Selectors:   selectors,
		Categories:  categories,
		Language:    config.Language,
	}
}

func (s *SourceService) modelToConfig(source *model.CrawlSource) CrawlSourceConfig {
	selectorsJSON, _ := serializeSelectors(&source.Selectors)
	categoriesJSON, _ := serializeStringArray(source.Categories)

	return CrawlSourceConfig{
		ID:          source.ID,
		Name:        source.Name,
		BaseURL:     source.BaseURL,
		Enabled:     source.Enabled,
		Priority:    source.Priority,
		CrawlFreq:   source.CrawlFreq,
		LastCrawled: source.LastCrawled,
		Selectors:   selectorsJSON,
		Categories:  categoriesJSON,
		Language:    source.Language,
	}
}

func serializeSelectors(sel *model.Selector) (string, error) {
	// In production, use proper JSON marshaling
	return fmt.Sprintf(`{"title":"%s","content":"%s","summary":"%s","author":"%s","published_at":"%s","image_url":"%s","article_list":"%s","article_link":"%s"}`,
		sel.Title, sel.Content, sel.Summary, sel.Author, sel.PublishedAt, sel.ImageURL, sel.ArticleList, sel.ArticleLink), nil
}

func deserializeSelectors(data string) model.Selector {
	// In production, use proper JSON unmarshaling
	// For now, return empty selector
	return model.Selector{}
}

func serializeStringArray(arr []string) (string, error) {
	// In production, use proper JSON marshaling
	return fmt.Sprintf(`%v`, arr), nil
}

func deserializeStringArray(data string) []string {
	// In production, use proper JSON unmarshaling
	return []string{}
}

func extractSampleTitles(news []*model.News, count int) []string {
	titles := make([]string, 0, count)
	for i := 0; i < count && i < len(news); i++ {
		titles = append(titles, news[i].Title)
	}
	return titles
}
