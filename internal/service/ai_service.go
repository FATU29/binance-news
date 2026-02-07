package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"crawl-news/internal/config"
	"crawl-news/internal/model"
	"crawl-news/internal/repository"
	"crawl-news/pkg/logger"
)

// AIService handles AI-based analysis of news
type AIService struct {
	newsRepo  *repository.NewsRepository
	aiBaseURL string
	client    *http.Client
	cfg       *config.AIServiceConfig
}

func NewAIService(newsRepo *repository.NewsRepository, cfg *config.AIServiceConfig) *AIService {
	aiURL := cfg.BaseURL
	if aiURL == "" {
		aiURL = os.Getenv("AI_SERVICE_URL")
		if aiURL == "" {
			aiURL = "http://localhost:8000"
		}
	}

	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	return &AIService{
		newsRepo:  newsRepo,
		aiBaseURL: aiURL,
		client: &http.Client{
			Timeout: timeout,
		},
		cfg: cfg,
	}
}

// AnalyzeSentiment performs sentiment analysis on a news article
func (s *AIService) AnalyzeSentiment(ctx context.Context, newsID string) (*model.SentimentAnalysis, error) {
	news, err := s.newsRepo.FindByID(ctx, newsID)
	if err != nil {
		return nil, fmt.Errorf("failed to find news: %w", err)
	}

	logger.Info("Analyzing sentiment for news: %s", newsID)

	// Try to call real AI service first
	sentiment, err := s.callAISentimentAPI(ctx, news)
	if err != nil {
		logger.Warn("AI service call failed, using fallback: %v", err)
		// Fallback to mock analysis
		sentiment = s.mockSentimentAnalysis(news)
	}

	// Update news with sentiment
	news.Sentiment = sentiment
	news.AIAnalyzed = true
	now := time.Now()
	news.AnalyzedAt = &now

	if err := s.newsRepo.Update(ctx, news); err != nil {
		return nil, fmt.Errorf("failed to update news: %w", err)
	}

	return sentiment, nil
}

// callAISentimentAPI calls the Python AI service for sentiment analysis
func (s *AIService) callAISentimentAPI(ctx context.Context, news *model.News) (*model.SentimentAnalysis, error) {
	// Prepare request body
	text := news.Title
	if news.Content != "" {
		text += " " + news.Content
	}
	if len(text) > 2000 {
		text = text[:2000] // Truncate if too long
	}

	reqBody := map[string]interface{}{
		"text": text,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Make HTTP request
	url := fmt.Sprintf("%s/api/v1/sentiment/analyze", s.aiBaseURL)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call AI service: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes := make([]byte, 1024)
		resp.Body.Read(bodyBytes)
		return nil, fmt.Errorf("AI service returned status: %d, body: %s", resp.StatusCode, string(bodyBytes))
	}

	// Read response body
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Parse response - try both formats
	var aiResp struct {
		SentimentLabel  string  `json:"sentiment_label"`
		SentimentScore  float64 `json:"sentiment_score"`
		Confidence      float64 `json:"confidence"`
		ModelVersion    string  `json:"model_version,omitempty"`
	}

	// Try to decode as direct response first
	if err := json.Unmarshal(bodyBytes, &aiResp); err != nil {
		// Try wrapped response format
		var wrappedResp struct {
			Status string `json:"status"`
			Data   struct {
				SentimentLabel  string  `json:"sentiment_label"`
				SentimentScore  float64 `json:"sentiment_score"`
				Confidence      float64 `json:"confidence"`
				ModelVersion    string  `json:"model_version,omitempty"`
			} `json:"data"`
		}
		if err2 := json.Unmarshal(bodyBytes, &wrappedResp); err2 != nil {
			return nil, fmt.Errorf("failed to parse response: %w (also tried wrapped format: %v), body: %s", err, err2, string(bodyBytes))
		}
		aiResp = wrappedResp.Data
	}

	// Convert to model
	// Map AI service labels to our labels
	label := strings.ToLower(aiResp.SentimentLabel)
	if label == "bullish" {
		label = "positive"
	} else if label == "bearish" {
		label = "negative"
	}

	// Convert score from 0-1 to -1 to 1 range
	score := (aiResp.SentimentScore - 0.5) * 2.0

	sentiment := &model.SentimentAnalysis{
		Label:      label,
		Score:      score,
		Confidence: aiResp.Confidence,
		Reasoning:  fmt.Sprintf("AI analysis using %s", aiResp.ModelVersion),
	}

	return sentiment, nil
}

