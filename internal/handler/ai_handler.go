package handler

import (
	"fmt"
	"net/http"

	"crawl-news/internal/model"
	"crawl-news/internal/service"
	"crawl-news/pkg/httputil"

	"github.com/gin-gonic/gin"
)

type AIHandler struct {
	aiService      *service.AIService
	newsService    *service.NewsService
	sentimentQueue *service.SentimentQueue
}

func NewAIHandler(aiService *service.AIService, newsService *service.NewsService, sentimentQueue *service.SentimentQueue) *AIHandler {
	return &AIHandler{
		aiService:      aiService,
		newsService:    newsService,
		sentimentQueue: sentimentQueue,
	}
}

// AnalyzeSentiment godoc
// @Summary Analyze sentiment of a news article
// @Description Perform AI sentiment analysis on a specific news article
// @Tags ai
// @Accept json
// @Produce json
// @Param id path string true "News ID"
// @Success 200 {object} httputil.Response
// @Router /api/v1/ai/sentiment/{id} [post]
func (h *AIHandler) AnalyzeSentiment(c *gin.Context) {
	id := c.Param("id")

	sentiment, err := h.aiService.AnalyzeSentiment(c.Request.Context(), id)
	if err != nil {
		httputil.ErrorResponse(c, http.StatusInternalServerError, "Failed to analyze sentiment", err)
		return
	}

	httputil.SuccessResponse(c, gin.H{
		"news_id":   id,
		"sentiment": sentiment,
	})
}

// AnalyzePriceImpact godoc
// @Summary Analyze price impact of news
// @Description Predict potential price impact on trading pairs from news
// @Tags ai
// @Accept json
// @Produce json
// @Param body body model.AIAnalysisRequest true "Analysis request"
// @Success 200 {object} httputil.Response
// @Router /api/v1/ai/price-impact [post]
func (h *AIHandler) AnalyzePriceImpact(c *gin.Context) {
	var req model.AIAnalysisRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.ErrorResponse(c, http.StatusBadRequest, "Invalid request", err)
		return
	}

	priceImpact, err := h.aiService.AnalyzePriceImpact(c.Request.Context(), req.NewsID, req.TradingPairs)
	if err != nil {
		httputil.ErrorResponse(c, http.StatusInternalServerError, "Failed to analyze price impact", err)
		return
	}

	httputil.SuccessResponse(c, gin.H{
		"news_id":       req.NewsID,
		"trading_pairs": req.TradingPairs,
		"price_impact":  priceImpact,
	})
}

// FullAnalysis godoc
// @Summary Perform full AI analysis
// @Description Perform complete AI analysis including sentiment and price impact
// @Tags ai
// @Accept json
// @Produce json
// @Param body body model.AIAnalysisRequest true "Analysis request"
// @Success 200 {object} httputil.Response
// @Router /api/v1/ai/analyze [post]
func (h *AIHandler) FullAnalysis(c *gin.Context) {
	var req model.AIAnalysisRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.ErrorResponse(c, http.StatusBadRequest, "Invalid request", err)
		return
	}

	news, err := h.aiService.FullAnalysis(c.Request.Context(), req.NewsID, req.TradingPairs)
	if err != nil {
		httputil.ErrorResponse(c, http.StatusInternalServerError, "Failed to perform analysis", err)
		return
	}

	httputil.SuccessResponse(c, news)
}

// BatchAnalyze godoc
// @Summary Batch analyze multiple news
// @Description Perform AI analysis on multiple news articles at once
// @Tags ai
// @Accept json
// @Produce json
// @Param body body []string true "Array of news IDs"
// @Success 200 {object} httputil.Response
// @Router /api/v1/ai/batch-analyze [post]
func (h *AIHandler) BatchAnalyze(c *gin.Context) {
	var newsIDs []string
	if err := c.ShouldBindJSON(&newsIDs); err != nil {
		httputil.ErrorResponse(c, http.StatusBadRequest, "Invalid request", err)
		return
	}

	results, err := h.aiService.BatchAnalyze(c.Request.Context(), newsIDs)
	if err != nil {
		httputil.ErrorResponse(c, http.StatusInternalServerError, "Failed to batch analyze", err)
		return
	}

	httputil.SuccessResponse(c, gin.H{
		"total":   len(results),
		"results": results,
	})
}

