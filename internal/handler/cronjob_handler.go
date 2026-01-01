package handler

import (
	"net/http"

	"crawl-news/internal/service"
	"crawl-news/pkg/httputil"

	"github.com/gin-gonic/gin"
)

type CronJobHandler struct {
	cronJobService *service.CronJobService
}

func NewCronJobHandler(cronJobService *service.CronJobService) *CronJobHandler {
	return &CronJobHandler{
		cronJobService: cronJobService,
	}
}

// StartCronJob godoc
// @Summary Start cron job service
// @Description Start the automatic crawling cron job service
// @Tags cronjob
// @Accept json
// @Produce json
// @Success 200 {object} httputil.Response
// @Router /api/v1/cronjob/start [post]
func (h *CronJobHandler) StartCronJob(c *gin.Context) {
	err := h.cronJobService.Start(c.Request.Context())
	if err != nil {
		httputil.ErrorResponse(c, http.StatusInternalServerError, "Failed to start cron job", err)
		return
	}

	httputil.SuccessResponse(c, gin.H{
		"message": "Cron job service started successfully",
	})
}

// StopCronJob godoc
// @Summary Stop cron job service
// @Description Stop the automatic crawling cron job service
// @Tags cronjob
// @Accept json
// @Produce json
// @Success 200 {object} httputil.Response
// @Router /api/v1/cronjob/stop [post]
func (h *CronJobHandler) StopCronJob(c *gin.Context) {
	h.cronJobService.Stop()

	httputil.SuccessResponse(c, gin.H{
		"message": "Cron job service stopped successfully",
	})
}

// GetCronJobStatus godoc
// @Summary Get cron job status
// @Description Get current status of the cron job service
// @Tags cronjob
// @Accept json
// @Produce json
// @Success 200 {object} httputil.Response
// @Router /api/v1/cronjob/status [get]
func (h *CronJobHandler) GetCronJobStatus(c *gin.Context) {
	status := h.cronJobService.GetStatus()
	httputil.SuccessResponse(c, status)
}

// SetCronJobInterval godoc
// @Summary Set cron job interval
// @Description Set the interval for the cron job (cron expression)
// @Tags cronjob
// @Accept json
// @Produce json
// @Param interval body map[string]string true "Cron interval"
// @Success 200 {object} httputil.Response
// @Router /api/v1/cronjob/interval [post]
func (h *CronJobHandler) SetCronJobInterval(c *gin.Context) {
	var req struct {
		Interval string `json:"interval" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.ErrorResponse(c, http.StatusBadRequest, "Invalid request", err)
		return
	}

	err := h.cronJobService.SetInterval(req.Interval)
	if err != nil {
		httputil.ErrorResponse(c, http.StatusBadRequest, "Failed to set interval", err)
		return
	}

	httputil.SuccessResponse(c, gin.H{
		"message": "Interval updated successfully",
		"interval": req.Interval,
	})
}

// TriggerCronJobNow godoc
// @Summary Trigger cron job now
// @Description Manually trigger a crawl job immediately
// @Tags cronjob
// @Accept json
// @Produce json
// @Success 200 {object} httputil.Response
// @Router /api/v1/cronjob/trigger [post]
func (h *CronJobHandler) TriggerCronJobNow(c *gin.Context) {
	err := h.cronJobService.TriggerNow(c.Request.Context())
	if err != nil {
		httputil.ErrorResponse(c, http.StatusInternalServerError, "Failed to trigger crawl", err)
		return
	}

	httputil.SuccessResponse(c, gin.H{
		"message": "Crawl job triggered successfully",
	})
}

