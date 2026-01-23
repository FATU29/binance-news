package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/gocolly/colly/v2"

	"crawl-news/internal/crawler"
	"crawl-news/internal/model"
	"crawl-news/pkg/logger"
)

// AdaptiveCrawler can learn and adapt to website structure changes
type AdaptiveCrawler struct {
	baseCrawler      *crawler.Crawler
	selectorLearner  *SelectorLearner
	structureMonitor *StructureMonitor
}

func NewAdaptiveCrawler(baseCrawler *crawler.Crawler, learner *SelectorLearner, monitor *StructureMonitor) *AdaptiveCrawler {
	return &AdaptiveCrawler{
		baseCrawler:      baseCrawler,
		selectorLearner:  learner,
		structureMonitor: monitor,
	}
}

// CrawlAdaptive crawls with automatic selector fallback and learning
func (ac *AdaptiveCrawler) CrawlAdaptive(ctx context.Context, source *model.CrawlSource) ([]*model.News, error) {
	logger.Info("Starting adaptive crawl for: %s", source.Name)

	var results []*model.News

	// Try with configured selectors first
	collector := colly.NewCollector(
		colly.AllowedDomains(extractDomain(source.BaseURL)),
		colly.MaxDepth(2),
	)

	articlesFound := 0

	// Try primary selector for article list
	primaryArticleSelector := source.Selectors.ArticleList

	collector.OnHTML(primaryArticleSelector, func(e *colly.HTMLElement) {
		articlesFound++
		news := ac.extractNewsFromElement(e, source)
		if news != nil {
			results = append(results, news)
		}
	})

	collector.OnError(func(r *colly.Response, err error) {
		logger.Error("Crawl error for %s: %v", source.Name, err)
	})

	err := collector.Visit(source.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to visit %s: %w", source.BaseURL, err)
	}

	// If primary selector failed or found too few results, try fallback
	if articlesFound < 3 {
		logger.Warn("Primary selector found only %d articles for %s, trying fallbacks", articlesFound, source.Name)

		fallbackResults, err := ac.tryFallbackSelectors(ctx, source)
		if err != nil {
			logger.Error("Fallback crawl failed for %s: %v", source.Name, err)
			// Record failure for structure monitoring
			ac.selectorLearner.RecordSelectorFailure(source.Name, "article_list", primaryArticleSelector)
		} else {
			results = append(results, fallbackResults...)
			logger.Info("Fallback crawl found %d additional articles", len(fallbackResults))
		}
	} else {
		// Record success
		ac.selectorLearner.RecordSelectorSuccess(source.Name, "article_list", primaryArticleSelector)
	}

	// If still no results, trigger auto-discovery
	if len(results) == 0 {
		logger.Warn("No results found with configured selectors, triggering auto-discovery for %s", source.Name)

		discoveryResults, err := ac.autoDiscoverAndCrawl(ctx, source)
		if err != nil {
			logger.Error("Auto-discovery failed for %s: %v", source.Name, err)
			return results, fmt.Errorf("all crawl strategies failed")
		}

		results = discoveryResults
		logger.Info("Auto-discovery found %d articles", len(results))
	}

	return results, nil
}

// tryFallbackSelectors attempts to crawl using learned fallback selectors
func (ac *AdaptiveCrawler) tryFallbackSelectors(ctx context.Context, source *model.CrawlSource) ([]*model.News, error) {
	// Get fallback selectors from learner
	fallbacks, err := ac.selectorLearner.GetFallbackSelectors(source.Name, "article_list")
	if err != nil || len(fallbacks) == 0 {
		return nil, fmt.Errorf("no fallback selectors available")
	}

	var results []*model.News

	for _, fallback := range fallbacks {
		if !fallback.IsActive || fallback.Selector == source.Selectors.ArticleList {
			continue // Skip inactive or already tried selectors
		}

		logger.Info("Trying fallback selector for %s: %s (confidence: %.2f%%)",
			source.Name, fallback.Selector, fallback.Confidence)

		collector := colly.NewCollector(
			colly.AllowedDomains(extractDomain(source.BaseURL)),
			colly.MaxDepth(2),
		)

		foundWithThisFallback := 0

		collector.OnHTML(fallback.Selector, func(e *colly.HTMLElement) {
			news := ac.extractNewsFromElement(e, source)
			if news != nil {
				results = append(results, news)
				foundWithThisFallback++
			}
		})

		err := collector.Visit(source.BaseURL)
		if err != nil {
			ac.selectorLearner.RecordSelectorFailure(source.Name, "article_list", fallback.Selector)
			continue
		}

		if foundWithThisFallback > 0 {
			// Success! Record it and potentially promote this selector
			ac.selectorLearner.RecordSelectorSuccess(source.Name, "article_list", fallback.Selector)

			if foundWithThisFallback >= 5 {
				// If it found good results, promote it
				logger.Info("Fallback selector performed well, promoting: %s", fallback.Selector)
				ac.selectorLearner.PromoteSelector(source.Name, "article_list", fallback.Selector)

				// Update source configuration
				source.Selectors.ArticleList = fallback.Selector
			}

			break // Found working selector
		}
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("all fallback selectors failed")
	}

	return results, nil
}