// GetAnalyzedNews godoc
// @Summary Get analyzed news
// @Description Retrieve news that have been analyzed by AI with filters
// @Tags ai
// @Accept json
// @Produce json
// @Param trading_pair query string false "Filter by trading pair"
// @Param sentiment query string false "Filter by sentiment (positive/negative/neutral)"
// @Param min_score query number false "Minimum sentiment score"
// @Success 200 {object} httputil.Response
// @Router /api/v1/ai/analyzed-news [get]
func (h *AIHandler) GetAnalyzedNews(c *gin.Context) {
	tradingPair := c.Query("trading_pair")
	sentiment := c.Query("sentiment")
	minScore := c.Query("min_score")

	filter := &model.NewsFilter{
		Sentiment: sentiment,
	}

	if tradingPair != "" {
		filter.TradingPairs = []string{tradingPair}
	}

	if minScore != "" {
		var score float64
		if _, err := fmt.Sscanf(minScore, "%f", &score); err == nil {
			filter.MinScore = &score
		}
	}

	aiAnalyzed := true
	filter.AIAnalyzed = &aiAnalyzed

	news, err := h.aiService.GetAnalyzedNews(c.Request.Context(), filter)
	if err != nil {
		httputil.ErrorResponse(c, http.StatusInternalServerError, "Failed to fetch analyzed news", err)
		return
	}

	httputil.SuccessResponse(c, gin.H{
		"total": len(news),
		"news":  news,
	})
}

// GetUnanalyzedNews godoc
// @Summary Get unanalyzed news
// @Description Retrieve news that haven't been analyzed by AI yet
// @Tags ai
// @Accept json
// @Produce json
// @Param limit query int false "Limit results" default(50)
// @Success 200 {object} httputil.Response
// @Router /api/v1/ai/unanalyzed-news [get]
func (h *AIHandler) GetUnanalyzedNews(c *gin.Context) {
	limit := 50
	if limitStr := c.Query("limit"); limitStr != "" {
		fmt.Sscanf(limitStr, "%d", &limit)
	}

	news, err := h.newsService.GetUnanalyzedNews(c.Request.Context(), limit)
	if err != nil {
		httputil.ErrorResponse(c, http.StatusInternalServerError, "Failed to fetch unanalyzed news", err)
		return
	}

	httputil.SuccessResponse(c, gin.H{
		"total": len(news),
		"news":  news,
	})
}

// ReanalyzeAll godoc
// @Summary Re-analyze all articles with keyword fallback
// @Description Re-run AI sentiment analysis on articles that were analyzed with keyword fallback
// @Tags ai
// @Produce json
// @Success 200 {object} httputil.Response
// @Router /api/v1/ai/reanalyze-all [post]
func (h *AIHandler) ReanalyzeAll(c *gin.Context) {
	// Run in background to avoid HTTP timeout
	go func() {
		total, success, err := h.aiService.ReanalyzeAll(c.Request.Context())
		if err != nil {
			fmt.Printf("Re-analysis failed: %v\n", err)
			return
		}
		fmt.Printf("Re-analysis complete: %d total, %d success, %d failed\n", total, success, total-success)
	}()

	httputil.SuccessResponse(c, gin.H{
		"message": "Re-analysis started in background. Check logs for progress.",
	})
}

// QueueStats godoc
// @Summary Get sentiment queue statistics
// @Description Returns real-time stats of the sentiment analysis queue (pending, processing, completed, failed)
// @Tags ai
// @Produce json
// @Success 200 {object} httputil.Response
// @Router /api/v1/ai/queue-stats [get]
func (h *AIHandler) QueueStats(c *gin.Context) {
	if h.sentimentQueue == nil {
		httputil.ErrorResponse(c, http.StatusServiceUnavailable, "Sentiment queue not available", nil)
		return
	}

	stats := h.sentimentQueue.Stats()
	httputil.SuccessResponse(c, stats)
}