package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"crawl-news/internal/config"
	"crawl-news/internal/db"
	"crawl-news/internal/handler"
	"crawl-news/internal/repository"
	"crawl-news/internal/service"
	"crawl-news/pkg/logger"

	"github.com/gin-gonic/gin"
)

func main() {
	// Initialize logger
	logger.Init()

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Initialize database
	if err := db.InitDB(&cfg.Database); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	logger.Info("Database initialized successfully")
	defer db.Close()

	// Initialize Redis
	if err := db.InitRedis(&cfg.Redis); err != nil {
		log.Fatalf("Failed to initialize Redis: %v", err)
	}
	logger.Info("Redis initialized successfully")
	defer db.CloseRedis()

	// Set Gin mode
	if cfg.Server.Environment == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	// Initialize repositories
	newsRepo := repository.NewNewsRepository(&cfg.Redis)

	// Initialize services
	crawlerService := service.NewCrawlerService(newsRepo)
	newsService := service.NewNewsService(newsRepo)
	aiService := service.NewAIService(newsRepo, &cfg.AIService)
	analyticsService := service.NewAnalyticsService(newsRepo)
	cronJobService := service.NewCronJobService(crawlerService, newsRepo)

	// Connect AI service to crawler for auto-analysis
	crawlerService.SetAIService(aiService)

	// Initialize handlers
	newsHandler := handler.NewNewsHandler(newsService, crawlerService)
	crawlerHandler := handler.NewCrawlerHandler(crawlerService)
	aiHandler := handler.NewAIHandler(aiService, newsService)
	analyticsHandler := handler.NewAnalyticsHandler(analyticsService)
	cronJobHandler := handler.NewCronJobHandler(cronJobService)
	healthHandler := handler.NewHealthHandler()

	// Start cron job service if enabled
	if cfg.CronJob.Enabled {
		logger.Info("Starting cron job service...")
		if err := cronJobService.Start(context.Background()); err != nil {
			logger.Error("Failed to start cron job service: %v", err)
		} else {
			logger.Info("Cron job service started successfully")
		}
		defer cronJobService.Stop()
	}

	// Setup router
	router := setupRouter(newsHandler, crawlerHandler, aiHandler, analyticsHandler, cronJobHandler, healthHandler)

	// Create HTTP server
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	// Start server in goroutine
	go func() {
		logger.Info("Starting server on port %d", cfg.Server.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down server...")

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	logger.Info("Server exited")
}

func setupRouter(newsHandler *handler.NewsHandler, crawlerHandler *handler.CrawlerHandler, aiHandler *handler.AIHandler, analyticsHandler *handler.AnalyticsHandler, cronJobHandler *handler.CronJobHandler, healthHandler *handler.HealthHandler) *gin.Engine {
	router := gin.New()

	// Global middleware
	router.Use(gin.Recovery())
	router.Use(logger.GinLogger())
	// CORS is handled by the gateway, so we don't need it here
	// router.Use(middleware.CORS())

	// Health check endpoints
	router.GET("/health", healthHandler.Health)
	router.GET("/ready", healthHandler.Ready)

	// API v1 routes
	v1 := router.Group("/api/v1")
	{
		// News endpoints
		news := v1.Group("/news")
		{
			news.GET("", newsHandler.GetNews)
			news.GET("/filter", newsHandler.GetNewsWithFilter) // New endpoint for filtered + paginated
			news.GET("/advanced", newsHandler.GetNewsAdvanced)
			news.GET("/pair/:pair", newsHandler.GetNewsByTradingPair)
			news.GET("/summaries", newsHandler.GetNewsSummaries)
			news.GET("/:id", newsHandler.GetNewsByID)
			news.POST("/:id/fetch-detail", newsHandler.FetchNewsDetail)
		}

		// Crawler endpoints
		crawler := v1.Group("/crawler")
		{
			crawler.POST("/start", crawlerHandler.StartCrawler)
			crawler.POST("/stop", crawlerHandler.StopCrawler)
			crawler.GET("/status", crawlerHandler.GetStatus)
		}

		// CronJob endpoints
		cronjob := v1.Group("/cronjob")
		{
			cronjob.POST("/start", cronJobHandler.StartCronJob)
			cronjob.POST("/stop", cronJobHandler.StopCronJob)
			cronjob.GET("/status", cronJobHandler.GetCronJobStatus)
			cronjob.POST("/interval", cronJobHandler.SetCronJobInterval)
			cronjob.POST("/trigger", cronJobHandler.TriggerCronJobNow)
		}

		// AI Analysis endpoints
		ai := v1.Group("/ai")
		{
			ai.POST("/sentiment/:id", aiHandler.AnalyzeSentiment)
			ai.POST("/price-impact", aiHandler.AnalyzePriceImpact)
			ai.POST("/analyze", aiHandler.FullAnalysis)
			ai.POST("/batch-analyze", aiHandler.BatchAnalyze)
			ai.GET("/analyzed-news", aiHandler.GetAnalyzedNews)
			ai.GET("/unanalyzed-news", aiHandler.GetUnanalyzedNews)
		}

		// Analytics endpoints
		analytics := v1.Group("/analytics")
		{
			analytics.GET("/crawl", analyticsHandler.GetCrawlStats)
			analytics.GET("/page", analyticsHandler.GetPageStats)
			analytics.GET("/source/:source", analyticsHandler.GetSourceAnalytics)
			analytics.GET("/sentiment/trends", analyticsHandler.GetSentimentTrends)
			analytics.GET("/sentiment/pair/:pair", analyticsHandler.GetSentimentByPair)
		}
	}

	return router
}