// AnalyzePriceImpact analyzes potential price impact of news
func (s *AIService) AnalyzePriceImpact(ctx context.Context, newsID string, tradingPairs []string) (*model.PriceImpact, error) {
	news, err := s.newsRepo.FindByID(ctx, newsID)
	if err != nil {
		return nil, fmt.Errorf("failed to find news: %w", err)
	}

	logger.Info("Analyzing price impact for news: %s", newsID)

	// TODO: Integrate with real AI model
	// This should analyze news content and predict price impact
	priceImpact := s.mockPriceImpactAnalysis(news, tradingPairs)

	// Update news
	news.PriceImpact = priceImpact
	news.RelatedPairs = tradingPairs
	news.AIAnalyzed = true
	now := time.Now()
	news.AnalyzedAt = &now

	if err := s.newsRepo.Update(ctx, news); err != nil {
		return nil, fmt.Errorf("failed to update news: %w", err)
	}

	return priceImpact, nil
}

// FullAnalysis performs complete AI analysis (sentiment + price impact)
func (s *AIService) FullAnalysis(ctx context.Context, newsID string, tradingPairs []string) (*model.News, error) {
	news, err := s.newsRepo.FindByID(ctx, newsID)
	if err != nil {
		return nil, fmt.Errorf("failed to find news: %w", err)
	}

	logger.Info("Performing full AI analysis for news: %s", newsID)

	// Analyze sentiment using real AI service, fallback to mock
	sentiment, err := s.callAISentimentAPI(ctx, news)
	if err != nil {
		logger.Warn("AI service call failed in FullAnalysis, using fallback: %v", err)
		sentiment = s.mockSentimentAnalysis(news)
	}
	news.Sentiment = sentiment

	// Analyze price impact
	priceImpact := s.mockPriceImpactAnalysis(news, tradingPairs)
	news.PriceImpact = priceImpact
	news.RelatedPairs = tradingPairs

	// Extract trading pairs from content if not provided
	if len(tradingPairs) == 0 {
		news.RelatedPairs = s.extractTradingPairs(news)
	}

	news.AIAnalyzed = true
	now := time.Now()
	news.AnalyzedAt = &now

	if err := s.newsRepo.Update(ctx, news); err != nil {
		return nil, fmt.Errorf("failed to update news: %w", err)
	}

	return news, nil
}

// BatchAnalyze analyzes multiple news articles
func (s *AIService) BatchAnalyze(ctx context.Context, newsIDs []string) (map[string]*model.News, error) {
	results := make(map[string]*model.News)

	for _, newsID := range newsIDs {
		news, err := s.FullAnalysis(ctx, newsID, nil)
		if err != nil {
			logger.Error("Failed to analyze news %s: %v", newsID, err)
			continue
		}
		results[newsID] = news
	}

	return results, nil
}

