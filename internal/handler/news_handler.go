package handler

import (
	"fmt"
	"net/http"
	"time"

	"crawl-news/internal/model"
	"crawl-news/internal/service"
	"crawl-news/pkg/httputil"

	"github.com/gin-gonic/gin"
)

type NewsHandler struct {
	newsService    *service.NewsService
	crawlerService *service.CrawlerService
}

func NewNewsHandler(newsService *service.NewsService, crawlerService *service.CrawlerService) *NewsHandler {
	return &NewsHandler{
		newsService:    newsService,
		crawlerService: crawlerService,
	}
}

// GetNews godoc
// @Summary Get list of news
// @Description Get paginated list of news articles with pagination metadata
// @Tags news
// @Accept json
// @Produce json
// @Param page query int false "Page number" default(1)
// @Param limit query int false "Items per page" default(20)
// @Param source query string false "Filter by source"
// @Param category query string false "Filter by category"
// @Success 200 {object} httputil.Response
// @Router /api/v1/news [get]
func (h *NewsHandler) GetNews(c *gin.Context) {
	page := c.DefaultQuery("page", "1")
	limit := c.DefaultQuery("limit", "20")
	source := c.Query("source")
	category := c.Query("category")

	news, total, err := h.newsService.GetNews(c.Request.Context(), page, limit, source, category)
	if err != nil {
		httputil.ErrorResponse(c, http.StatusInternalServerError, "Failed to fetch news", err)
		return
	}

	// Convert page and limit to int for calculation
	pageInt, _ := c.GetQuery("page")
	limitInt, _ := c.GetQuery("limit")
	var pageNum, limitNum int
	fmt.Sscanf(pageInt, "%d", &pageNum)
	fmt.Sscanf(limitInt, "%d", &limitNum)
	if pageNum == 0 {
		pageNum = 1
	}
	if limitNum == 0 {
		limitNum = 20
	}

	// Calculate pagination metadata
	totalPages := (total + limitNum - 1) / limitNum
	hasNext := pageNum < totalPages
	hasPrev := pageNum > 1

	// Return with pagination metadata
	httputil.SuccessResponse(c, gin.H{
		"items": news,
		"pagination": gin.H{
			"page":        pageNum,
			"limit":       limitNum,
			"total":       total,
			"total_pages": totalPages,
			"has_next":    hasNext,
			"has_prev":    hasPrev,
		},
	})
}

// GetNewsByID godoc
// @Summary Get news by ID
// @Description Get a single news article by ID
// @Tags news
// @Accept json
// @Produce json
// @Param id path string true "News ID"
// @Success 200 {object} httputil.Response
// @Router /api/v1/news/{id} [get]
func (h *NewsHandler) GetNewsByID(c *gin.Context) {
	id := c.Param("id")

	news, err := h.newsService.GetNewsByID(c.Request.Context(), id)
	if err != nil {
		httputil.ErrorResponse(c, http.StatusNotFound, "News not found", err)
		return
	}

	httputil.SuccessResponse(c, news)
}

// GetNewsWithFilter godoc
// @Summary Get news with filters and pagination
// @Description Get paginated news with filtering options including sentiment, trading pairs, etc
// @Tags news
// @Accept json
// @Produce json
// @Param page query int false "Page number" default(1)
// @Param limit query int false "Items per page" default(20)
// @Param sources query string false "Comma-separated list of sources"
// @Param categories query string false "Comma-separated list of categories"
// @Param trading_pairs query string false "Comma-separated list of trading pairs (e.g., BTCUSDT,ETHUSDT)"
// @Param sentiment query string false "Filter by sentiment (positive/negative/neutral)"
// @Param ai_analyzed query boolean false "Filter by AI analysis status"
// @Success 200 {object} httputil.Response
// @Router /api/v1/news/filter [get]
func (h *NewsHandler) GetNewsWithFilter(c *gin.Context) {
	// Parse pagination params
	var pageNum, limitNum int
	pageStr := c.DefaultQuery("page", "1")
	limitStr := c.DefaultQuery("limit", "20")
	fmt.Sscanf(pageStr, "%d", &pageNum)
	fmt.Sscanf(limitStr, "%d", &limitNum)
	if pageNum < 1 {
		pageNum = 1
	}
	if limitNum < 1 || limitNum > 100 {
		limitNum = 20
	}

	// Parse filters
	filter := &model.NewsFilter{}

	if sourcesStr := c.Query("sources"); sourcesStr != "" {
		filter.Sources = parseCommaSeparated(sourcesStr)
	}
	if categoriesStr := c.Query("categories"); categoriesStr != "" {
		filter.Categories = parseCommaSeparated(categoriesStr)
	}
	if pairsStr := c.Query("trading_pairs"); pairsStr != "" {
		filter.TradingPairs = parseCommaSeparated(pairsStr)
	}
	filter.Sentiment = c.Query("sentiment")

	if aiAnalyzedStr := c.Query("ai_analyzed"); aiAnalyzedStr != "" {
		var analyzed bool
		if _, err := fmt.Sscanf(aiAnalyzedStr, "%t", &analyzed); err == nil {
			filter.AIAnalyzed = &analyzed
		}
	}

	// Parse parsing method filter
	if parsingMethod := c.Query("parsing_method"); parsingMethod != "" {
		filter.ParsingMethod = parsingMethod
	}

	// Get filtered news with pagination
	news, total, err := h.newsService.GetNewsWithFilterPaginated(c.Request.Context(), filter, pageNum, limitNum)
	if err != nil {
		httputil.ErrorResponse(c, http.StatusInternalServerError, "Failed to fetch news", err)
		return
	}

	// Calculate pagination metadata
	totalPages := (total + limitNum - 1) / limitNum
	hasNext := pageNum < totalPages
	hasPrev := pageNum > 1

	httputil.SuccessResponse(c, gin.H{
		"items": news,
		"pagination": gin.H{
			"page":        pageNum,
			"limit":       limitNum,
			"total":       total,
			"total_pages": totalPages,
			"has_next":    hasNext,
			"has_prev":    hasPrev,
		},
	})
}

