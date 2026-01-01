package service

import (
	"context"
	"strconv"

	"crawl-news/internal/model"
	"crawl-news/internal/repository"
)

type NewsService struct {
	newsRepo *repository.NewsRepository
}

func NewNewsService(newsRepo *repository.NewsRepository) *NewsService {
	return &NewsService{
		newsRepo: newsRepo,
	}
}

func (s *NewsService) GetNews(ctx context.Context, pageStr, limitStr, source, category string) ([]model.News, int, error) {
	page, _ := strconv.Atoi(pageStr)
	if page <= 0 {
		page = 1
	}

	limit, _ := strconv.Atoi(limitStr)
	if limit <= 0 {
		limit = 20
	}

	return s.newsRepo.FindAll(ctx, page, limit, source, category)
}

func (s *NewsService) GetNewsByID(ctx context.Context, id string) (*model.News, error) {
	return s.newsRepo.FindByID(ctx, id)
}

func (s *NewsService) CreateNews(ctx context.Context, news *model.News) error {
	return s.newsRepo.Create(ctx, news)
}

func (s *NewsService) UpdateNews(ctx context.Context, news *model.News) error {
	return s.newsRepo.Update(ctx, news)
}

func (s *NewsService) DeleteNews(ctx context.Context, id string) error {
	return s.newsRepo.Delete(ctx, id)
}

// GetNewsWithFilter retrieves news with advanced filtering
func (s *NewsService) GetNewsWithFilter(ctx context.Context, filter *model.NewsFilter) ([]model.News, error) {
	return s.newsRepo.FindWithFilter(ctx, filter)
}

// GetNewsByTradingPair retrieves news related to a specific trading pair
func (s *NewsService) GetNewsByTradingPair(ctx context.Context, pair string) ([]model.News, error) {
	return s.newsRepo.FindByTradingPair(ctx, pair)
}

// GetNewsSummaries retrieves simplified news data for listing
func (s *NewsService) GetNewsSummaries(ctx context.Context, filter *model.NewsFilter, limit int) ([]model.NewsSummary, error) {
	return s.newsRepo.GetNewsSummaries(ctx, filter, limit)
}

// GetUnanalyzedNews retrieves news that haven't been analyzed by AI
func (s *NewsService) GetUnanalyzedNews(ctx context.Context, limit int) ([]model.News, error) {
	return s.newsRepo.GetUnanalyzedNews(ctx, limit)
}

// GetNewsWithFilterPaginated retrieves news with filters and pagination
func (s *NewsService) GetNewsWithFilterPaginated(ctx context.Context, filter *model.NewsFilter, page, limit int) ([]model.News, int, error) {
	return s.newsRepo.FindWithFilterPaginated(ctx, filter, page, limit)
}
