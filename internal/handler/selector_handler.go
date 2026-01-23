package handler

import (
	"net/http"
	"strconv"

	"crawl-news/internal/service"
	"crawl-news/pkg/httputil"

	"github.com/gin-gonic/gin"
)

type SelectorHandler struct {
	selectorLearner *service.SelectorLearner
}

func NewSelectorHandler(learner *service.SelectorLearner) *SelectorHandler {
	return &SelectorHandler{
		selectorLearner: learner,
	}
}

type DiscoverSelectorsRequest struct {
	SourceURL  string `json:"source_url" binding:"required"`
	SourceName string `json:"source_name" binding:"required"`
}

// DiscoverSelectors godoc
// @Summary Discover selectors for a source
// @Description Automatically discover CSS selectors for a news source
// @Tags selectors
// @Accept json
// @Produce json
// @Param body body DiscoverSelectorsRequest true "Discovery request"
// @Success 200 {object} httputil.Response
// @Router /api/v1/selectors/discover [post]
func (h *SelectorHandler) DiscoverSelectors(c *gin.Context) {
	var req DiscoverSelectorsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.ErrorResponse(c, http.StatusBadRequest, "Invalid request", err)
		return
	}

	discovered, err := h.selectorLearner.DiscoverSelectors(c.Request.Context(), req.SourceURL, req.SourceName)
	if err != nil {
		httputil.ErrorResponse(c, http.StatusInternalServerError, "Selector discovery failed", err)
		return
	}

	httputil.SuccessResponse(c, gin.H{
		"message":    "Selectors discovered successfully",
		"count":      len(discovered),
		"selectors":  discovered,
		"source":     req.SourceName,
	})
}

// GetBestSelector godoc
// @Summary Get best selector for element type
// @Description Get the most reliable selector for a source and element type
// @Tags selectors
// @Accept json
// @Produce json
// @Param source query string true "Source name"
// @Param element_type query string true "Element type (title, content, image, etc.)"
// @Success 200 {object} httputil.Response
// @Router /api/v1/selectors/best [get]
func (h *SelectorHandler) GetBestSelector(c *gin.Context) {
	source := c.Query("source")
	elementType := c.Query("element_type")

	if source == "" || elementType == "" {
		httputil.ErrorResponse(c, http.StatusBadRequest, "source and element_type are required", nil)
		return
	}

	selector, err := h.selectorLearner.GetBestSelector(source, elementType)
	if err != nil {
		httputil.ErrorResponse(c, http.StatusNotFound, "No selector found", err)
		return
	}

	httputil.SuccessResponse(c, selector)
}

// GetFallbackSelectors godoc
// @Summary Get fallback selectors
// @Description Get list of fallback selectors for a source and element type
// @Tags selectors
// @Accept json
// @Produce json
// @Param source query string true "Source name"
// @Param element_type query string true "Element type"
// @Success 200 {object} httputil.Response
// @Router /api/v1/selectors/fallbacks [get]
func (h *SelectorHandler) GetFallbackSelectors(c *gin.Context) {
	source := c.Query("source")
	elementType := c.Query("element_type")

	if source == "" || elementType == "" {
		httputil.ErrorResponse(c, http.StatusBadRequest, "source and element_type are required", nil)
		return
	}

	selectors, err := h.selectorLearner.GetFallbackSelectors(source, elementType)
	if err != nil {
		httputil.ErrorResponse(c, http.StatusInternalServerError, "Failed to get fallback selectors", err)
		return
	}

	httputil.SuccessResponse(c, gin.H{
		"source":       source,
		"element_type": elementType,
		"fallbacks":    selectors,
		"count":        len(selectors),
	})
}

type RecordSelectorResultRequest struct {
	Source      string `json:"source" binding:"required"`
	ElementType string `json:"element_type" binding:"required"`
	Selector    string `json:"selector" binding:"required"`
	Success     bool   `json:"success"`
}

// RecordSelectorResult godoc
// @Summary Record selector extraction result
// @Description Record whether a selector successfully extracted data
// @Tags selectors
// @Accept json
// @Produce json
// @Param body body RecordSelectorResultRequest true "Selector result"
// @Success 200 {object} httputil.Response
// @Router /api/v1/selectors/record [post]
func (h *SelectorHandler) RecordSelectorResult(c *gin.Context) {
	var req RecordSelectorResultRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.ErrorResponse(c, http.StatusBadRequest, "Invalid request", err)
		return
	}

	var err error
	if req.Success {
		err = h.selectorLearner.RecordSelectorSuccess(req.Source, req.ElementType, req.Selector)
	} else {
		err = h.selectorLearner.RecordSelectorFailure(req.Source, req.ElementType, req.Selector)
	}

	if err != nil {
		httputil.ErrorResponse(c, http.StatusInternalServerError, "Failed to record result", err)
		return
	}

	httputil.SuccessResponse(c, gin.H{
		"message": "Selector result recorded successfully",
	})
}