// GetNewsAdvanced godoc
// @Summary Get news with advanced filters
// @Description Get news with advanced filtering options including date range, sentiment, trading pairs
// @Tags news
// @Accept json
// @Produce json
// @Param start_date query string false "Start date (RFC3339 format)"
// @Param end_date query string false "End date (RFC3339 format)"
// @Param sources query string false "Comma-separated list of sources"
// @Param categories query string false "Comma-separated list of categories"
// @Param trading_pairs query string false "Comma-separated list of trading pairs (e.g., BTCUSDT,ETHUSDT)"
// @Param sentiment query string false "Filter by sentiment (positive/negative/neutral)"
// @Param min_score query number false "Minimum sentiment score (-1.0 to 1.0)"
// @Param ai_analyzed query boolean false "Filter by AI analysis status"
// @Param language query string false "Filter by language"
// @Success 200 {object} httputil.Response
// @Router /api/v1/news/advanced [get]
func (h *NewsHandler) GetNewsAdvanced(c *gin.Context) {
	filter := &model.NewsFilter{}

	// Parse date range
	if startDateStr := c.Query("start_date"); startDateStr != "" {
		if startDate, err := time.Parse(time.RFC3339, startDateStr); err == nil {
			filter.StartDate = &startDate
		}
	}
	if endDateStr := c.Query("end_date"); endDateStr != "" {
		if endDate, err := time.Parse(time.RFC3339, endDateStr); err == nil {
			filter.EndDate = &endDate
		}
	}

	// Parse sources
	if sourcesStr := c.Query("sources"); sourcesStr != "" {
		filter.Sources = parseCommaSeparated(sourcesStr)
	}

	// Parse categories
	if categoriesStr := c.Query("categories"); categoriesStr != "" {
		filter.Categories = parseCommaSeparated(categoriesStr)
	}

	// Parse trading pairs
	if pairsStr := c.Query("trading_pairs"); pairsStr != "" {
		filter.TradingPairs = parseCommaSeparated(pairsStr)
	}

	// Parse sentiment
	filter.Sentiment = c.Query("sentiment")

	// Parse min score
	if minScoreStr := c.Query("min_score"); minScoreStr != "" {
		var score float64
		if _, err := fmt.Sscanf(minScoreStr, "%f", &score); err == nil {
			filter.MinScore = &score
		}
	}

	// Parse AI analyzed
	if aiAnalyzedStr := c.Query("ai_analyzed"); aiAnalyzedStr != "" {
		var analyzed bool
		if _, err := fmt.Sscanf(aiAnalyzedStr, "%t", &analyzed); err == nil {
			filter.AIAnalyzed = &analyzed
		}
	}

	// Parse parsing method filter
	if parsingMethod := c.Query("parsing_method"); parsingMethod != "" {
		filter.ParsingMethod = parsingMethod
	}

	// Parse language
	filter.Language = c.Query("language")

	news, err := h.newsService.GetNewsWithFilter(c.Request.Context(), filter)
	if err != nil {
		httputil.ErrorResponse(c, http.StatusInternalServerError, "Failed to fetch news", err)
		return
	}

	// Return news array directly
	httputil.SuccessResponse(c, news)
}

// GetNewsByTradingPair godoc
// @Summary Get news by trading pair
// @Description Get news related to a specific trading pair
// @Tags news
// @Accept json
// @Produce json
// @Param pair path string true "Trading pair (e.g., BTCUSDT)"
// @Success 200 {object} httputil.Response
// @Router /api/v1/news/pair/{pair} [get]
func (h *NewsHandler) GetNewsByTradingPair(c *gin.Context) {
	pair := c.Param("pair")

	news, err := h.newsService.GetNewsByTradingPair(c.Request.Context(), pair)
	if err != nil {
		httputil.ErrorResponse(c, http.StatusInternalServerError, "Failed to fetch news", err)
		return
	}

	// Return news array directly
	httputil.SuccessResponse(c, news)
}