// autoDiscoverAndCrawl discovers selectors and crawls
func (ac *AdaptiveCrawler) autoDiscoverAndCrawl(ctx context.Context, source *model.CrawlSource) ([]*model.News, error) {
	logger.Info("Starting auto-discovery for %s", source.Name)

	// Discover selectors
	discovered, err := ac.selectorLearner.DiscoverSelectors(ctx, source.BaseURL, source.Name)
	if err != nil {
		return nil, fmt.Errorf("selector discovery failed: %w", err)
	}

	if len(discovered) == 0 {
		return nil, fmt.Errorf("no selectors discovered")
	}

	// Try discovered selectors
	var results []*model.News

	for _, selector := range discovered {
		if selector.ElementType != "article_list" {
			continue
		}

		logger.Info("Trying discovered selector: %s (confidence: %.2f%%)",
			selector.Selector, selector.Confidence)

		collector := colly.NewCollector(
			colly.AllowedDomains(extractDomain(source.BaseURL)),
			colly.MaxDepth(2),
		)

		foundCount := 0

		collector.OnHTML(selector.Selector, func(e *colly.HTMLElement) {
			news := ac.extractNewsFromElement(e, source)
			if news != nil {
				results = append(results, news)
				foundCount++
			}
		})

		err := collector.Visit(source.BaseURL)
		if err != nil {
			continue
		}

		if foundCount > 0 {
			logger.Info("Discovered selector found %d articles, updating configuration", foundCount)

			// Update source configuration with discovered selector
			source.Selectors.ArticleList = selector.Selector
			ac.selectorLearner.PromoteSelector(source.Name, "article_list", selector.Selector)

			break
		}
	}

	return results, nil
}

// extractNewsFromElement extracts news data from HTML element with adaptive selectors
func (ac *AdaptiveCrawler) extractNewsFromElement(e *colly.HTMLElement, source *model.CrawlSource) *model.News {
	// Try to extract with multiple strategies

	// 1. Try configured selectors
	title := strings.TrimSpace(e.ChildText(source.Selectors.Title))
	link := e.ChildAttr(source.Selectors.ArticleLink, "href")

	// 2. If failed, try common patterns
	if title == "" {
		title = ac.tryExtractTitle(e)
	}

	if link == "" {
		link = ac.tryExtractLink(e)
	}

	// Must have at least title and link
	if title == "" || link == "" {
		return nil
	}

	// Make URL absolute
	if !strings.HasPrefix(link, "http") {
		link = source.BaseURL + link
	}

	news := &model.News{
		Title:     title,
		SourceURL: link,
		Source:    source.Name,
	}

	// Try to extract other fields with fallbacks
	news.Summary = ac.tryExtractSummary(e, source)
	news.ImageURL = ac.tryExtractImage(e, source)
	news.Author = ac.tryExtractAuthor(e, source)

	return news
}

// tryExtractTitle tries multiple strategies to extract title
func (ac *AdaptiveCrawler) tryExtractTitle(e *colly.HTMLElement) string {
	selectors := []string{
		"h1", "h2", "h3",
		".title", ".headline", ".post-title",
		"[itemprop='headline']",
		"a[href]", // Last resort
	}

	for _, sel := range selectors {
		title := strings.TrimSpace(e.ChildText(sel))
		if title != "" && len(title) > 10 && len(title) < 300 {
			return title
		}
	}

	return ""
}

// tryExtractLink tries multiple strategies to extract link
func (ac *AdaptiveCrawler) tryExtractLink(e *colly.HTMLElement) string {
	selectors := []string{
		"a", "a.title", "a.headline",
		".title a", ".headline a",
	}

	for _, sel := range selectors {
		link := e.ChildAttr(sel, "href")
		if link != "" && (strings.Contains(link, "http") || strings.HasPrefix(link, "/")) {
			return link
		}
	}

	return ""
}

// tryExtractSummary tries multiple strategies to extract summary
func (ac *AdaptiveCrawler) tryExtractSummary(e *colly.HTMLElement, source *model.CrawlSource) string {
	selectors := []string{
		source.Selectors.Summary,
		".summary", ".excerpt", ".description",
		".lead", ".intro", "p",
		"[itemprop='description']",
	}

	for _, sel := range selectors {
		if sel == "" {
			continue
		}
		summary := strings.TrimSpace(e.ChildText(sel))
		if summary != "" && len(summary) > 20 && len(summary) < 1000 {
			return summary
		}
	}

	return ""
}

// tryExtractImage tries multiple strategies to extract image
func (ac *AdaptiveCrawler) tryExtractImage(e *colly.HTMLElement, source *model.CrawlSource) string {
	selectors := []string{
		source.Selectors.ImageURL,
		"img", ".thumbnail img", ".featured-image img",
		"figure img", "[itemprop='image']",
	}

	for _, sel := range selectors {
		if sel == "" {
			continue
		}
		img := e.ChildAttr(sel, "src")
		if img != "" {
			return img
		}
		// Try data-src for lazy loaded images
		img = e.ChildAttr(sel, "data-src")
		if img != "" {
			return img
		}
	}

	return ""
}

// tryExtractAuthor tries multiple strategies to extract author
func (ac *AdaptiveCrawler) tryExtractAuthor(e *colly.HTMLElement, source *model.CrawlSource) string {
	selectors := []string{
		source.Selectors.Author,
		".author", ".by-author", ".post-author",
		"[rel='author']", "[itemprop='author']",
		".author-name", ".byline",
	}

	for _, sel := range selectors {
		if sel == "" {
			continue
		}
		author := strings.TrimSpace(e.ChildText(sel))
		if author != "" && len(author) < 100 {
			return author
		}
	}

	return ""
}

// extractDomain extracts domain from URL for colly
func extractDomain(urlStr string) string {
	// Remove protocol
	domain := strings.TrimPrefix(urlStr, "https://")
	domain = strings.TrimPrefix(domain, "http://")

	// Remove path
	if idx := strings.Index(domain, "/"); idx != -1 {
		domain = domain[:idx]
	}

	// Remove www. for matching
	domain = strings.TrimPrefix(domain, "www.")

	return domain
}
