package handler

import (
	"net/http"
	"time"

	"crawl-news/internal/service"
	"crawl-news/pkg/httputil"

	"github.com/gin-gonic/gin"
)

type CrawlerHandler struct {
	crawlerService *service.CrawlerService
}

func NewCrawlerHandler(crawlerService *service.CrawlerService) *CrawlerHandler {
	return &CrawlerHandler{
		crawlerService: crawlerService,
	}
}

type StartCrawlerRequest struct {
	Source       string `json:"source" binding:"required"`
	OnlyNew      bool   `json:"only_new,omitempty"`      // Only crawl news newer than last crawl
	MinAgeHours  int    `json:"min_age_hours,omitempty"` // Only crawl news from last N hours
	ForceRefresh bool   `json:"force_refresh,omitempty"` // Force crawl even if news exists
	Sources      []string `json:"sources,omitempty"`     // Multiple sources to crawl
}

// StartCrawler godoc
// @Summary Start crawler
// @Description Start crawling from a specific source or multiple sources
// @Tags crawler
// @Accept json
// @Produce json
// @Param body body StartCrawlerRequest true "Crawler start request"
// @Success 200 {object} httputil.Response
// @Router /api/v1/crawler/start [post]
func (h *CrawlerHandler) StartCrawler(c *gin.Context) {
	var req StartCrawlerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.ErrorResponse(c, http.StatusBadRequest, "Invalid request", err)
		return
	}

	// Build crawl options
	options := service.CrawlOptions{
		OnlyNew:      req.OnlyNew,
		ForceRefresh: req.ForceRefresh,
	}
	if req.MinAgeHours > 0 {
		options.MinAge = time.Duration(req.MinAgeHours) * time.Hour
	}

	// Handle multiple sources
	if len(req.Sources) > 0 {
		jobIDs, err := h.crawlerService.StartCrawlMultipleSources(c.Request.Context(), req.Sources, options)
		if err != nil {
			httputil.ErrorResponse(c, http.StatusInternalServerError, "Failed to start crawlers", err)
			return
		}
		httputil.SuccessResponse(c, gin.H{
			"message": "Crawlers started successfully",
			"job_ids": jobIDs,
			"count":   len(jobIDs),
		})
		return
	}

	// Single source crawl
	jobID, err := h.crawlerService.StartCrawlWithOptions(c.Request.Context(), req.Source, options)
	if err != nil {
		httputil.ErrorResponse(c, http.StatusInternalServerError, "Failed to start crawler", err)
		return
	}

	httputil.SuccessResponse(c, gin.H{
		"message": "Crawler started successfully",
		"job_id":  jobID,
	})
}

// StopCrawler godoc
// @Summary Stop crawler
// @Description Stop an active crawling job
// @Tags crawler
// @Accept json
// @Produce json
// @Success 200 {object} httputil.Response
// @Router /api/v1/crawler/stop [post]
func (h *CrawlerHandler) StopCrawler(c *gin.Context) {
	err := h.crawlerService.StopCrawl(c.Request.Context())
	if err != nil {
		httputil.ErrorResponse(c, http.StatusInternalServerError, "Failed to stop crawler", err)
		return
	}

	httputil.SuccessResponse(c, gin.H{
		"message": "Crawler stopped successfully",
	})
}

// GetStatus godoc
// @Summary Get crawler status
// @Description Get current status of the crawler
// @Tags crawler
// @Accept json
// @Produce json
// @Success 200 {object} httputil.Response
// @Router /api/v1/crawler/status [get]
func (h *CrawlerHandler) GetStatus(c *gin.Context) {
	status := h.crawlerService.GetStatus(c.Request.Context())
	httputil.SuccessResponse(c, status)
}