// GetLatestNewsBySymbol godoc
// @Summary Get latest 10 news by trading symbol
// @Description Get the 10 most recent news articles for a specific trading pair/symbol (e.g., BTCUSDT)
// @Tags news
// @Accept json
// @Produce json
// @Param symbol path string true "Trading symbol (e.g., BTCUSDT, ETHUSDT)"
// @Param limit query int false "Number of articles to return (default: 10, max: 50)"
// @Success 200 {object} httputil.Response
// @Router /api/v1/news/latest/{symbol} [get]
func (h *NewsHandler) GetLatestNewsBySymbol(c *gin.Context) {
	symbol := c.Param("symbol")
	
	// Parse limit with default 10
	limit := 10
	if limitStr := c.Query("limit"); limitStr != "" {
		fmt.Sscanf(limitStr, "%d", &limit)
		// Cap at 50
		if limit > 50 {
			limit = 50
		}
		if limit < 1 {
			limit = 10
		}
	}

	news, err := h.newsService.GetLatestNewsBySymbol(c.Request.Context(), symbol, limit)
	if err != nil {
		httputil.ErrorResponse(c, http.StatusInternalServerError, "Failed to fetch latest news", err)
		return
	}

	httputil.SuccessResponse(c, gin.H{
		"symbol": symbol,
		"count":  len(news),
		"limit":  limit,
		"items":  news,
	})
}

// GetNewsSummaries godoc
// @Summary Get news summaries
// @Description Get simplified news data for listing views
// @Tags news
// @Accept json
// @Produce json
// @Param limit query int false "Limit results" default(50)
// @Param trading_pair query string false "Filter by trading pair"
// @Param sentiment query string false "Filter by sentiment"
// @Success 200 {object} httputil.Response
// @Router /api/v1/news/summaries [get]
func (h *NewsHandler) GetNewsSummaries(c *gin.Context) {
	limit := 50
	if limitStr := c.Query("limit"); limitStr != "" {
		fmt.Sscanf(limitStr, "%d", &limit)
	}

	filter := &model.NewsFilter{}

	if pair := c.Query("trading_pair"); pair != "" {
		filter.TradingPairs = []string{pair}
	}

	if sentiment := c.Query("sentiment"); sentiment != "" {
		filter.Sentiment = sentiment
	}

	summaries, err := h.newsService.GetNewsSummaries(c.Request.Context(), filter, limit)
	if err != nil {
		httputil.ErrorResponse(c, http.StatusInternalServerError, "Failed to fetch news summaries", err)
		return
	}

	// Return summaries directly as data
	httputil.SuccessResponse(c, summaries)
}

// Helper function to parse comma-separated strings
func parseCommaSeparated(s string) []string {
	var result []string
	for _, item := range splitString(s, ",") {
		trimmed := trimSpace(item)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func splitString(s, sep string) []string {
	if s == "" {
		return nil
	}
	var result []string
	start := 0
	for i := 0; i < len(s); i++ {
		if i == len(s)-1 || (i < len(s)-1 && s[i:i+len(sep)] == sep) {
			if i == len(s)-1 {
				result = append(result, s[start:])
			} else {
				result = append(result, s[start:i])
				i += len(sep) - 1
				start = i + 1
			}
		}
	}
	return result
}

func trimSpace(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n') {
		end--
	}
	return s[start:end]
}

// FetchNewsDetail godoc
// @Summary Fetch full details for a news article
// @Description Crawls the source URL to extract full article content and updates the database
// @Tags news
// @Accept json
// @Produce json
// @Param id path string true "News ID"
// @Success 200 {object} httputil.Response
// @Router /api/v1/news/{id}/fetch-detail [post]
func (h *NewsHandler) FetchNewsDetail(c *gin.Context) {
	id := c.Param("id")

	// Get the existing news to get the source URL
	existingNews, err := h.newsService.GetNewsByID(c.Request.Context(), id)
	if err != nil {
		httputil.ErrorResponse(c, http.StatusNotFound, "News not found", err)
		return
	}

	if existingNews.SourceURL == "" {
		httputil.ErrorResponse(c, http.StatusBadRequest, "News has no source URL", nil)
		return
	}

	// Fetch details from URL
	detailedNews, err := h.crawlerService.FetchDetailFromURL(c.Request.Context(), existingNews.SourceURL)
	if err != nil {
		httputil.ErrorResponse(c, http.StatusInternalServerError, "Failed to fetch article details", err)
		return
	}

	// Update the existing news with detailed content
	existingNews.Content = detailedNews.Content
	existingNews.Author = detailedNews.Author
	if len(detailedNews.Tags) > 0 {
		existingNews.Tags = detailedNews.Tags
	}
	if detailedNews.ImageURL != "" && existingNews.ImageURL == "" {
		existingNews.ImageURL = detailedNews.ImageURL
	}

	// Save updated news
	if err := h.newsService.UpdateNews(c.Request.Context(), existingNews); err != nil {
		httputil.ErrorResponse(c, http.StatusInternalServerError, "Failed to update news", err)
		return
	}

	httputil.SuccessResponse(c, existingNews)
}
