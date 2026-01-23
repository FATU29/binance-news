package service

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"crawl-news/internal/config"
	"crawl-news/internal/model"
	"crawl-news/pkg/logger"
)

// AIHTMLParser handles AI-based HTML content parsing
type AIHTMLParser struct {
	aiBaseURL      string
	client         *http.Client
	cfg            *config.AIServiceConfig
	enabled        bool
	forceAIParsing bool // If true, always use AI, never fallback to rule-based
}

// NewAIHTMLParser creates a new AI HTML parser service
func NewAIHTMLParser(cfg *config.AIServiceConfig) *AIHTMLParser {
	aiURL := cfg.BaseURL
	if aiURL == "" {
		aiURL = "http://localhost:8000"
	}

	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 120 * time.Second // Longer timeout for AI parsing (2 minutes)
	}

	return &AIHTMLParser{
		aiBaseURL:      aiURL,
		client: &http.Client{
			Timeout: timeout,
		},
		cfg:            cfg,
		enabled:        true, // Enable by default, can be disabled via config
		forceAIParsing: cfg.ForceAIParsing,
	}
}

// ParseHTMLRequest represents the request to AI HTML parser
type ParseHTMLRequest struct {
	HTMLContent string `json:"html_content"`
	URL         string `json:"url,omitempty"`
	SourceName  string `json:"source_name,omitempty"`
}

// ParseHTMLResponse represents the response from AI HTML parser
type ParseHTMLResponse struct {
	Success   bool                 `json:"success"`
	Article   *ParsedArticle       `json:"article,omitempty"`
	Confidence float64            `json:"confidence"`
	Method    string              `json:"method"`
	Error     string              `json:"error,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// ParsedArticle represents parsed article data from AI
type ParsedArticle struct {
	Title       string   `json:"title"`
	Content     string   `json:"content"`
	Summary     string   `json:"summary,omitempty"`
	Author      string   `json:"author,omitempty"`
	PublishedAt string   `json:"published_at,omitempty"`
	ImageURL    string   `json:"image_url,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Language    string   `json:"language,omitempty"`
}

