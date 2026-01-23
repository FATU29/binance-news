package model

import (
	"database/sql/driver"
	"encoding/json"
	"time"

	"gorm.io/gorm"
)

type News struct {
	ID          string      `json:"id" gorm:"primaryKey;size:100"`
	Title       string      `json:"title" gorm:"size:500;not null;index"`
	Content     string      `json:"content" gorm:"type:text"`
	Summary     string      `json:"summary" gorm:"type:text"`
	Author      string      `json:"author" gorm:"size:200"`
	Source      string      `json:"source" gorm:"size:100;not null;index"`
	SourceURL   string      `json:"source_url" gorm:"size:500;uniqueIndex"`
	ImageURL    string      `json:"image_url" gorm:"size:500"`
	Category    string      `json:"category" gorm:"size:100;index"`
	Tags        StringArray `json:"tags" gorm:"type:jsonb"`
	PublishedAt time.Time   `json:"published_at" gorm:"index"`
	CrawledAt   time.Time   `json:"crawled_at" gorm:"index"`
	Language    string      `json:"language" gorm:"size:10"`

	// AI Analysis fields
	Sentiment    *SentimentAnalysis `json:"sentiment,omitempty" gorm:"type:jsonb;serializer:json"`
	RelatedPairs StringArray        `json:"related_pairs,omitempty" gorm:"type:jsonb"`
	PriceImpact  *PriceImpact       `json:"price_impact,omitempty" gorm:"type:jsonb;serializer:json"`
	AIAnalyzed   bool               `json:"ai_analyzed" gorm:"default:false;index"`
	AnalyzedAt   *time.Time         `json:"analyzed_at,omitempty"`

	// Parsing metadata
	ParsingMethod     string  `json:"parsing_method,omitempty" gorm:"size:20"`               // "rule-based", "ai", "fallback"
	ParsingConfidence float64 `json:"parsing_confidence,omitempty" gorm:"type:decimal(3,2)"` // 0.0 to 1.0

	// GORM fields
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`
}

// StringArray is a custom type for storing string slices in PostgreSQL as JSONB
type StringArray []string

// Scan implements sql.Scanner interface for reading from database
func (s *StringArray) Scan(value interface{}) error {
	if value == nil {
		*s = []string{}
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return nil
	}
	return json.Unmarshal(bytes, s)
}

// Value implements driver.Valuer interface for writing to database
func (s StringArray) Value() (driver.Value, error) {
	if s == nil {
		return nil, nil
	}
	return json.Marshal(s)
}

// SentimentAnalysis represents AI sentiment analysis result
type SentimentAnalysis struct {
	Score      float64  `json:"score"`      // -1.0 (very negative) to 1.0 (very positive)
	Label      string   `json:"label"`      // "positive", "negative", "neutral"
	Confidence float64  `json:"confidence"` // 0.0 to 1.0
	Keywords   []string `json:"keywords,omitempty"`
	Reasoning  string   `json:"reasoning,omitempty"` // AI explanation
}

// Scan implements sql.Scanner interface for reading from database
func (s *SentimentAnalysis) Scan(value interface{}) error {
	if value == nil {
		return nil // Keep as nil pointer
	}

	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return nil
	}

	if len(bytes) == 0 || string(bytes) == "null" {
		return nil // Keep as nil pointer
	}

	// Create new instance if nil
	if s == nil {
		*s = SentimentAnalysis{}
	}

	return json.Unmarshal(bytes, s)
}

// Value implements driver.Valuer interface for writing to database
func (s *SentimentAnalysis) Value() (driver.Value, error) {
	if s == nil {
		return nil, nil
	}
	if s.Label == "" && s.Score == 0 && s.Confidence == 0 {
		return nil, nil
	}
	return json.Marshal(s)
}

// PriceImpact represents potential price impact from news
type PriceImpact struct {
	Direction  string  `json:"direction"` // "up", "down", "neutral"
	Magnitude  string  `json:"magnitude"` // "high", "medium", "low"
	TimeFrame  string  `json:"timeframe"` // "short", "medium", "long"
	Confidence float64 `json:"confidence"`
	Reasoning  string  `json:"reasoning,omitempty"`
}

// Scan implements sql.Scanner interface for reading from database
func (p *PriceImpact) Scan(value interface{}) error {
	if value == nil {
		return nil // Keep as nil pointer
	}

	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return nil
	}

	if len(bytes) == 0 || string(bytes) == "null" {
		return nil // Keep as nil pointer
	}

	// Create new instance if nil
	if p == nil {
		*p = PriceImpact{}
	}

	return json.Unmarshal(bytes, p)
}

// Value implements driver.Valuer interface for writing to database
func (p *PriceImpact) Value() (driver.Value, error) {
	if p == nil {
		return nil, nil
	}
	if p.Direction == "" && p.Magnitude == "" && p.TimeFrame == "" && p.Confidence == 0 {
		return nil, nil
	}
	return json.Marshal(p)
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
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	BaseURL     string    `json:"base_url"`
	Enabled     bool      `json:"enabled"`
	Priority    int       `json:"priority"`   // Higher = more important
	CrawlFreq   int       `json:"crawl_freq"` // Minutes between crawls
	LastCrawled time.Time `json:"last_crawled"`
	Selectors   Selector  `json:"selectors"`
	Categories  []string  `json:"categories"` // Types of news this source provides
	Language    string    `json:"language"`
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

// NewsFilter for advanced filtering
type NewsFilter struct {
	StartDate     *time.Time `json:"start_date,omitempty"`
	EndDate       *time.Time `json:"end_date,omitempty"`
	Sources       []string   `json:"sources,omitempty"`
	Categories    []string   `json:"categories,omitempty"`
	TradingPairs  []string   `json:"trading_pairs,omitempty"`
	Sentiment     string     `json:"sentiment,omitempty"` // "positive", "negative", "neutral"
	MinScore      *float64   `json:"min_score,omitempty"`
	AIAnalyzed    *bool      `json:"ai_analyzed,omitempty"`
	Language      string     `json:"language,omitempty"`
	ParsingMethod string     `json:"parsing_method,omitempty"` // "rule-based", "ai", "fallback"
}

// NewsSummary for listing views
type NewsSummary struct {
	ID           string             `json:"id"`
	Title        string             `json:"title"`
	Summary      string             `json:"summary"`
	Source       string             `json:"source"`
	ImageURL     string             `json:"image_url"`
	PublishedAt  time.Time          `json:"published_at"`
	Sentiment    *SentimentAnalysis `json:"sentiment,omitempty"`
	RelatedPairs []string           `json:"related_pairs,omitempty"`
	PriceImpact  *PriceImpact       `json:"price_impact,omitempty"`
}

// AIAnalysisRequest for requesting AI analysis
type AIAnalysisRequest struct {
	NewsID       string   `json:"news_id" binding:"required"`
	TradingPairs []string `json:"trading_pairs,omitempty"`
	AnalysisType string   `json:"analysis_type"` // "sentiment", "price_impact", "full"
}