// ReanalyzeAll re-runs sentiment analysis on all articles that used keyword fallback
func (s *AIService) ReanalyzeAll(ctx context.Context) (int, int, error) {
	// Use a background context with generous timeout since this is a long-running batch job
	bgCtx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	// Get all analyzed news
	aiAnalyzed := true
	filter := &model.NewsFilter{
		AIAnalyzed: &aiAnalyzed,
	}
	allNews, err := s.newsRepo.FindWithFilter(bgCtx, filter)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to fetch news: %w", err)
	}

	total := 0
	success := 0
	for _, news := range allNews {
		// Re-analyze articles that used keyword fallback or have no reasoning
		if news.Sentiment != nil && strings.Contains(news.Sentiment.Reasoning, "keyword analysis") {
			total++
			sentiment, err := s.callAISentimentAPI(bgCtx, &news)
			if err != nil {
				logger.Error("Failed to re-analyze news %s: %v", news.ID, err)
				continue
			}
			news.Sentiment = sentiment
			now := time.Now()
			news.AnalyzedAt = &now
			if err := s.newsRepo.Update(bgCtx, &news); err != nil {
				logger.Error("Failed to update news %s: %v", news.ID, err)
				continue
			}
			success++
			logger.Info("Re-analyzed news %s: %s -> %s (score: %.2f)", news.ID, "keyword", sentiment.Label, sentiment.Score)
		}
	}

	return total, success, nil
}

// GetAnalyzedNews retrieves news with AI analysis
func (s *AIService) GetAnalyzedNews(ctx context.Context, filter *model.NewsFilter) ([]model.News, error) {
	// This would filter news that have been analyzed
	// For now, delegate to repository with filter
	return s.newsRepo.FindWithFilter(ctx, filter)
}

// --- Mock Analysis Functions (Replace with real AI integration) ---

func (s *AIService) mockSentimentAnalysis(news *model.News) *model.SentimentAnalysis {
	text := strings.ToLower(news.Title + " " + news.Content)

	// Simple keyword-based sentiment
	positiveKeywords := []string{"surge", "rally", "bullish", "gain", "rise", "profit", "success", "breakthrough", "adoption", "partnership"}
	negativeKeywords := []string{"crash", "fall", "bearish", "loss", "decline", "scam", "hack", "regulation", "ban", "warning"}

	positiveCount := 0
	negativeCount := 0
	foundKeywords := []string{}

	for _, keyword := range positiveKeywords {
		if strings.Contains(text, keyword) {
			positiveCount++
			foundKeywords = append(foundKeywords, keyword)
		}
	}

	for _, keyword := range negativeKeywords {
		if strings.Contains(text, keyword) {
			negativeCount++
			foundKeywords = append(foundKeywords, keyword)
		}
	}

	score := float64(positiveCount-negativeCount) / 10.0
	if score > 1.0 {
		score = 1.0
	}
	if score < -1.0 {
		score = -1.0
	}

	label := "neutral"
	if score > 0.3 {
		label = "positive"
	} else if score < -0.3 {
		label = "negative"
	}

	confidence := 0.7
	if positiveCount+negativeCount > 3 {
		confidence = 0.9
	}

	reasoning := fmt.Sprintf("Based on keyword analysis, found %d positive and %d negative indicators", positiveCount, negativeCount)

	return &model.SentimentAnalysis{
		Score:      score,
		Label:      label,
		Confidence: confidence,
		Keywords:   foundKeywords,
		Reasoning:  reasoning,
	}
}

func (s *AIService) mockPriceImpactAnalysis(news *model.News, tradingPairs []string) *model.PriceImpact {
	// Use sentiment to predict price impact
	direction := "neutral"
	magnitude := "low"

	if news.Sentiment != nil {
		if news.Sentiment.Score > 0.5 {
			direction = "up"
			magnitude = "high"
		} else if news.Sentiment.Score > 0.2 {
			direction = "up"
			magnitude = "medium"
		} else if news.Sentiment.Score < -0.5 {
			direction = "down"
			magnitude = "high"
		} else if news.Sentiment.Score < -0.2 {
			direction = "down"
			magnitude = "medium"
		}
	}

	timeframe := "short"
	text := strings.ToLower(news.Title + " " + news.Content)
	if strings.Contains(text, "regulation") || strings.Contains(text, "policy") {
		timeframe = "long"
	} else if strings.Contains(text, "partnership") || strings.Contains(text, "adoption") {
		timeframe = "medium"
	}

	reasoning := fmt.Sprintf("Predicted %s movement in %s term based on sentiment analysis", direction, timeframe)

	return &model.PriceImpact{
		Direction:  direction,
		Magnitude:  magnitude,
		TimeFrame:  timeframe,
		Confidence: 0.75,
		Reasoning:  reasoning,
	}
}

