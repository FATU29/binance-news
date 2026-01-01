package handler

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"crawl-news/internal/service"
)

type AnalyticsHandler struct {
	analyticsService *service.AnalyticsService
}

func NewAnalyticsHandler(analyticsService *service.AnalyticsService) *AnalyticsHandler {
	return &AnalyticsHandler{
		analyticsService: analyticsService,
	}
}

// GetCrawlStats returns comprehensive crawling statistics
// @Summary Get crawl statistics
// @Description Get comprehensive statistics about crawled news
// @Tags analytics
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /analytics/crawl [get]
func (h *AnalyticsHandler) GetCrawlStats(c *gin.Context) {
	stats, err := h.analyticsService.GetCrawlStats(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "Failed to get crawl statistics",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   stats,
	})
}

// GetPageStats returns page-specific statistics
// @Summary Get page statistics
// @Description Get statistics about news pages and activity
// @Tags analytics
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /analytics/page [get]
func (h *AnalyticsHandler) GetPageStats(c *gin.Context) {
	stats, err := h.analyticsService.GetPageStats(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "Failed to get page statistics",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   stats,
	})
}

// GetSourceAnalytics returns analytics for a specific source
// @Summary Get source analytics
// @Description Get detailed analytics for a specific news source
// @Tags analytics
// @Produce json
// @Param source path string true "News source"
// @Success 200 {object} map[string]interface{}
// @Router /analytics/source/{source} [get]
func (h *AnalyticsHandler) GetSourceAnalytics(c *gin.Context) {
	source := c.Param("source")
	if source == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "Source parameter is required",
		})
		return
	}

	analytics, err := h.analyticsService.GetSourceAnalytics(c.Request.Context(), source)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "Failed to get source analytics",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   analytics,
	})
}

// GetSentimentTrends returns sentiment trends over time
// @Summary Get sentiment trends
// @Description Get sentiment analysis trends over time (hourly, daily, weekly, monthly)
// @Tags analytics
// @Produce json
// @Param timeframe query string false "Timeframe: hour, day, week, month" default(day)
// @Param start_date query string false "Start date (RFC3339)"
// @Param end_date query string false "End date (RFC3339)"
// @Param sources query string false "Comma-separated sources"
// @Param trading_pairs query string false "Comma-separated trading pairs"
// @Success 200 {object} map[string]interface{}
// @Router /analytics/sentiment/trends [get]
func (h *AnalyticsHandler) GetSentimentTrends(c *gin.Context) {
	req := &service.SentimentTrendRequest{
		Timeframe: c.DefaultQuery("timeframe", "day"),
	}

	// Parse start date
	if startDateStr := c.Query("start_date"); startDateStr != "" {
		if startDate, err := time.Parse(time.RFC3339, startDateStr); err == nil {
			req.StartDate = &startDate
		}
	}

	// Parse end date
	if endDateStr := c.Query("end_date"); endDateStr != "" {
		if endDate, err := time.Parse(time.RFC3339, endDateStr); err == nil {
			req.EndDate = &endDate
		}
	}

	// Parse sources
	if sourcesStr := c.Query("sources"); sourcesStr != "" {
		sources := []string{}
		for _, s := range strings.Split(sourcesStr, ",") {
			if trimmed := strings.TrimSpace(s); trimmed != "" {
				sources = append(sources, trimmed)
			}
		}
		req.Sources = sources
	}

	// Parse trading pairs
	if pairsStr := c.Query("trading_pairs"); pairsStr != "" {
		pairs := []string{}
		for _, p := range strings.Split(pairsStr, ",") {
			if trimmed := strings.TrimSpace(p); trimmed != "" {
				pairs = append(pairs, trimmed)
			}
		}
		req.TradingPairs = pairs
	}

	trends, err := h.analyticsService.GetSentimentTrends(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "Failed to get sentiment trends",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   trends,
	})
}

// GetSentimentByPair returns sentiment breakdown for a trading pair
// @Summary Get sentiment by trading pair
// @Description Get sentiment analysis breakdown for a specific trading pair
// @Tags analytics
// @Produce json
// @Param pair path string true "Trading pair (e.g., BTCUSDT)"
// @Success 200 {object} map[string]interface{}
// @Router /analytics/sentiment/pair/{pair} [get]
func (h *AnalyticsHandler) GetSentimentByPair(c *gin.Context) {
	pair := c.Param("pair")
	if pair == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "Trading pair parameter is required",
		})
		return
	}

	analytics, err := h.analyticsService.GetSentimentByTradingPair(c.Request.Context(), pair)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "Failed to get sentiment by pair",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   analytics,
	})
}
