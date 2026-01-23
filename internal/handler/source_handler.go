package handler

import (
	"net/http"

	"crawl-news/internal/model"
	"crawl-news/internal/service"
	"crawl-news/pkg/httputil"

	"github.com/gin-gonic/gin"
)

type SourceHandler struct {
	sourceService *service.SourceService
}

func NewSourceHandler(sourceService *service.SourceService) *SourceHandler {
	return &SourceHandler{
		sourceService: sourceService,
	}
}

// GetAllSources godoc
// @Summary Get all crawl sources
// @Description Get list of all configured crawl sources
// @Tags sources
// @Accept json
// @Produce json
// @Param enabled query bool false "Filter by enabled status"
// @Success 200 {object} httputil.Response
// @Router /api/v1/sources [get]
func (h *SourceHandler) GetAllSources(c *gin.Context) {
	enabledFilter := c.Query("enabled")
	
	var sources []model.CrawlSource
	var err error
	
	if enabledFilter == "true" {
		trueVal := true
		sources, err = h.sourceService.GetAllSources(&trueVal)
	} else if enabledFilter == "false" {
		falseVal := false
		sources, err = h.sourceService.GetAllSources(&falseVal)
	} else {
		sources, err = h.sourceService.GetAllSources(nil)
	}
	
	if err != nil {
		httputil.ErrorResponse(c, http.StatusInternalServerError, "Failed to get sources", err)
		return
	}

	httputil.SuccessResponse(c, gin.H{
		"sources": sources,
		"count":   len(sources),
	})
}

// GetSourceByName godoc
// @Summary Get source by name
// @Description Get detailed information about a specific source
// @Tags sources
// @Accept json
// @Produce json
// @Param name path string true "Source name"
// @Success 200 {object} httputil.Response
// @Router /api/v1/sources/{name} [get]
func (h *SourceHandler) GetSourceByName(c *gin.Context) {
	name := c.Param("name")
	
	source, err := h.sourceService.GetSourceByName(name)
	if err != nil {
		httputil.ErrorResponse(c, http.StatusNotFound, "Source not found", err)
		return
	}

	httputil.SuccessResponse(c, source)
}

// CreateSource godoc
// @Summary Create new crawl source
// @Description Add a new crawl source configuration
// @Tags sources
// @Accept json
// @Produce json
// @Param body body model.CrawlSource true "Source configuration"
// @Success 201 {object} httputil.Response
// @Router /api/v1/sources [post]
func (h *SourceHandler) CreateSource(c *gin.Context) {
	var source model.CrawlSource
	if err := c.ShouldBindJSON(&source); err != nil {
		httputil.ErrorResponse(c, http.StatusBadRequest, "Invalid request", err)
		return
	}

	if err := h.sourceService.CreateSource(&source); err != nil {
		httputil.ErrorResponse(c, http.StatusInternalServerError, "Failed to create source", err)
		return
	}

	httputil.SuccessResponse(c, gin.H{
		"message": "Source created successfully",
		"source":  source,
	})
}

// UpdateSource godoc
// @Summary Update crawl source
// @Description Update an existing crawl source configuration
// @Tags sources
// @Accept json
// @Produce json
// @Param name path string true "Source name"
// @Param body body model.CrawlSource true "Updated source configuration"
// @Success 200 {object} httputil.Response
// @Router /api/v1/sources/{name} [put]
func (h *SourceHandler) UpdateSource(c *gin.Context) {
	name := c.Param("name")
	
	var updates model.CrawlSource
	if err := c.ShouldBindJSON(&updates); err != nil {
		httputil.ErrorResponse(c, http.StatusBadRequest, "Invalid request", err)
		return
	}

	if err := h.sourceService.UpdateSource(name, &updates); err != nil {
		httputil.ErrorResponse(c, http.StatusInternalServerError, "Failed to update source", err)
		return
	}

	httputil.SuccessResponse(c, gin.H{
		"message": "Source updated successfully",
	})
}

