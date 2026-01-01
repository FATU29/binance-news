package service

import (
	"context"
	"time"

	"crawl-news/internal/repository"
	"crawl-news/pkg/logger"

	"gorm.io/gorm"
)

type StructureMonitor struct {
	db   *gorm.DB
	repo *repository.NewsRepository
}

func NewStructureMonitor(db *gorm.DB, repo *repository.NewsRepository) *StructureMonitor {
	return &StructureMonitor{
		db:   db,
		repo: repo,
	}
}

// SelectorPattern tracks selector usage and success rates
type SelectorPattern struct {
	ID           int       `gorm:"primaryKey"`
	Source       string    `gorm:"size:100;not null;index"`
	ElementType  string    `gorm:"size:50;not null"`
	Selector     string    `gorm:"size:500;not null"`
	SuccessCount int       `gorm:"default:0"`
	FailureCount int       `gorm:"default:0"`
	SuccessRate  float64   `gorm:"default:0"`
	LastUsed     time.Time `gorm:"index"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// StructureAlert alerts when selectors stop working
type StructureAlert struct {
	ID           int       `gorm:"primaryKey"`
	Source       string    `gorm:"size:100;not null;index"`
	Selector     string    `gorm:"size:500"`
	FailureCount int       `gorm:"default:0"`
	Severity     string    `gorm:"size:20"` // "warning", "critical"
	Notified     bool      `gorm:"default:false"`
	Resolved     bool      `gorm:"default:false;index"`
	CreatedAt    time.Time `gorm:"index"`
	ResolvedAt   *time.Time
}

// RecordSuccess records a successful selector extraction
func (s *StructureMonitor) RecordSuccess(source, elementType, selector string) error {
	var pattern SelectorPattern

	err := s.db.Where("source = ? AND element_type = ? AND selector = ?",
		source, elementType, selector).
		First(&pattern).Error

	if err == gorm.ErrRecordNotFound {
		// Create new pattern
		pattern = SelectorPattern{
			Source:       source,
			ElementType:  elementType,
			Selector:     selector,
			SuccessCount: 1,
			FailureCount: 0,
			LastUsed:     time.Now(),
		}
		pattern.SuccessRate = 100.0
		return s.db.Create(&pattern).Error
	}

	// Update existing pattern
	pattern.SuccessCount++
	pattern.LastUsed = time.Now()
	pattern.SuccessRate = float64(pattern.SuccessCount) /
		float64(pattern.SuccessCount+pattern.FailureCount) * 100

	return s.db.Save(&pattern).Error
}

// RecordFailure records a failed selector extraction
func (s *StructureMonitor) RecordFailure(source, elementType, selector string) error {
	var pattern SelectorPattern

	err := s.db.Where("source = ? AND element_type = ? AND selector = ?",
		source, elementType, selector).
		First(&pattern).Error

	if err == gorm.ErrRecordNotFound {
		// Create new pattern with failure
		pattern = SelectorPattern{
			Source:       source,
			ElementType:  elementType,
			Selector:     selector,
			SuccessCount: 0,
			FailureCount: 1,
			LastUsed:     time.Now(),
		}
		pattern.SuccessRate = 0.0
		s.db.Create(&pattern)

		// Check if we should create alert
		s.checkAndCreateAlert(source, selector, 1)
		return nil
	}

	// Update existing pattern
	pattern.FailureCount++
	pattern.LastUsed = time.Now()
	pattern.SuccessRate = float64(pattern.SuccessCount) /
		float64(pattern.SuccessCount+pattern.FailureCount) * 100

	s.db.Save(&pattern)

	// Check if we should create alert
	if pattern.SuccessRate < 50 && pattern.FailureCount > 3 {
		s.checkAndCreateAlert(source, selector, pattern.FailureCount)
	}

	return nil
}

// checkAndCreateAlert creates an alert if needed
func (s *StructureMonitor) checkAndCreateAlert(source, selector string, failures int) {
	// Check if alert already exists
	var existingAlert StructureAlert
	err := s.db.Where("source = ? AND selector = ? AND resolved = false",
		source, selector).
		First(&existingAlert).Error

	if err == gorm.ErrRecordNotFound {
		// Create new alert
		severity := "warning"
		if failures > 10 {
			severity = "critical"
		}

		alert := StructureAlert{
			Source:       source,
			Selector:     selector,
			FailureCount: failures,
			Severity:     severity,
			Notified:     false,
			Resolved:     false,
		}

		if err := s.db.Create(&alert).Error; err != nil {
			logger.Error("Failed to create structure alert: %v", err)
		} else {
			logger.Warn("Created structure alert for %s: %s (failures: %d)",
				source, selector, failures)

			// Send notification
			s.sendNotification(alert)
		}
	}
}

// sendNotification sends notification about structure changes
func (s *StructureMonitor) sendNotification(alert StructureAlert) {
	// TODO: Implement notification (email, Slack, Discord, etc.)
	logger.Warn("ALERT: Structure change detected for %s - %s (severity: %s)",
		alert.Source, alert.Selector, alert.Severity)

	// You can integrate with:
	// - Email service
	// - Slack webhook
	// - Discord webhook
	// - PagerDuty
	// - Custom alerting system
}

// GetBestSelectors returns the most successful selectors for a source/element type
func (s *StructureMonitor) GetBestSelectors(source, elementType string, limit int) ([]SelectorPattern, error) {
	var patterns []SelectorPattern

	err := s.db.Where("source = ? AND element_type = ?", source, elementType).
		Order("success_rate DESC, last_used DESC").
		Limit(limit).
		Find(&patterns).Error

	return patterns, err
}

// CheckHealth returns all unresolved alerts
func (s *StructureMonitor) CheckHealth() ([]StructureAlert, error) {
	var alerts []StructureAlert

	err := s.db.Where("resolved = false").
		Order("severity DESC, created_at DESC").
		Find(&alerts).Error

	return alerts, err
}

// ResolveAlert marks an alert as resolved
func (s *StructureMonitor) ResolveAlert(alertID int) error {
	now := time.Now()
	return s.db.Model(&StructureAlert{}).
		Where("id = ?", alertID).
		Updates(map[string]interface{}{
			"resolved":    true,
			"resolved_at": &now,
		}).Error
}

// GetSourceHealth returns health metrics for a source
func (s *StructureMonitor) GetSourceHealth(source string) (map[string]interface{}, error) {
	var patterns []SelectorPattern

	err := s.db.Where("source = ?", source).Find(&patterns).Error
	if err != nil {
		return nil, err
	}

	totalSuccess := 0
	totalFailure := 0
	var avgSuccessRate float64

	for _, pattern := range patterns {
		totalSuccess += pattern.SuccessCount
		totalFailure += pattern.FailureCount
		avgSuccessRate += pattern.SuccessRate
	}

	if len(patterns) > 0 {
		avgSuccessRate = avgSuccessRate / float64(len(patterns))
	}

	// Get unresolved alerts for this source
	var alertCount int64
	s.db.Model(&StructureAlert{}).
		Where("source = ? AND resolved = false", source).
		Count(&alertCount)

	health := "healthy"
	if avgSuccessRate < 50 || alertCount > 5 {
		health = "critical"
	} else if avgSuccessRate < 70 || alertCount > 2 {
		health = "warning"
	}

	return map[string]interface{}{
		"source":            source,
		"health":            health,
		"total_success":     totalSuccess,
		"total_failure":     totalFailure,
		"avg_success_rate":  avgSuccessRate,
		"unresolved_alerts": alertCount,
		"patterns_tracked":  len(patterns),
	}, nil
}

// CleanupOldPatterns removes patterns not used in the last 30 days
func (s *StructureMonitor) CleanupOldPatterns(ctx context.Context) error {
	thirtyDaysAgo := time.Now().AddDate(0, 0, -30)

	result := s.db.Where("last_used < ? AND success_count = 0", thirtyDaysAgo).
		Delete(&SelectorPattern{})

	if result.Error != nil {
		return result.Error
	}

	logger.Info("Cleaned up %d old selector patterns", result.RowsAffected)
	return nil
}

// GetStatistics returns overall statistics
func (s *StructureMonitor) GetStatistics() (map[string]interface{}, error) {
	var totalPatterns int64
	s.db.Model(&SelectorPattern{}).Count(&totalPatterns)

	var activeAlerts int64
	s.db.Model(&StructureAlert{}).Where("resolved = false").Count(&activeAlerts)

	var criticalAlerts int64
	s.db.Model(&StructureAlert{}).
		Where("resolved = false AND severity = ?", "critical").
		Count(&criticalAlerts)

	// Get sources with issues
	var sourcesWithIssues []string
	s.db.Model(&StructureAlert{}).
		Where("resolved = false").
		Distinct("source").
		Pluck("source", &sourcesWithIssues)

	return map[string]interface{}{
		"total_patterns":      totalPatterns,
		"active_alerts":       activeAlerts,
		"critical_alerts":     criticalAlerts,
		"sources_with_issues": sourcesWithIssues,
	}, nil
}