// ParseHTML parses HTML content using AI service
func (p *AIHTMLParser) ParseHTML(ctx context.Context, htmlContent string, url string, sourceName string) (*ParsedArticle, error) {
	if !p.enabled {
		return nil, fmt.Errorf("AI HTML parser is disabled")
	}

	if len(htmlContent) < 100 {
		return nil, fmt.Errorf("HTML content too short (minimum 100 characters)")
	}

	// Truncate HTML if too long (AI service has token limits)
	// Keep first 50KB for context
	maxHTMLSize := 50 * 1024
	if len(htmlContent) > maxHTMLSize {
		htmlContent = htmlContent[:maxHTMLSize]
		logger.Info("Truncated HTML content to %d bytes for AI parsing", maxHTMLSize)
	}

	reqBody := ParseHTMLRequest{
		HTMLContent: htmlContent,
		URL:         url,
		SourceName:  sourceName,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Make HTTP request to AI service
	apiURL := fmt.Sprintf("%s/api/v1/ai/parse-html", p.aiBaseURL)
	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	logger.Info("Calling AI HTML parser for URL: %s (source: %s)", url, sourceName)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call AI HTML parser: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes := make([]byte, 1024)
		resp.Body.Read(bodyBytes)
		return nil, fmt.Errorf("AI HTML parser returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	// Read response body
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Parse response
	var aiResp ParseHTMLResponse
	if err := json.Unmarshal(bodyBytes, &aiResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if !aiResp.Success {
		return nil, fmt.Errorf("AI parsing failed: %s", aiResp.Error)
	}

	if aiResp.Article == nil {
		return nil, fmt.Errorf("AI parsing returned no article data")
	}

	logger.Info("AI HTML parsing successful (confidence: %.2f, method: %s)", aiResp.Confidence, aiResp.Method)

	return aiResp.Article, nil
}

// ParseHTMLToNews converts parsed article to News model
func (p *AIHTMLParser) ParseHTMLToNews(ctx context.Context, htmlContent string, url string, sourceName string) (*model.News, error) {
	// Use ParseHTMLWithMetadata to get full response with metadata
	parsed, resp, err := p.ParseHTMLWithMetadata(ctx, htmlContent, url, sourceName)
	if err != nil {
		return nil, err
	}

	// Get parsing metadata from response
	parsingMethod := "ai"
	parsingConfidence := 0.85 // Default confidence
	
	if resp != nil {
		parsingMethod = resp.Method
		parsingConfidence = resp.Confidence
	}

	// Parse published date
	var publishedAt time.Time
	if parsed.PublishedAt != "" {
		// Try multiple date formats
		formats := []string{
			time.RFC3339,
			"2006-01-02T15:04:05Z07:00",
			"2006-01-02",
			"2006-01-02 15:04:05",
		}
		for _, format := range formats {
			if t, err := time.Parse(format, parsed.PublishedAt); err == nil {
				publishedAt = t
				break
			}
		}
		if publishedAt.IsZero() {
			publishedAt = time.Now()
		}
	} else {
		publishedAt = time.Now()
	}

	// Generate ID from URL
	hash := md5.Sum([]byte(url))
	sourcePrefix := sourceName
	if len(sourceName) > 2 {
		sourcePrefix = sourceName[:2]
	}
	id := fmt.Sprintf("%s-%s", sourcePrefix, hex.EncodeToString(hash[:])[:16])

	// Determine category from tags or source
	category := "crypto" // Default
	if len(parsed.Tags) > 0 {
		// Check if tags contain category keywords
		for _, tag := range parsed.Tags {
			tagLower := strings.ToLower(tag)
			if strings.Contains(tagLower, "exchange") || strings.Contains(tagLower, "binance") {
				category = "exchange"
				break
			} else if strings.Contains(tagLower, "defi") {
				category = "defi"
				break
			} else if strings.Contains(tagLower, "nft") {
				category = "nft"
				break
			}
		}
	}
	if strings.Contains(strings.ToLower(sourceName), "binance") {
		category = "exchange"
	}

	// Normalize image URL (convert relative to absolute)
	imageURL := parsed.ImageURL
	if imageURL != "" {
		imageURL = p.normalizeImageURL(imageURL, url)
	}

	news := &model.News{
		ID:                id,
		Title:             parsed.Title,
		Content:           parsed.Content,
		Summary:           parsed.Summary,
		Author:            parsed.Author,
		Source:            sourceName,
		SourceURL:         url,
		ImageURL:          imageURL,
		Tags:              parsed.Tags,
		Language:          parsed.Language,
		PublishedAt:       publishedAt,
		CrawledAt:         time.Now(),
		Category:          category,
		ParsingMethod:     parsingMethod,
		ParsingConfidence: parsingConfidence,
	}

	return news, nil
}

// ParseHTMLWithMetadata parses HTML and returns both article and metadata
func (p *AIHTMLParser) ParseHTMLWithMetadata(ctx context.Context, htmlContent string, url string, sourceName string) (*ParsedArticle, *ParseHTMLResponse, error) {
	if !p.enabled {
		return nil, nil, fmt.Errorf("AI HTML parser is disabled")
	}

	if len(htmlContent) < 100 {
		return nil, nil, fmt.Errorf("HTML content too short (minimum 100 characters)")
	}

	// Truncate HTML if too long
	maxHTMLSize := 50 * 1024
	if len(htmlContent) > maxHTMLSize {
		htmlContent = htmlContent[:maxHTMLSize]
		logger.Info("Truncated HTML content to %d bytes for AI parsing", maxHTMLSize)
	}

	reqBody := ParseHTMLRequest{
		HTMLContent: htmlContent,
		URL:         url,
		SourceName:  sourceName,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	apiURL := fmt.Sprintf("%s/api/v1/ai/parse-html", p.aiBaseURL)
	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to call AI HTML parser: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes := make([]byte, 1024)
		resp.Body.Read(bodyBytes)
		return nil, nil, fmt.Errorf("AI HTML parser returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var aiResp ParseHTMLResponse
	if err := json.Unmarshal(bodyBytes, &aiResp); err != nil {
		return nil, nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if !aiResp.Success || aiResp.Article == nil {
		return nil, &aiResp, fmt.Errorf("AI parsing failed: %s", aiResp.Error)
	}

	return aiResp.Article, &aiResp, nil
}

// SetEnabled enables or disables the AI HTML parser
func (p *AIHTMLParser) SetEnabled(enabled bool) {
	p.enabled = enabled
}

// IsEnabled returns whether the AI HTML parser is enabled
func (p *AIHTMLParser) IsEnabled() bool {
	return p.enabled
}

// IsForceAIParsing returns whether AI-only parsing is forced
func (p *AIHTMLParser) IsForceAIParsing() bool {
	return p.forceAIParsing
}

// normalizeImageURL converts relative URLs to absolute URLs
func (p *AIHTMLParser) normalizeImageURL(imageURL string, baseURLStr string) string {
	if imageURL == "" {
		return ""
	}

	// Parse base URL
	baseURL, err := url.Parse(baseURLStr)
	if err != nil {
		logger.Warn("Failed to parse base URL for image normalization: %s", baseURLStr)
		return imageURL // Return original if can't parse
	}

	// Remove query parameters that might cause issues
	if idx := strings.Index(imageURL, "?"); idx > 0 {
		imageURL = imageURL[:idx]
	}

	// Handle protocol-relative URLs
	if strings.HasPrefix(imageURL, "//") {
		return "https:" + imageURL
	}

	// Handle absolute URLs
	if strings.HasPrefix(imageURL, "http://") || strings.HasPrefix(imageURL, "https://") {
		return imageURL
	}

	// Handle relative URLs
	if strings.HasPrefix(imageURL, "/") {
		return baseURL.Scheme + "://" + baseURL.Host + imageURL
	}

	// Handle relative URLs without leading slash
	return baseURL.Scheme + "://" + baseURL.Host + "/" + imageURL
}