type PromoteSelectorRequest struct {
	Source      string `json:"source" binding:"required"`
	ElementType string `json:"element_type" binding:"required"`
	Selector    string `json:"selector" binding:"required"`
}

// PromoteSelector godoc
// @Summary Promote selector to primary
// @Description Set a selector as the primary selector for its type
// @Tags selectors
// @Accept json
// @Produce json
// @Param body body PromoteSelectorRequest true "Selector to promote"
// @Success 200 {object} httputil.Response
// @Router /api/v1/selectors/promote [post]
func (h *SelectorHandler) PromoteSelector(c *gin.Context) {
	var req PromoteSelectorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.ErrorResponse(c, http.StatusBadRequest, "Invalid request", err)
		return
	}

	if err := h.selectorLearner.PromoteSelector(req.Source, req.ElementType, req.Selector); err != nil {
		httputil.ErrorResponse(c, http.StatusInternalServerError, "Failed to promote selector", err)
		return
	}

	httputil.SuccessResponse(c, gin.H{
		"message": "Selector promoted successfully",
	})
}

type AutoHealRequest struct {
	Source         string `json:"source" binding:"required"`
	ElementType    string `json:"element_type" binding:"required"`
	FailedSelector string `json:"failed_selector" binding:"required"`
}

// AutoHealSelector godoc
// @Summary Auto-heal failed selector
// @Description Automatically find and promote a working replacement selector
// @Tags selectors
// @Accept json
// @Produce json
// @Param body body AutoHealRequest true "Auto-heal request"
// @Success 200 {object} httputil.Response
// @Router /api/v1/selectors/auto-heal [post]
func (h *SelectorHandler) AutoHealSelector(c *gin.Context) {
	var req AutoHealRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.ErrorResponse(c, http.StatusBadRequest, "Invalid request", err)
		return
	}

	replacement, err := h.selectorLearner.AutoHealSelector(
		c.Request.Context(),
		req.Source,
		req.ElementType,
		req.FailedSelector,
	)

	if err != nil {
		httputil.ErrorResponse(c, http.StatusInternalServerError, "Auto-heal failed", err)
		return
	}

	httputil.SuccessResponse(c, gin.H{
		"message":             "Auto-heal successful",
		"failed_selector":     req.FailedSelector,
		"replacement_selector": replacement,
	})
}

// GetSelectorStatistics godoc
// @Summary Get selector statistics
// @Description Get statistics about learned selectors for a source
// @Tags selectors
// @Accept json
// @Produce json
// @Param source path string true "Source name"
// @Success 200 {object} httputil.Response
// @Router /api/v1/selectors/stats/{source} [get]
func (h *SelectorHandler) GetSelectorStatistics(c *gin.Context) {
	source := c.Param("source")

	// Get statistics by element type
	elementTypes := []string{"article_list", "title", "content", "summary", "author", "date", "image"}
	
	stats := make(map[string]interface{})
	totalActive := 0
	totalLearned := 0

	for _, elementType := range elementTypes {
		selectors, err := h.selectorLearner.GetFallbackSelectors(source, elementType)
		if err != nil {
			continue
		}

		activeCount := 0
		for _, sel := range selectors {
			if sel.IsActive {
				activeCount++
			}
		}

		stats[elementType] = map[string]interface{}{
			"total":  len(selectors),
			"active": activeCount,
		}

		totalLearned += len(selectors)
		totalActive += activeCount
	}

	httputil.SuccessResponse(c, gin.H{
		"source":         source,
		"total_learned":  totalLearned,
		"total_active":   totalActive,
		"by_element":     stats,
	})
}

// ListLearnedSelectors godoc
// @Summary List learned selectors
// @Description Get all learned selectors with pagination
// @Tags selectors
// @Accept json
// @Produce json
// @Param source query string false "Filter by source"
// @Param element_type query string false "Filter by element type"
// @Param active query bool false "Filter by active status"
// @Param page query int false "Page number" default(1)
// @Param limit query int false "Items per page" default(20)
// @Success 200 {object} httputil.Response
// @Router /api/v1/selectors/list [get]
func (h *SelectorHandler) ListLearnedSelectors(c *gin.Context) {
	source := c.Query("source")
	elementType := c.Query("element_type")
	activeStr := c.Query("active")
	
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}

	// Build filters
	filters := make(map[string]interface{})
	if source != "" {
		filters["source"] = source
	}
	if elementType != "" {
		filters["element_type"] = elementType
	}
	if activeStr == "true" {
		filters["is_active"] = true
	} else if activeStr == "false" {
		filters["is_active"] = false
	}

	// For now, return a simple response
	// In production, implement proper pagination query
	httputil.SuccessResponse(c, gin.H{
		"message": "Selector listing not yet fully implemented",
		"filters": filters,
		"page":    page,
		"limit":   limit,
	})
}