// DeleteSource godoc
// @Summary Delete crawl source
// @Description Remove a crawl source from configuration
// @Tags sources
// @Accept json
// @Produce json
// @Param name path string true "Source name"
// @Success 200 {object} httputil.Response
// @Router /api/v1/sources/{name} [delete]
func (h *SourceHandler) DeleteSource(c *gin.Context) {
	name := c.Param("name")
	
	if err := h.sourceService.DeleteSource(name); err != nil {
		httputil.ErrorResponse(c, http.StatusInternalServerError, "Failed to delete source", err)
		return
	}

	httputil.SuccessResponse(c, gin.H{
		"message": "Source deleted successfully",
	})
}

// UpdateSourceSelectors godoc
// @Summary Update source selectors
// @Description Update CSS selectors for a specific source
// @Tags sources
// @Accept json
// @Produce json
// @Param name path string true "Source name"
// @Param body body model.Selector true "Updated selectors"
// @Success 200 {object} httputil.Response
// @Router /api/v1/sources/{name}/selectors [put]
func (h *SourceHandler) UpdateSourceSelectors(c *gin.Context) {
	name := c.Param("name")
	
	var selectors model.Selector
	if err := c.ShouldBindJSON(&selectors); err != nil {
		httputil.ErrorResponse(c, http.StatusBadRequest, "Invalid request", err)
		return
	}

	if err := h.sourceService.UpdateSelectors(name, &selectors); err != nil {
		httputil.ErrorResponse(c, http.StatusInternalServerError, "Failed to update selectors", err)
		return
	}

	httputil.SuccessResponse(c, gin.H{
		"message": "Selectors updated successfully",
	})
}

// EnableSource godoc
// @Summary Enable crawl source
// @Description Enable a disabled crawl source
// @Tags sources
// @Accept json
// @Produce json
// @Param name path string true "Source name"
// @Success 200 {object} httputil.Response
// @Router /api/v1/sources/{name}/enable [post]
func (h *SourceHandler) EnableSource(c *gin.Context) {
	name := c.Param("name")
	
	if err := h.sourceService.SetSourceEnabled(name, true); err != nil {
		httputil.ErrorResponse(c, http.StatusInternalServerError, "Failed to enable source", err)
		return
	}

	httputil.SuccessResponse(c, gin.H{
		"message": "Source enabled successfully",
	})
}

// DisableSource godoc
// @Summary Disable crawl source
// @Description Disable a crawl source temporarily
// @Tags sources
// @Accept json
// @Produce json
// @Param name path string true "Source name"
// @Success 200 {object} httputil.Response
// @Router /api/v1/sources/{name}/disable [post]
func (h *SourceHandler) DisableSource(c *gin.Context) {
	name := c.Param("name")
	
	if err := h.sourceService.SetSourceEnabled(name, false); err != nil {
		httputil.ErrorResponse(c, http.StatusInternalServerError, "Failed to disable source", err)
		return
	}

	httputil.SuccessResponse(c, gin.H{
		"message": "Source disabled successfully",
	})
}

// TestSource godoc
// @Summary Test crawl source
// @Description Test if a source is working correctly
// @Tags sources
// @Accept json
// @Produce json
// @Param name path string true "Source name"
// @Success 200 {object} httputil.Response
// @Router /api/v1/sources/{name}/test [post]
func (h *SourceHandler) TestSource(c *gin.Context) {
	name := c.Param("name")
	
	result, err := h.sourceService.TestSource(c.Request.Context(), name)
	if err != nil {
		httputil.ErrorResponse(c, http.StatusInternalServerError, "Source test failed", err)
		return
	}

	httputil.SuccessResponse(c, result)
}

// GetSourceHealth godoc
// @Summary Get source health metrics
// @Description Get health and performance metrics for a source
// @Tags sources
// @Accept json
// @Produce json
// @Param name path string true "Source name"
// @Success 200 {object} httputil.Response
// @Router /api/v1/sources/{name}/health [get]
func (h *SourceHandler) GetSourceHealth(c *gin.Context) {
	name := c.Param("name")
	
	health, err := h.sourceService.GetSourceHealth(name)
	if err != nil {
		httputil.ErrorResponse(c, http.StatusInternalServerError, "Failed to get source health", err)
		return
	}

	httputil.SuccessResponse(c, health)
}