func (s *AIService) extractTradingPairs(news *model.News) []string {
	text := strings.ToUpper(news.Title + " " + news.Content)
	pairs := []string{}
	seenPairs := make(map[string]bool)

	// 1. Get all Binance pairs (cached)
	allPairs := s.getAllBinancePairs()

	// 2. Direct pair mentions (BTCUSDT, BTC/USDT, BTC-USDT)
	for _, pair := range allPairs {
		if len(pair) < 6 {
			continue
		}

		// Extract base and quote
		base := pair[:len(pair)-4]
		quote := pair[len(pair)-4:]

		patterns := []string{
			pair,               // BTCUSDT
			base + "/" + quote, // BTC/USDT
			base + "-" + quote, // BTC-USDT
			base + " " + quote, // BTC USDT
		}

		for _, pattern := range patterns {
			if strings.Contains(text, pattern) {
				if !seenPairs[pair] {
					pairs = append(pairs, pair)
					seenPairs[pair] = true
				}
				break
			}
		}
	}

	// 3. Cryptocurrency name mentions
	cryptoNames := map[string]string{
		"BITCOIN":      "BTCUSDT",
		"BTC":          "BTCUSDT",
		"ETHEREUM":     "ETHUSDT",
		"ETH":          "ETHUSDT",
		"BINANCE COIN": "BNBUSDT",
		"BNB":          "BNBUSDT",
		"RIPPLE":       "XRPUSDT",
		"XRP":          "XRPUSDT",
		"CARDANO":      "ADAUSDT",
		"ADA":          "ADAUSDT",
		"SOLANA":       "SOLUSDT",
		"SOL":          "SOLUSDT",
		"DOGECOIN":     "DOGEUSDT",
		"DOGE":         "DOGEUSDT",
		"POLKADOT":     "DOTUSDT",
		"DOT":          "DOTUSDT",
		"POLYGON":      "MATICUSDT",
		"MATIC":        "MATICUSDT",
		"AVALANCHE":    "AVAXUSDT",
		"AVAX":         "AVAXUSDT",
		"CHAINLINK":    "LINKUSDT",
		"LINK":         "LINKUSDT",
		"LITECOIN":     "LTCUSDT",
		"LTC":          "LTCUSDT",
		"STELLAR":      "XLMUSDT",
		"XLM":          "XLMUSDT",
		"MONERO":       "XMRUSDT",
		"XMR":          "XMRUSDT",
		"TRON":         "TRXUSDT",
		"TRX":          "TRXUSDT",
	}

	for name, pair := range cryptoNames {
		if strings.Contains(text, name) && !seenPairs[pair] {
			pairs = append(pairs, pair)
			seenPairs[pair] = true
		}
	}

	return pairs
}

// getAllBinancePairs fetches all trading pairs from Binance (cached for 1 hour)
func (s *AIService) getAllBinancePairs() []string {
	// TODO: Implement Redis caching
	// cacheKey := "binance:all_pairs"
	// Try to get from cache, if not found fetch from Binance API

	// For now, use fallback list of common USDT pairs
	return []string{
		"BTCUSDT", "ETHUSDT", "BNBUSDT", "XRPUSDT", "ADAUSDT",
		"SOLUSDT", "DOGEUSDT", "DOTUSDT", "MATICUSDT", "AVAXUSDT",
		"LINKUSDT", "LTCUSDT", "XLMUSDT", "XMRUSDT", "TRXUSDT",
		"ATOMUSDT", "ETCUSDT", "UNIUSDT", "VETUSDT", "ICPUSDT",
		"FILUSDT", "FTMUSDT", "ALGOUSDT", "MANAUSDT", "SANDUSDT",
		"AXSUSDT", "NEARUSDT", "AAVEUSDT", "THETAUSDT", "APTUSDT",
	}
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
