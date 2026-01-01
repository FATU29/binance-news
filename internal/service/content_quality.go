package service

import (
	"crawl-news/internal/model"
	"strings"
	"time"
)

type ContentQualityService struct{}

func NewContentQualityService() *ContentQualityService {
	return &ContentQualityService{}
}

type QualityScore struct {
	Score      float64
	Reasons    []string
	ShouldShow bool
}

// AssessQuality evaluates the quality of a news article
func (s *ContentQualityService) AssessQuality(news *model.News) QualityScore {
	score := 100.0
	reasons := []string{}
	
	// 1. Content Length
	contentLen := len(news.Content)
	if contentLen < 100 {
		score -= 40
		reasons = append(reasons, "Content too short (< 100 chars)")
	} else if contentLen < 200 {
		score -= 20
		reasons = append(reasons, "Content short (< 200 chars)")
	} else if contentLen > 500 && contentLen < 5000 {
		score += 10
		reasons = append(reasons, "Good content length")
	} else if contentLen > 5000 {
		score += 5
		reasons = append(reasons, "Comprehensive content")
	}
	
	// 2. Has Author
	if news.Author == "" {
		score -= 10
		reasons = append(reasons, "No author")
	} else {
		score += 5
		reasons = append(reasons, "Has author")
	}
	
	// 3. Has Image
	if news.ImageURL == "" {
		score -= 5
		reasons = append(reasons, "No image")
	} else {
		score += 3
		reasons = append(reasons, "Has image")
	}
	
	// 4. Title Quality
	titleLen := len(news.Title)
	if titleLen < 10 {
		score -= 20
		reasons = append(reasons, "Title too short")
	} else if titleLen < 20 {
		score -= 10
		reasons = append(reasons, "Title short")
	} else if titleLen > 20 && titleLen < 100 {
		score += 5
		reasons = append(reasons, "Good title length")
	} else if titleLen > 150 {
		score -= 10
		reasons = append(reasons, "Title too long (clickbait?)")
	}
	
	// 5. Check for spam/low quality patterns
	text := strings.ToLower(news.Title + " " + news.Content)
	
	spamWords := []string{
		"click here", "buy now", "limited offer", "act fast",
		"hurry up", "don't miss", "incredible deal", "make money fast",
		"get rich", "miracle", "guaranteed", "free money",
	}
	
	spamCount := 0
	for _, word := range spamWords {
		if strings.Contains(text, word) {
			spamCount++
		}
	}
	
	if spamCount > 0 {
		penalty := float64(spamCount * 15)
		score -= penalty
		reasons = append(reasons, "Contains spam/clickbait keywords")
	}
	
	// 6. Check for excessive caps (SHOUTING)
	upperCount := 0
	for _, char := range news.Title {
		if char >= 'A' && char <= 'Z' {
			upperCount++
		}
	}
	
	if titleLen > 0 {
		upperRatio := float64(upperCount) / float64(titleLen)
		if upperRatio > 0.6 {
			score -= 15
			reasons = append(reasons, "Excessive capitalization")
		}
	}
	
	// 7. Has Tags/Categories
	if len(news.Tags) > 0 {
		score += 5
		reasons = append(reasons, "Has tags/categories")
	}
	
	// 8. Has Summary
	if news.Summary != "" && len(news.Summary) > 50 {
		score += 5
		reasons = append(reasons, "Has summary")
	}
	
	// 9. Freshness bonus (within last 24 hours)
	hoursSincePublished := time.Since(news.PublishedAt).Hours()
	if hoursSincePublished < 24 {
		score += 10
		reasons = append(reasons, "Fresh content (< 24h)")
	} else if hoursSincePublished < 72 {
		score += 5
		reasons = append(reasons, "Recent content (< 3 days)")
	}
	
	// 10. Source credibility
	credibleSources := []string{
		"coindesk", "cointelegraph", "binance", "coinmarketcap",
		"theblock", "decrypt", "bitcoincom",
	}
	
	isCredible := false
	for _, source := range credibleSources {
		if news.Source == source {
			isCredible = true
			break
		}
	}
	
	if isCredible {
		score += 10
		reasons = append(reasons, "From credible source")
	}
	
	// 11. Check for duplicate/repetitive content
	if s.isRepetitive(news.Content) {
		score -= 15
		reasons = append(reasons, "Repetitive content detected")
	}
	
	// Cap score between 0 and 100
	if score > 100 {
		score = 100
	}
	if score < 0 {
		score = 0
	}
	
	// Threshold for showing content: 60 points
	shouldShow := score >= 60
	
	return QualityScore{
		Score:      score,
		Reasons:    reasons,
		ShouldShow: shouldShow,
	}
}

// isRepetitive checks if content has too many repeated phrases
func (s *ContentQualityService) isRepetitive(content string) bool {
	if len(content) < 100 {
		return false
	}
	
	// Simple check: look for repeated sentences
	sentences := strings.Split(content, ".")
	if len(sentences) < 3 {
		return false
	}
	
	sentenceMap := make(map[string]int)
	for _, sentence := range sentences {
		sentence = strings.TrimSpace(strings.ToLower(sentence))
		if len(sentence) > 10 {
			sentenceMap[sentence]++
		}
	}
	
	// If more than 30% sentences are duplicated, consider repetitive
	duplicateCount := 0
	for _, count := range sentenceMap {
		if count > 1 {
			duplicateCount += count - 1
		}
	}
	
	if len(sentences) > 0 {
		duplicateRatio := float64(duplicateCount) / float64(len(sentences))
		return duplicateRatio > 0.3
	}
	
	return false
}

// FilterHighQuality returns only high quality news
func (s *ContentQualityService) FilterHighQuality(newsList []*model.News) []*model.News {
	filtered := []*model.News{}
	
	for _, news := range newsList {
		quality := s.AssessQuality(news)
		if quality.ShouldShow {
			filtered = append(filtered, news)
		}
	}
	
	return filtered
}

