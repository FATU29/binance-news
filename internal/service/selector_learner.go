package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"crawl-news/pkg/logger"

	"github.com/gocolly/colly/v2"
	"gorm.io/gorm"
)

// SelectorLearner learns HTML structure patterns automatically
type SelectorLearner struct {
	db              *gorm.DB
	structureMonitor *StructureMonitor
}

func NewSelectorLearner(db *gorm.DB, monitor *StructureMonitor) *SelectorLearner {
	return &SelectorLearner{
		db:              db,
		structureMonitor: monitor,
	}
}

// LearnedSelector represents a discovered selector pattern
type LearnedSelector struct {
	ID           int       `gorm:"primaryKey"`
	Source       string    `gorm:"size:100;not null;index"`
	ElementType  string    `gorm:"size:50;not null"`
	Selector     string    `gorm:"size:500;not null;uniqueIndex:idx_source_element_selector"`
	Confidence   float64   `gorm:"default:0"`  // 0-100
	SuccessCount int       `gorm:"default:0"`
	FailureCount int       `gorm:"default:0"`
	IsActive     bool      `gorm:"default:true"`
	IsPrimary    bool      `gorm:"default:false"` // Primary selector for this element type
	DiscoveredAt time.Time
	LastTested   time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// SelectorFallback provides fallback selectors when primary fails
type SelectorFallback struct {
	ID           int       `gorm:"primaryKey"`
	Source       string    `gorm:"size:100;not null;index"`
	ElementType  string    `gorm:"size:50;not null"`
	PrimarySelector   string `gorm:"size:500;not null"`
	FallbackSelector  string `gorm:"size:500;not null"`
	Priority     int       `gorm:"default:0"` // Lower = higher priority
	SuccessRate  float64   `gorm:"default:0"`
	LastUsed     time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// DiscoverSelectors analyzes a page to discover potential selectors
func (sl *SelectorLearner) DiscoverSelectors(ctx context.Context, sourceURL string, sourceName string) ([]LearnedSelector, error) {
	logger.Info("Starting selector discovery for %s", sourceName)
	
	var discovered []LearnedSelector
	collector := colly.NewCollector()

	// Common patterns for news articles
	articlePatterns := []string{
		"article", ".article", "#article",
		".post", ".news-item", ".story",
		".entry", ".content-item",
		"[itemtype*='Article']", "[itemtype*='NewsArticle']",
	}

	titlePatterns := []string{
		"h1", "h2.title", ".title h1", ".headline",
		".post-title", ".article-title", ".entry-title",
		"[itemprop='headline']", "header h1",
	}

	contentPatterns := []string{
		".content", ".post-content", ".article-content",
		".entry-content", ".story-body", ".article-body",
		"[itemprop='articleBody']", "article p",
	}

	imagePatterns := []string{
		".featured-image img", ".post-image img",
		".article-image img", "figure img",
		"[itemprop='image']", ".thumbnail img",
	}

	authorPatterns := []string{
		".author", ".by-author", ".post-author",
		"[rel='author']", "[itemprop='author']",
		".author-name", ".byline",
	}

	datePatterns := []string{
		"time", "[datetime]", ".date", ".published",
		".post-date", "[itemprop='datePublished']",
		".timestamp", ".article-date",
	}

	// Try to find article containers
	collector.OnHTML("body", func(e *colly.HTMLElement) {
		// Test article patterns
		for _, pattern := range articlePatterns {
			count := 0
			e.ForEach(pattern, func(_ int, el *colly.HTMLElement) {
				count++
			})
			
			if count > 0 {
				discovered = append(discovered, LearnedSelector{
					Source:       sourceName,
					ElementType:  "article_list",
					Selector:     pattern,
					Confidence:   calculateConfidence(count, pattern),
					SuccessCount: 1,
					DiscoveredAt: time.Now(),
					LastTested:   time.Now(),
				})
			}
		}

		// Test title patterns
		for _, pattern := range titlePatterns {
			if testSelector(e, pattern) {
				discovered = append(discovered, LearnedSelector{
					Source:       sourceName,
					ElementType:  "title",
					Selector:     pattern,
					Confidence:   calculateSelectorConfidence(pattern, "title"),
					SuccessCount: 1,
					DiscoveredAt: time.Now(),
					LastTested:   time.Now(),
				})
			}
		}

		// Test content patterns
		for _, pattern := range contentPatterns {
			if testSelector(e, pattern) {
				discovered = append(discovered, LearnedSelector{
					Source:       sourceName,
					ElementType:  "content",
					Selector:     pattern,
					Confidence:   calculateSelectorConfidence(pattern, "content"),
					SuccessCount: 1,
					DiscoveredAt: time.Now(),
					LastTested:   time.Now(),
				})
			}
		}

		// Test image patterns
		for _, pattern := range imagePatterns {
			if testSelector(e, pattern) {
				discovered = append(discovered, LearnedSelector{
					Source:       sourceName,
					ElementType:  "image",
					Selector:     pattern,
					Confidence:   calculateSelectorConfidence(pattern, "image"),
					SuccessCount: 1,
					DiscoveredAt: time.Now(),
					LastTested:   time.Now(),
				})
			}
		}

		// Test author patterns
		for _, pattern := range authorPatterns {
			if testSelector(e, pattern) {
				discovered = append(discovered, LearnedSelector{
					Source:       sourceName,
					ElementType:  "author",
					Selector:     pattern,
					Confidence:   calculateSelectorConfidence(pattern, "author"),
					SuccessCount: 1,
					DiscoveredAt: time.Now(),
					LastTested:   time.Now(),
				})
			}
		}

		// Test date patterns
		for _, pattern := range datePatterns {
			if testSelector(e, pattern) {
				discovered = append(discovered, LearnedSelector{
					Source:       sourceName,
					ElementType:  "date",
					Selector:     pattern,
					Confidence:   calculateSelectorConfidence(pattern, "date"),
					SuccessCount: 1,
					DiscoveredAt: time.Now(),
					LastTested:   time.Now(),
				})
			}
		}
	})

	collector.OnError(func(r *colly.Response, err error) {
		logger.Error("Failed to discover selectors for %s: %v", sourceName, err)
	})

	err := collector.Visit(sourceURL)
	if err != nil {
		return nil, fmt.Errorf("failed to visit URL for discovery: %w", err)
	}

	logger.Info("Discovered %d potential selectors for %s", len(discovered), sourceName)
	
	// Save discovered selectors to database
	for i := range discovered {
		if err := sl.saveDiscoveredSelector(&discovered[i]); err != nil {
			logger.Error("Failed to save discovered selector: %v", err)
		}
	}

	return discovered, nil
}

// testSelector checks if a selector exists and has content
func testSelector(e *colly.HTMLElement, selector string) bool {
	text := strings.TrimSpace(e.ChildText(selector))
	return text != "" && len(text) > 10
}

// calculateConfidence calculates confidence score based on count and selector specificity
func calculateConfidence(count int, selector string) float64 {
	// Base confidence on element count (prefer 3-10 items for article lists)
	countScore := 0.0
	if count >= 3 && count <= 10 {
		countScore = 70.0
	} else if count > 10 && count <= 20 {
		countScore = 60.0
	} else if count == 1 || count == 2 {
		countScore = 40.0
	} else {
		countScore = 30.0
	}

	// Add bonus for specific selectors
	specificityBonus := 0.0
	if strings.Contains(selector, "article") || strings.Contains(selector, "post") {
		specificityBonus = 20.0
	} else if strings.Contains(selector, "news") || strings.Contains(selector, "story") {
		specificityBonus = 15.0
	} else if strings.Contains(selector, "item") {
		specificityBonus = 10.0
	}

	return min(100.0, countScore+specificityBonus)
}

// calculateSelectorConfidence calculates confidence for non-list selectors
func calculateSelectorConfidence(selector, elementType string) float64 {
	confidence := 50.0

	// Semantic HTML bonus
	if strings.Contains(selector, "[itemprop") || strings.Contains(selector, "[itemtype") {
		confidence += 30.0
	}

	// Element type matching bonus
	switch elementType {
	case "title":
		if strings.Contains(selector, "h1") || strings.Contains(selector, "title") || strings.Contains(selector, "headline") {
			confidence += 20.0
		}
	case "content":
		if strings.Contains(selector, "content") || strings.Contains(selector, "body") || strings.Contains(selector, "article p") {
			confidence += 20.0
		}
	case "author":
		if strings.Contains(selector, "author") || strings.Contains(selector, "byline") {
			confidence += 25.0
		}
	case "date":
		if selector == "time" || strings.Contains(selector, "date") || strings.Contains(selector, "[datetime]") {
			confidence += 25.0
		}
	case "image":
		if strings.Contains(selector, "img") {
			confidence += 15.0
		}
	}

	return min(100.0, confidence)
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// saveDiscoveredSelector saves or updates a discovered selector
func (sl *SelectorLearner) saveDiscoveredSelector(selector *LearnedSelector) error {
	var existing LearnedSelector
	
	err := sl.db.Where("source = ? AND element_type = ? AND selector = ?",
		selector.Source, selector.ElementType, selector.Selector).
		First(&existing).Error

	if err == gorm.ErrRecordNotFound {
		// Create new
		return sl.db.Create(selector).Error
	}

	// Update existing
	existing.SuccessCount++
	existing.LastTested = time.Now()
	existing.Confidence = (existing.Confidence + selector.Confidence) / 2 // Average confidence
	
	return sl.db.Save(&existing).Error
}

// GetBestSelector returns the best selector for a source and element type
func (sl *SelectorLearner) GetBestSelector(source, elementType string) (*LearnedSelector, error) {
	var selector LearnedSelector
	
	err := sl.db.Where("source = ? AND element_type = ? AND is_active = true",
		source, elementType).
		Order("confidence DESC, success_count DESC, is_primary DESC").
		First(&selector).Error

	if err != nil {
		return nil, err
	}

	return &selector, nil
}

// GetFallbackSelectors returns fallback selectors for a source and element type
func (sl *SelectorLearner) GetFallbackSelectors(source, elementType string) ([]LearnedSelector, error) {
	var selectors []LearnedSelector
	
	err := sl.db.Where("source = ? AND element_type = ? AND is_active = true",
		source, elementType).
		Order("confidence DESC, success_count DESC").
		Limit(5).
		Find(&selectors).Error

	return selectors, err
}

// RecordSelectorSuccess records a successful extraction
func (sl *SelectorLearner) RecordSelectorSuccess(source, elementType, selector string) error {
	var learned LearnedSelector
	
	err := sl.db.Where("source = ? AND element_type = ? AND selector = ?",
		source, elementType, selector).First(&learned).Error

	if err == gorm.ErrRecordNotFound {
		// Create new with success
		learned = LearnedSelector{
			Source:       source,
			ElementType:  elementType,
			Selector:     selector,
			Confidence:   70.0,
			SuccessCount: 1,
			FailureCount: 0,
			IsActive:     true,
			DiscoveredAt: time.Now(),
			LastTested:   time.Now(),
		}
		return sl.db.Create(&learned).Error
	}

	// Update existing
	learned.SuccessCount++
	learned.LastTested = time.Now()
	
	// Recalculate confidence
	totalAttempts := learned.SuccessCount + learned.FailureCount
	successRate := float64(learned.SuccessCount) / float64(totalAttempts)
	learned.Confidence = successRate * 100

	// Also notify structure monitor
	if sl.structureMonitor != nil {
		sl.structureMonitor.RecordSuccess(source, elementType, selector)
	}

	return sl.db.Save(&learned).Error
}

// RecordSelectorFailure records a failed extraction
func (sl *SelectorLearner) RecordSelectorFailure(source, elementType, selector string) error {
	var learned LearnedSelector
	
	err := sl.db.Where("source = ? AND element_type = ? AND selector = ?",
		source, elementType, selector).First(&learned).Error

	if err == gorm.ErrRecordNotFound {
		// Create new with failure
		learned = LearnedSelector{
			Source:       source,
			ElementType:  elementType,
			Selector:     selector,
			Confidence:   30.0,
			SuccessCount: 0,
			FailureCount: 1,
			IsActive:     true,
			DiscoveredAt: time.Now(),
			LastTested:   time.Now(),
		}
		return sl.db.Create(&learned).Error
	}

	// Update existing
	learned.FailureCount++
	learned.LastTested = time.Now()
	
	// Recalculate confidence
	totalAttempts := learned.SuccessCount + learned.FailureCount
	successRate := float64(learned.SuccessCount) / float64(totalAttempts)
	learned.Confidence = successRate * 100

	// Deactivate if failure rate is too high
	if learned.FailureCount > 5 && learned.Confidence < 30 {
		learned.IsActive = false
		logger.Warn("Deactivated selector for %s/%s: %s (confidence: %.2f%%)",
			source, elementType, selector, learned.Confidence)
	}

	// Also notify structure monitor
	if sl.structureMonitor != nil {
		sl.structureMonitor.RecordFailure(source, elementType, selector)
	}

	return sl.db.Save(&learned).Error
}

// AutoHealSelector attempts to find a replacement when primary selector fails
func (sl *SelectorLearner) AutoHealSelector(ctx context.Context, source, elementType, failedSelector string) (*LearnedSelector, error) {
	logger.Info("Attempting auto-heal for %s/%s (failed: %s)", source, elementType, failedSelector)

	// Record failure
	sl.RecordSelectorFailure(source, elementType, failedSelector)

	// Get alternative selectors
	alternatives, err := sl.GetFallbackSelectors(source, elementType)
	if err != nil {
		return nil, fmt.Errorf("no fallback selectors found: %w", err)
	}

	// Try alternatives in order of confidence
	for _, alt := range alternatives {
		if alt.Selector == failedSelector {
			continue // Skip the failed one
		}

		logger.Info("Trying fallback selector: %s (confidence: %.2f%%)", alt.Selector, alt.Confidence)
		
		// Test the selector
		// In real implementation, you'd test it on actual page
		// For now, promote it as the new primary
		
		return &alt, nil
	}

	// If no alternatives work, trigger discovery
	logger.Warn("No working fallbacks found, triggering re-discovery for %s", source)
	
	return nil, fmt.Errorf("no working selectors found, need re-discovery")
}

// PromoteSelector promotes a selector to primary for its type
func (sl *SelectorLearner) PromoteSelector(source, elementType, selector string) error {
	// Demote all current primaries
	sl.db.Model(&LearnedSelector{}).
		Where("source = ? AND element_type = ? AND is_primary = true", source, elementType).
		Update("is_primary", false)

	// Promote the new one
	return sl.db.Model(&LearnedSelector{}).
		Where("source = ? AND element_type = ? AND selector = ?", source, elementType, selector).
		Update("is_primary", true).Error
}
