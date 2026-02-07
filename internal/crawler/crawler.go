package crawler

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"crawl-news/internal/model"
	"crawl-news/pkg/logger"

	"github.com/gocolly/colly/v2"
	"github.com/mmcdole/gofeed"
)

// Crawler handles web scraping operations
type Crawler struct {
	sources  []model.CrawlSource
	aiParser AIParserInterface // Optional AI parser for fallback
}

// AIParserInterface defines interface for AI HTML parser
type AIParserInterface interface {
	ParseHTMLToNews(ctx context.Context, htmlContent string, url string, sourceName string) (*model.News, error)
	IsEnabled() bool
	IsForceAIParsing() bool
}

func NewCrawler() *Crawler {
	return &Crawler{
		sources:  getDefaultSources(),
		aiParser: nil,
	}
}

// SetAIParser sets the AI parser for fallback parsing
func (c *Crawler) SetAIParser(parser AIParserInterface) {
	c.aiParser = parser
}

// Crawl executes the crawling operation for a specific source
func (c *Crawler) Crawl(ctx context.Context, sourceName string) ([]*model.News, error) {
	logger.Info("Starting crawl for source: %s", sourceName)

	// Find the source configuration
	var source *model.CrawlSource
	for _, s := range c.sources {
		if s.Name == sourceName {
			source = &s
			break
		}
	}

	if source == nil {
		return nil, fmt.Errorf("source %s not found", sourceName)
	}

	if !source.Enabled {
		return nil, fmt.Errorf("source %s is disabled", sourceName)
	}

	// Use specific crawler based on source
	var results []*model.News
	var err error

	switch sourceName {
	case "cointelegraph":
		results, err = c.crawlCoinTelegraph(ctx, source)
	case "coindesk":
		results, err = c.crawlCoinDesk(ctx, source)
	case "cryptonews":
		results, err = c.crawlCryptoNews(ctx, source)
	case "binance":
		results, err = c.crawlBinance(ctx, source)
	case "coinmarketcap":
		results, err = c.crawlCoinMarketCap(ctx, source)
	case "bitcoincom":
		results, err = c.crawlBitcoinCom(ctx, source)
	case "theblock":
		results, err = c.crawlTheBlock(ctx, source)
	case "decrypt":
		results, err = c.crawlDecrypt(ctx, source)
	case "utoday":
		results, err = c.crawlUToday(ctx, source)
	case "cryptoslate":
		results, err = c.crawlCryptoSlate(ctx, source)
	case "beincrypto":
		results, err = c.crawlRSSFeed(ctx, source, "https://beincrypto.com/feed/", "bi")
	case "ambcrypto":
		results, err = c.crawlRSSFeed(ctx, source, "https://ambcrypto.com/feed/", "am")
	case "newsbtc":
		results, err = c.crawlRSSFeed(ctx, source, "https://www.newsbtc.com/feed/", "nb")
	case "bitcoinmagazine":
		results, err = c.crawlRSSFeed(ctx, source, "https://bitcoinmagazine.com/.rss/full/", "bm")
	case "cryptopotato":
		results, err = c.crawlRSSFeed(ctx, source, "https://cryptopotato.com/feed/", "cp")
	case "99bitcoins":
		results, err = c.crawlRSSFeed(ctx, source, "https://99bitcoins.com/feed/", "99")
	default:
		return nil, fmt.Errorf("no crawler implementation for source: %s", sourceName)
	}

	if err != nil {
		logger.Error("Crawl failed for source %s: %v", sourceName, err)
		return nil, err
	}

	logger.Info("Crawl completed for source: %s. Found %d items", sourceName, len(results))
	return results, nil
}

func (c *Crawler) crawlCoinTelegraph(ctx context.Context, source *model.CrawlSource) ([]*model.News, error) {
	var results []*model.News
	var mu sync.Mutex

	// Main collector for article list
	listCollector := colly.NewCollector(
		colly.AllowedDomains("cointelegraph.com", "www.cointelegraph.com"),
		colly.MaxDepth(1),
	)

	// Detail collector for individual articles
	detailCollector := listCollector.Clone()

	// Set up detail collector FIRST
	detailCollector.OnHTML("article, .post, .article-content", func(detailEl *colly.HTMLElement) {
		url := detailEl.Request.URL.String()

		mu.Lock()
		defer mu.Unlock()

		// Find the news item by URL
		for i, news := range results {
			if news.SourceURL == url {
				// Extract full content
				content := detailEl.ChildText(".post-content, .post__content, article p")
				if content != "" && len(content) > 100 {
					results[i].Content = strings.TrimSpace(content)
					logger.Info("Extracted content for: %s (%d chars)", news.Title, len(content))
				}

				// Extract author
				author := detailEl.ChildText(".post-meta__author-name, .author-name, .post__author")
				if author != "" {
					results[i].Author = strings.TrimSpace(author)
				}

				// Extract better image if available
				detailImage := detailEl.ChildAttr(".post__picture img, .post-cover img, .article-image img", "src")
				if detailImage != "" && results[i].ImageURL == "" {
					results[i].ImageURL = detailImage
				}

				// Extract tags
				var tags []string
				detailEl.ForEach(".post__tags a, .tags a, .article-tags a", func(_ int, tagEl *colly.HTMLElement) {
					tag := strings.TrimSpace(tagEl.Text)
					if tag != "" {
						tags = append(tags, tag)
					}
				})
				if len(tags) > 0 {
					results[i].Tags = tags
				}
				break
			}
		}
	})

	listCollector.OnHTML(".post-card-inline", func(e *colly.HTMLElement) {
		title := e.ChildText("a.post-card-inline__title-link")
		link := e.ChildAttr("a.post-card-inline__title-link", "href")
		summary := e.ChildText(".post-card-inline__text")
		imageURL := e.ChildAttr("img", "src")

		if title == "" || link == "" {
			return
		}

		// Make URL absolute
		if !strings.HasPrefix(link, "http") {
			link = source.BaseURL + link
		}

		// Generate unique ID from URL
		hash := md5.Sum([]byte(link))
		id := fmt.Sprintf("ct-%s", hex.EncodeToString(hash[:])[:16])

		// Parse published date from <time datetime="..."> element
		publishedAt := time.Now()
		dateTimeAttr := e.ChildAttr("time.post-card-inline__date", "datetime")
		relativeText := strings.TrimSpace(e.ChildText("time.post-card-inline__date"))

		if dateTimeAttr != "" {
			// Try parsing the datetime attribute (format: "2026-02-06")
			if parsed, err := time.Parse("2006-01-02", dateTimeAttr); err == nil {
				publishedAt = parsed
				// Try to refine with relative time text (e.g., "3 hours ago", "25 minutes ago")
				if strings.Contains(relativeText, "ago") {
					publishedAt = parseRelativeTime(relativeText)
				}
			}
		}

		news := &model.News{
			ID:          id,
			Title:       strings.TrimSpace(title),
			Summary:     strings.TrimSpace(summary),
			Source:      source.Name,
			SourceURL:   link,
			ImageURL:    imageURL,
			Category:    "crypto",
			Language:    "en",
			PublishedAt: publishedAt,
			CrawledAt:   time.Now(),
		}

		mu.Lock()
		results = append(results, news)
		articleCount := len(results)
		mu.Unlock()

		// Try to fetch full article content (limit to first 5 articles to avoid too long crawl)
		if articleCount <= 5 {
			logger.Info("Fetching details for: %s", title)
			detailCollector.Visit(link)
		}
	})

	listCollector.OnError(func(r *colly.Response, err error) {
		logger.Error("CoinTelegraph crawl error: %v", err)
	})

	err := listCollector.Visit(source.BaseURL + "/tags/bitcoin")
	if err != nil {
		return nil, fmt.Errorf("failed to visit CoinTelegraph: %w", err)
	}

	listCollector.Wait()
	detailCollector.Wait()

	// After getting article list, crawl detail pages with AI parser
	if c.aiParser != nil && c.aiParser.IsEnabled() && len(results) > 0 {
		logger.Info("Starting AI-based detail page crawl for %d articles", len(results))
		c.crawlDetailPagesWithAI(ctx, results, source.Name, 5) // Max 5 concurrent requests
	}

	logger.Info("CoinTelegraph: Crawled %d articles", len(results))
	return results, nil
}

func (c *Crawler) crawlCoinDesk(ctx context.Context, source *model.CrawlSource) ([]*model.News, error) {
	var results []*model.News

	// CoinDesk has RSS feed - more reliable
	fp := gofeed.NewParser()
	feed, err := fp.ParseURL("https://www.coindesk.com/arc/outboundfeeds/rss/")
	if err != nil {
		logger.Error("CoinDesk RSS feed error: %v", err)
		return nil, fmt.Errorf("failed to parse CoinDesk RSS: %w", err)
	}

	for _, item := range feed.Items {
		if item == nil || item.Title == "" || item.Link == "" {
			continue
		}

		// Generate unique ID
		hash := md5.Sum([]byte(item.Link))
		id := fmt.Sprintf("cd-%s", hex.EncodeToString(hash[:])[:16])

		// Parse published date
		publishedAt := time.Now()
		if item.PublishedParsed != nil {
			publishedAt = *item.PublishedParsed
		}

		// Extract image from description or use default
		imageURL := ""
		if item.Image != nil && item.Image.URL != "" {
			imageURL = item.Image.URL
		}

		news := &model.News{
			ID:          id,
			Title:       strings.TrimSpace(item.Title),
			Summary:     strings.TrimSpace(item.Description),
			Content:     strings.TrimSpace(item.Content),
			Source:      source.Name,
			SourceURL:   item.Link,
			ImageURL:    imageURL,
			Category:    "crypto",
			Language:    "en",
			PublishedAt: publishedAt,
			CrawledAt:   time.Now(),
		}

		// Extract author if available
		if len(item.Authors) > 0 {
			news.Author = item.Authors[0].Name
		}

		// Extract categories as tags
		if len(item.Categories) > 0 {
			news.Tags = item.Categories
		}

		results = append(results, news)
	}

	logger.Info("CoinDesk RSS: Found %d items", len(results))

	// After getting article list, crawl detail pages with AI parser
	if c.aiParser != nil && c.aiParser.IsEnabled() && len(results) > 0 {
		logger.Info("Starting AI-based detail page crawl for %d articles", len(results))
		c.crawlDetailPagesWithAI(ctx, results, source.Name, 5) // Max 5 concurrent requests
	}

	return results, nil
}

func (c *Crawler) crawlCryptoNews(ctx context.Context, source *model.CrawlSource) ([]*model.News, error) {
	var results []*model.News

	// CryptoNews has RSS feed
	fp := gofeed.NewParser()
	feed, err := fp.ParseURL("https://cryptonews.com/news/feed/")
	if err != nil {
		logger.Error("CryptoNews RSS feed error: %v", err)
		return nil, fmt.Errorf("failed to parse CryptoNews RSS: %w", err)
	}

	for _, item := range feed.Items {
		if item == nil || item.Title == "" || item.Link == "" {
			continue
		}

		// Generate unique ID
		hash := md5.Sum([]byte(item.Link))
		id := fmt.Sprintf("cn-%s", hex.EncodeToString(hash[:])[:16])

		// Parse published date
		publishedAt := time.Now()
		if item.PublishedParsed != nil {
			publishedAt = *item.PublishedParsed
		}

		// Extract image
		imageURL := ""
		if item.Image != nil && item.Image.URL != "" {
			imageURL = item.Image.URL
		}

		news := &model.News{
			ID:          id,
			Title:       strings.TrimSpace(item.Title),
			Summary:     strings.TrimSpace(item.Description),
			Content:     strings.TrimSpace(item.Content),
			Source:      source.Name,
			SourceURL:   item.Link,
			ImageURL:    imageURL,
			Category:    "crypto",
			Language:    "en",
			PublishedAt: publishedAt,
			CrawledAt:   time.Now(),
		}

		// Extract author
		if len(item.Authors) > 0 {
			news.Author = item.Authors[0].Name
		}

		// Extract categories
		if len(item.Categories) > 0 {
			news.Tags = item.Categories
		}

		results = append(results, news)
	}

	logger.Info("CryptoNews RSS: Found %d items", len(results))

	// After getting article list, crawl detail pages with AI parser
	if c.aiParser != nil && c.aiParser.IsEnabled() && len(results) > 0 {
		logger.Info("Starting AI-based detail page crawl for %d articles", len(results))
		c.crawlDetailPagesWithAI(ctx, results, source.Name, 5)
	}

	return results, nil
}

func (c *Crawler) crawlBinance(ctx context.Context, source *model.CrawlSource) ([]*model.News, error) {
	var results []*model.News

	// Option 1: Try Binance news API
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get("https://www.binance.com/bapi/composite/v1/public/cms/article/list/query?type=1&pageSize=20&pageNo=1")

	if err == nil && resp.StatusCode == 200 {
		defer resp.Body.Close()

		var apiResp struct {
			Data struct {
				Articles []struct {
					ID          int    `json:"id"`
					Code        string `json:"code"`
					Title       string `json:"title"`
					Body        string `json:"body"`
					Summary     string `json:"brief"`
					PublishTime int64  `json:"publishDate"`
					Cover       string `json:"cover"`
				} `json:"articles"`
			} `json:"data"`
		}

		if json.NewDecoder(resp.Body).Decode(&apiResp) == nil {
			for _, article := range apiResp.Data.Articles {
				url := fmt.Sprintf("https://www.binance.com/en/news/%s", article.Code)
				hash := md5.Sum([]byte(url))
				id := fmt.Sprintf("bn-%s", hex.EncodeToString(hash[:])[:16])

				publishedAt := time.Unix(apiResp.Data.Articles[0].PublishTime/1000, 0)

				news := &model.News{
					ID:          id,
					Title:       strings.TrimSpace(article.Title),
					Summary:     strings.TrimSpace(article.Summary),
					Content:     strings.TrimSpace(article.Body),
					Source:      "binance",
					SourceURL:   url,
					ImageURL:    article.Cover,
					Category:    "exchange",
					Language:    "en",
					PublishedAt: publishedAt,
					CrawledAt:   time.Now(),
				}

				results = append(results, news)
			}

			logger.Info("Binance API: Found %d items", len(results))
			return results, nil
		}
	}

	// Option 2: Fallback to RSS feed
	fp := gofeed.NewParser()
	feed, err := fp.ParseURL("https://www.binance.com/en/support/announcement/New%20Cryptocurrency%20Listing?c=48&navId=48&hl=en")
	if err != nil {
		logger.Error("Binance feed error: %v", err)
		return results, nil // Return empty results instead of error
	}

	for _, item := range feed.Items {
		if item == nil || item.Title == "" || item.Link == "" {
			continue
		}

		hash := md5.Sum([]byte(item.Link))
		id := fmt.Sprintf("bn-%s", hex.EncodeToString(hash[:])[:16])

		publishedAt := time.Now()
		if item.PublishedParsed != nil {
			publishedAt = *item.PublishedParsed
		}

		news := &model.News{
			ID:          id,
			Title:       strings.TrimSpace(item.Title),
			Summary:     strings.TrimSpace(item.Description),
			Content:     strings.TrimSpace(item.Content),
			Source:      "binance",
			SourceURL:   item.Link,
			Category:    "exchange",
			Language:    "en",
			PublishedAt: publishedAt,
			CrawledAt:   time.Now(),
		}

		results = append(results, news)
	}

	logger.Info("Binance RSS: Found %d items", len(results))

	// After getting article list, crawl detail pages with AI parser
	if c.aiParser != nil && c.aiParser.IsEnabled() && len(results) > 0 {
		logger.Info("Starting AI-based detail page crawl for %d articles", len(results))
		c.crawlDetailPagesWithAI(ctx, results, source.Name, 5)
	}

	return results, nil
}

func (c *Crawler) crawlCoinMarketCap(ctx context.Context, source *model.CrawlSource) ([]*model.News, error) {
	var results []*model.News

	// CoinMarketCap has RSS feed
	fp := gofeed.NewParser()
	feed, err := fp.ParseURL("https://coinmarketcap.com/headlines/rss/")
	if err != nil {
		logger.Error("CoinMarketCap RSS error: %v", err)
		return nil, fmt.Errorf("failed to parse CoinMarketCap RSS: %w", err)
	}

	for _, item := range feed.Items {
		if item == nil || item.Title == "" || item.Link == "" {
			continue
		}

		hash := md5.Sum([]byte(item.Link))
		id := fmt.Sprintf("cmc-%s", hex.EncodeToString(hash[:])[:16])

		publishedAt := time.Now()
		if item.PublishedParsed != nil {
			publishedAt = *item.PublishedParsed
		}

		imageURL := ""
		if item.Image != nil && item.Image.URL != "" {
			imageURL = item.Image.URL
		}

		news := &model.News{
			ID:          id,
			Title:       strings.TrimSpace(item.Title),
			Summary:     strings.TrimSpace(item.Description),
			Content:     strings.TrimSpace(item.Content),
			Source:      "coinmarketcap",
			SourceURL:   item.Link,
			ImageURL:    imageURL,
			Category:    "crypto",
			Language:    "en",
			PublishedAt: publishedAt,
			CrawledAt:   time.Now(),
		}

		if len(item.Categories) > 0 {
			news.Tags = item.Categories
		}

		results = append(results, news)
	}

	logger.Info("CoinMarketCap RSS: Found %d items", len(results))

	// After getting article list, crawl detail pages with AI parser
	if c.aiParser != nil && c.aiParser.IsEnabled() && len(results) > 0 {
		logger.Info("Starting AI-based detail page crawl for %d articles", len(results))
		c.crawlDetailPagesWithAI(ctx, results, source.Name, 5)
	}

	return results, nil
}

func (c *Crawler) crawlBitcoinCom(ctx context.Context, source *model.CrawlSource) ([]*model.News, error) {
	var results []*model.News

	// Bitcoin.com news RSS
	fp := gofeed.NewParser()
	feed, err := fp.ParseURL("https://news.bitcoin.com/feed/")
	if err != nil {
		logger.Error("Bitcoin.com RSS error: %v", err)
		return nil, fmt.Errorf("failed to parse Bitcoin.com RSS: %w", err)
	}

	for _, item := range feed.Items {
		if item == nil || item.Title == "" || item.Link == "" {
			continue
		}

		hash := md5.Sum([]byte(item.Link))
		id := fmt.Sprintf("bc-%s", hex.EncodeToString(hash[:])[:16])

		publishedAt := time.Now()
		if item.PublishedParsed != nil {
			publishedAt = *item.PublishedParsed
		}

		imageURL := ""
		if item.Image != nil && item.Image.URL != "" {
			imageURL = item.Image.URL
		}

		news := &model.News{
			ID:          id,
			Title:       strings.TrimSpace(item.Title),
			Summary:     strings.TrimSpace(item.Description),
			Content:     strings.TrimSpace(item.Content),
			Source:      "bitcoincom",
			SourceURL:   item.Link,
			ImageURL:    imageURL,
			Category:    "crypto",
			Language:    "en",
			PublishedAt: publishedAt,
			CrawledAt:   time.Now(),
		}

		if len(item.Authors) > 0 {
			news.Author = item.Authors[0].Name
		}

		if len(item.Categories) > 0 {
			news.Tags = item.Categories
		}

		results = append(results, news)
	}

	logger.Info("Bitcoin.com RSS: Found %d items", len(results))

	// After getting article list, crawl detail pages with AI parser
	if c.aiParser != nil && c.aiParser.IsEnabled() && len(results) > 0 {
		logger.Info("Starting AI-based detail page crawl for %d articles", len(results))
		c.crawlDetailPagesWithAI(ctx, results, source.Name, 5)
	}

	return results, nil
}

func (c *Crawler) crawlTheBlock(ctx context.Context, source *model.CrawlSource) ([]*model.News, error) {
	var results []*model.News

	// The Block RSS
	fp := gofeed.NewParser()
	feed, err := fp.ParseURL("https://www.theblock.co/rss.xml")
	if err != nil {
		logger.Error("The Block RSS error: %v", err)
		return nil, fmt.Errorf("failed to parse The Block RSS: %w", err)
	}

	for _, item := range feed.Items {
		if item == nil || item.Title == "" || item.Link == "" {
			continue
		}

		hash := md5.Sum([]byte(item.Link))
		id := fmt.Sprintf("tb-%s", hex.EncodeToString(hash[:])[:16])

		publishedAt := time.Now()
		if item.PublishedParsed != nil {
			publishedAt = *item.PublishedParsed
		}

		imageURL := ""
		if item.Image != nil && item.Image.URL != "" {
			imageURL = item.Image.URL
		}

		news := &model.News{
			ID:          id,
			Title:       strings.TrimSpace(item.Title),
			Summary:     strings.TrimSpace(item.Description),
			Content:     strings.TrimSpace(item.Content),
			Source:      "theblock",
			SourceURL:   item.Link,
			ImageURL:    imageURL,
			Category:    "crypto",
			Language:    "en",
			PublishedAt: publishedAt,
			CrawledAt:   time.Now(),
		}

		if len(item.Authors) > 0 {
			news.Author = item.Authors[0].Name
		}

		if len(item.Categories) > 0 {
			news.Tags = item.Categories
		}

		results = append(results, news)
	}

	logger.Info("The Block RSS: Found %d items", len(results))

	// After getting article list, crawl detail pages with AI parser
	if c.aiParser != nil && c.aiParser.IsEnabled() && len(results) > 0 {
		logger.Info("Starting AI-based detail page crawl for %d articles", len(results))
		c.crawlDetailPagesWithAI(ctx, results, source.Name, 5)
	}

	return results, nil
}

func (c *Crawler) crawlDecrypt(ctx context.Context, source *model.CrawlSource) ([]*model.News, error) {
	var results []*model.News

	// Decrypt RSS
	fp := gofeed.NewParser()
	feed, err := fp.ParseURL("https://decrypt.co/feed")
	if err != nil {
		logger.Error("Decrypt RSS error: %v", err)
		return nil, fmt.Errorf("failed to parse Decrypt RSS: %w", err)
	}

	for _, item := range feed.Items {
		if item == nil || item.Title == "" || item.Link == "" {
			continue
		}

		hash := md5.Sum([]byte(item.Link))
		id := fmt.Sprintf("dc-%s", hex.EncodeToString(hash[:])[:16])

		publishedAt := time.Now()
		if item.PublishedParsed != nil {
			publishedAt = *item.PublishedParsed
		}

		imageURL := ""
		if item.Image != nil && item.Image.URL != "" {
			imageURL = item.Image.URL
		}

		news := &model.News{
			ID:          id,
			Title:       strings.TrimSpace(item.Title),
			Summary:     strings.TrimSpace(item.Description),
			Content:     strings.TrimSpace(item.Content),
			Source:      "decrypt",
			SourceURL:   item.Link,
			ImageURL:    imageURL,
			Category:    "crypto",
			Language:    "en",
			PublishedAt: publishedAt,
			CrawledAt:   time.Now(),
		}

		if len(item.Authors) > 0 {
			news.Author = item.Authors[0].Name
		}

		if len(item.Categories) > 0 {
			news.Tags = item.Categories
		}

		results = append(results, news)
	}

	logger.Info("Decrypt RSS: Found %d items", len(results))

	// After getting article list, crawl detail pages with AI parser
	if c.aiParser != nil && c.aiParser.IsEnabled() && len(results) > 0 {
		logger.Info("Starting AI-based detail page crawl for %d articles", len(results))
		c.crawlDetailPagesWithAI(ctx, results, source.Name, 5)
	}

	return results, nil
}

func (c *Crawler) crawlUToday(ctx context.Context, source *model.CrawlSource) ([]*model.News, error) {
	var results []*model.News

	// U.Today RSS
	fp := gofeed.NewParser()
	feed, err := fp.ParseURL("https://u.today/rss")
	if err != nil {
		logger.Error("U.Today RSS error: %v", err)
		return nil, fmt.Errorf("failed to parse U.Today RSS: %w", err)
	}

	for _, item := range feed.Items {
		if item == nil || item.Title == "" || item.Link == "" {
			continue
		}

		hash := md5.Sum([]byte(item.Link))
		id := fmt.Sprintf("ut-%s", hex.EncodeToString(hash[:])[:16])

		publishedAt := time.Now()
		if item.PublishedParsed != nil {
			publishedAt = *item.PublishedParsed
		}

		imageURL := ""
		if item.Image != nil && item.Image.URL != "" {
			imageURL = item.Image.URL
		} else if len(item.Enclosures) > 0 && strings.HasPrefix(item.Enclosures[0].Type, "image") {
			imageURL = item.Enclosures[0].URL
		}

		news := &model.News{
			ID:          id,
			Title:       strings.TrimSpace(item.Title),
			Summary:     strings.TrimSpace(item.Description),
			Content:     strings.TrimSpace(item.Content),
			Source:      "utoday",
			SourceURL:   item.Link,
			ImageURL:    imageURL,
			Category:    "crypto",
			Language:    "en",
			PublishedAt: publishedAt,
			CrawledAt:   time.Now(),
		}

		if len(item.Authors) > 0 {
			news.Author = item.Authors[0].Name
		}

		if len(item.Categories) > 0 {
			news.Tags = item.Categories
		}

		results = append(results, news)
	}

	logger.Info("U.Today RSS: Found %d items", len(results))

	// After getting article list, crawl detail pages with AI parser
	if c.aiParser != nil && c.aiParser.IsEnabled() && len(results) > 0 {
		logger.Info("Starting AI-based detail page crawl for %d articles", len(results))
		c.crawlDetailPagesWithAI(ctx, results, source.Name, 5)
	}

	return results, nil
}

func (c *Crawler) crawlCryptoSlate(ctx context.Context, source *model.CrawlSource) ([]*model.News, error) {
	var results []*model.News

	// CryptoSlate RSS
	fp := gofeed.NewParser()
	feed, err := fp.ParseURL("https://cryptoslate.com/feed/")
	if err != nil {
		logger.Error("CryptoSlate RSS error: %v", err)
		return nil, fmt.Errorf("failed to parse CryptoSlate RSS: %w", err)
	}

	for _, item := range feed.Items {
		if item == nil || item.Title == "" || item.Link == "" {
			continue
		}

		hash := md5.Sum([]byte(item.Link))
		id := fmt.Sprintf("cs-%s", hex.EncodeToString(hash[:])[:16])

		publishedAt := time.Now()
		if item.PublishedParsed != nil {
			publishedAt = *item.PublishedParsed
		}

		imageURL := ""
		if item.Image != nil && item.Image.URL != "" {
			imageURL = item.Image.URL
		} else if len(item.Enclosures) > 0 && strings.HasPrefix(item.Enclosures[0].Type, "image") {
			imageURL = item.Enclosures[0].URL
		}

		news := &model.News{
			ID:          id,
			Title:       strings.TrimSpace(item.Title),
			Summary:     strings.TrimSpace(item.Description),
			Content:     strings.TrimSpace(item.Content),
			Source:      "cryptoslate",
			SourceURL:   item.Link,
			ImageURL:    imageURL,
			Category:    "crypto",
			Language:    "en",
			PublishedAt: publishedAt,
			CrawledAt:   time.Now(),
		}

		if len(item.Authors) > 0 {
			news.Author = item.Authors[0].Name
		}

		if len(item.Categories) > 0 {
			news.Tags = item.Categories
		}

		results = append(results, news)
	}

	logger.Info("CryptoSlate RSS: Found %d items", len(results))

	// After getting article list, crawl detail pages with AI parser
	if c.aiParser != nil && c.aiParser.IsEnabled() && len(results) > 0 {
		logger.Info("Starting AI-based detail page crawl for %d articles", len(results))
		c.crawlDetailPagesWithAI(ctx, results, source.Name, 5)
	}

	return results, nil
}

// crawlRSSFeed is a generic function to crawl any RSS feed
func (c *Crawler) crawlRSSFeed(ctx context.Context, source *model.CrawlSource, rssURL string, idPrefix string) ([]*model.News, error) {
	var results []*model.News

	fp := gofeed.NewParser()
	feed, err := fp.ParseURL(rssURL)
	if err != nil {
		logger.Error("%s RSS feed error: %v", source.Name, err)
		return nil, fmt.Errorf("failed to parse %s RSS: %w", source.Name, err)
	}

	for _, item := range feed.Items {
		if item == nil || item.Title == "" || item.Link == "" {
			continue
		}

		hash := md5.Sum([]byte(item.Link))
		id := fmt.Sprintf("%s-%s", idPrefix, hex.EncodeToString(hash[:])[:16])

		publishedAt := time.Now()
		if item.PublishedParsed != nil {
			publishedAt = *item.PublishedParsed
		}

		imageURL := ""
		if item.Image != nil && item.Image.URL != "" {
			imageURL = item.Image.URL
		} else if len(item.Enclosures) > 0 && strings.HasPrefix(item.Enclosures[0].Type, "image") {
			imageURL = item.Enclosures[0].URL
		}

		news := &model.News{
			ID:          id,
			Title:       strings.TrimSpace(item.Title),
			Summary:     strings.TrimSpace(item.Description),
			Content:     strings.TrimSpace(item.Content),
			Source:      source.Name,
			SourceURL:   item.Link,
			ImageURL:    imageURL,
			Category:    "crypto",
			Language:    "en",
			PublishedAt: publishedAt,
			CrawledAt:   time.Now(),
		}

		if len(item.Authors) > 0 {
			news.Author = item.Authors[0].Name
		}

		if len(item.Categories) > 0 {
			news.Tags = item.Categories
		}

		results = append(results, news)
	}

	logger.Info("%s RSS: Found %d items", source.Name, len(results))

	// After getting article list, crawl detail pages with AI parser
	if c.aiParser != nil && c.aiParser.IsEnabled() && len(results) > 0 {
		logger.Info("Starting AI-based detail page crawl for %d articles from %s", len(results), source.Name)
		c.crawlDetailPagesWithAI(ctx, results, source.Name, 5)
	}

	return results, nil
}

// getDefaultSources returns the default list of crawl sources
func getDefaultSources() []model.CrawlSource {
	return []model.CrawlSource{
		{
			Name:    "cointelegraph",
			BaseURL: "https://cointelegraph.com",
			Enabled: true,
			Selectors: model.Selector{
				ArticleList: ".post-card-inline",
				ArticleLink: "a.post-card-inline__title-link",
				Title:       "h1.post__title",
				Content:     ".post-content",
				Summary:     ".post__lead",
				Author:      ".post-meta__author-name",
				PublishedAt: ".post-meta__publish-date",
				ImageURL:    ".post__picture img",
			},
		},
		{
			Name:    "coindesk",
			BaseURL: "https://www.coindesk.com",
			Enabled: true,
			Selectors: model.Selector{
				ArticleList: ".article-cardstyles__StyledWrapper",
				ArticleLink: "a",
				Title:       "h1",
				Content:     ".article-body",
				Author:      ".author-name",
				PublishedAt: "time",
				ImageURL:    "img",
			},
		},
		{
			Name:    "cryptonews",
			BaseURL: "https://cryptonews.com",
			Enabled: true,
			Selectors: model.Selector{
				ArticleList: ".article-item",
				ArticleLink: "a.article-item__link",
				Title:       "h1.article__title",
				Content:     ".article__content",
				Author:      ".article__author",
				PublishedAt: ".article__date",
				ImageURL:    ".article__image img",
			},
		},
		{
			Name:      "binance",
			BaseURL:   "https://www.binance.com",
			Enabled:   true,
			Selectors: model.Selector{
				// Binance uses API/RSS, selectors not needed
			},
		},
		{
			Name:    "coinmarketcap",
			BaseURL: "https://coinmarketcap.com",
			Enabled: true,
			Selectors: model.Selector{
				ArticleList: ".sc-aef7b723-0",
				ArticleLink: "a",
				Title:       "h1",
				Content:     ".content",
			},
		},
		{
			Name:      "bitcoincom",
			BaseURL:   "https://news.bitcoin.com",
			Enabled:   true,
			Selectors: model.Selector{
				// RSS feed based
			},
		},
		{
			Name:      "theblock",
			BaseURL:   "https://www.theblock.co",
			Enabled:   true,
			Selectors: model.Selector{
				// RSS feed based
			},
		},
		{
			Name:      "decrypt",
			BaseURL:   "https://decrypt.co",
			Enabled:   true,
			Selectors: model.Selector{
				// RSS feed based
			},
		},
		{
			Name:      "utoday",
			BaseURL:   "https://u.today",
			Enabled:   true,
			Selectors: model.Selector{
				// RSS feed based
			},
		},
		{
			Name:      "cryptoslate",
			BaseURL:   "https://cryptoslate.com",
			Enabled:   true,
			Selectors: model.Selector{
				// RSS feed based
			},
		},
		{
			Name:      "beincrypto",
			BaseURL:   "https://beincrypto.com",
			Enabled:   true,
			Selectors: model.Selector{
				// RSS feed based
			},
		},
		{
			Name:      "ambcrypto",
			BaseURL:   "https://ambcrypto.com",
			Enabled:   true,
			Selectors: model.Selector{
				// RSS feed based
			},
		},
		{
			Name:      "newsbtc",
			BaseURL:   "https://www.newsbtc.com",
			Enabled:   true,
			Selectors: model.Selector{
				// RSS feed based
			},
		},
		{
			Name:      "bitcoinmagazine",
			BaseURL:   "https://bitcoinmagazine.com",
			Enabled:   true,
			Selectors: model.Selector{
				// RSS feed based
			},
		},
		{
			Name:      "cryptopotato",
			BaseURL:   "https://cryptopotato.com",
			Enabled:   true,
			Selectors: model.Selector{
				// RSS feed based
			},
		},
		{
			Name:      "99bitcoins",
			BaseURL:   "https://99bitcoins.com",
			Enabled:   true,
			Selectors: model.Selector{
				// RSS feed based
			},
		},
	}
}

// CrawlDetailPage crawls a specific URL to extract full article details
func (c *Crawler) CrawlDetailPage(ctx context.Context, url string) (*model.News, error) {
	logger.Info("Crawling detail page: %s", url)

	collector := colly.NewCollector(
		colly.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36"),
	)

	var news *model.News
	var mu sync.Mutex

	// Determine source from URL
	var sourceName string
	switch {
	case strings.Contains(url, "cointelegraph.com"):
		sourceName = "cointelegraph"
	case strings.Contains(url, "coindesk.com"):
		sourceName = "coindesk"
	case strings.Contains(url, "cryptonews.com"):
		sourceName = "cryptonews"
	case strings.Contains(url, "binance.com"):
		sourceName = "binance"
	case strings.Contains(url, "coinmarketcap.com"):
		sourceName = "coinmarketcap"
	case strings.Contains(url, "bitcoin.com"):
		sourceName = "bitcoincom"
	case strings.Contains(url, "theblock.co"):
		sourceName = "theblock"
	case strings.Contains(url, "decrypt.co"):
		sourceName = "decrypt"
	case strings.Contains(url, "u.today"):
		sourceName = "utoday"
	case strings.Contains(url, "cryptoslate.com"):
		sourceName = "cryptoslate"
	case strings.Contains(url, "beincrypto.com"):
		sourceName = "beincrypto"
	case strings.Contains(url, "ambcrypto.com"):
		sourceName = "ambcrypto"
	case strings.Contains(url, "newsbtc.com"):
		sourceName = "newsbtc"
	case strings.Contains(url, "bitcoinmagazine.com"):
		sourceName = "bitcoinmagazine"
	case strings.Contains(url, "cryptopotato.com"):
		sourceName = "cryptopotato"
	case strings.Contains(url, "99bitcoins.com"):
		sourceName = "99bitcoins"
	default:
		// Try to extract domain name as fallback
		if strings.Contains(url, "://") {
			parts := strings.Split(url, "://")
			if len(parts) > 1 {
				domain := strings.Split(parts[1], "/")[0]
				// Remove www. prefix
				domain = strings.TrimPrefix(domain, "www.")
				// Use domain as source name (sanitized)
				sourceName = strings.ToLower(strings.ReplaceAll(domain, ".", ""))
				logger.Info("Using generic extraction for unknown source: %s (from domain: %s)", sourceName, domain)
			} else {
				return nil, fmt.Errorf("unsupported source URL: %s", url)
			}
		} else {
			return nil, fmt.Errorf("unsupported source URL: %s", url)
		}
	}

	// Advanced HTML content analyzer
	collector.OnHTML("html", func(e *colly.HTMLElement) {
		mu.Lock()
		defer mu.Unlock()

		// Check if AI-only parsing is forced
		forceAI := c.aiParser != nil && c.aiParser.IsEnabled() && c.aiParser.IsForceAIParsing()

		if forceAI {
			// Force AI parsing - skip rule-based entirely
			logger.Info("AI-only mode: Using AI parser for: %s", url)

			// Get raw HTML
			htmlContent := string(e.Response.Body)

			// Try AI parsing
			aiNews, err := c.aiParser.ParseHTMLToNews(ctx, htmlContent, url, sourceName)
			if err == nil && aiNews != nil {
				news = aiNews
				logger.Info("AI parser successfully extracted article: %s (%d chars, method: %s, confidence: %.2f)",
					aiNews.Title, len(aiNews.Content), aiNews.ParsingMethod, aiNews.ParsingConfidence)
				return
			} else {
				logger.Error("AI parser failed in AI-only mode: %v", err)
				// In AI-only mode, we don't fallback to rule-based
				return
			}
		}

		// Normal mode: Try rule-based first, then AI as fallback
		// Extract title with multiple fallback strategies
		title := extractTitle(e)

		// Extract content with intelligent paragraph extraction
		content := extractContent(e, sourceName)

		// If rule-based extraction failed or got poor results (content < 500 chars for analysis), try AI parser
		// This ensures we have enough content for causal analysis (requires 500+ chars)
		if (title == "" || title == "Untitled Article" || len(content) < 500) && c.aiParser != nil && c.aiParser.IsEnabled() {
			logger.Info("Rule-based extraction insufficient (content: %d chars, need 500+ for analysis), trying AI parser for: %s", len(content), url)

			// Get raw HTML
			htmlContent := string(e.Response.Body)

			// Try AI parsing
			aiNews, err := c.aiParser.ParseHTMLToNews(ctx, htmlContent, url, sourceName)
			if err == nil && aiNews != nil {
				// AI parsing sets ParsingMethod and ParsingConfidence in ParseHTMLToNews
				news = aiNews
				logger.Info("AI parser successfully extracted article: %s (%d chars, method: %s, confidence: %.2f)",
					aiNews.Title, len(aiNews.Content), aiNews.ParsingMethod, aiNews.ParsingConfidence)
				return
			} else {
				logger.Warn("AI parser also failed: %v, falling back to rule-based", err)
			}
		}

		// Extract author
		author := extractAuthor(e)

		// Extract image with multiple selectors
		imageURL := extractImage(e)

		// Extract tags/categories
		tags := extractTags(e)

		// Extract published date
		publishedAt := extractPublishedDate(e)

		// Generate ID
		hash := md5.Sum([]byte(url))
		sourcePrefix := sourceName
		if len(sourceName) > 2 {
			sourcePrefix = sourceName[:2]
		}
		id := fmt.Sprintf("%s-%s", sourcePrefix, hex.EncodeToString(hash[:])[:16])

		// Create summary from content
		summary := createSummary(content, 300)

		news = &model.News{
			ID:                id,
			Title:             strings.TrimSpace(title),
			Content:           strings.TrimSpace(content),
			Summary:           summary,
			Author:            strings.TrimSpace(author),
			Source:            sourceName,
			SourceURL:         url,
			ImageURL:          imageURL,
			Category:          "crypto",
			Tags:              tags,
			Language:          "en",
			PublishedAt:       publishedAt,
			CrawledAt:         time.Now(),
			ParsingMethod:     "rule-based",
			ParsingConfidence: 0.8, // Default confidence for rule-based
		}

		logger.Info("Extracted article: %s (%d chars, author: %s)", title, len(content), author)
	})

	collector.OnError(func(r *colly.Response, err error) {
		logger.Error("Failed to crawl URL %s: %v", url, err)
	})

	// Visit the URL
	if err := collector.Visit(url); err != nil {
		return nil, fmt.Errorf("failed to visit URL: %w", err)
	}

	if news == nil {
		return nil, fmt.Errorf("no article content found at URL: %s", url)
	}

	return news, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// parseRelativeTime parses relative time strings like "3 hours ago", "25 minutes ago"
// and returns the approximate absolute time
func parseRelativeTime(text string) time.Time {
	now := time.Now()
	text = strings.ToLower(strings.TrimSpace(text))

	// Try to extract number and unit
	var num int
	var unit string
	_, err := fmt.Sscanf(text, "%d %s", &num, &unit)
	if err != nil || num <= 0 {
		return now
	}

	unit = strings.TrimSuffix(unit, "s") // "hours" -> "hour", "minutes" -> "minute"

	switch unit {
	case "minute":
		return now.Add(-time.Duration(num) * time.Minute)
	case "hour":
		return now.Add(-time.Duration(num) * time.Hour)
	case "day":
		return now.Add(-time.Duration(num) * 24 * time.Hour)
	case "week":
		return now.Add(-time.Duration(num) * 7 * 24 * time.Hour)
	default:
		return now
	}
}

// extractTitle extracts article title with multiple fallback strategies
func extractTitle(e *colly.HTMLElement) string {
	// Try multiple selectors in order of preference
	selectors := []string{
		"h1.post__title",
		"h1.article-title",
		"h1.article__title",
		".post-title h1",
		"article h1",
		"h1",
		"meta[property='og:title']",
		"title",
	}

	for _, selector := range selectors {
		var title string
		if strings.Contains(selector, "meta") {
			title = e.ChildAttr(selector, "content")
		} else {
			title = e.ChildText(selector)
		}

		if title != "" && len(title) > 10 {
			// Clean up title
			title = strings.TrimSpace(title)
			// Remove common suffixes
			title = strings.Split(title, " | ")[0]
			title = strings.Split(title, " - ")[0]
			return title
		}
	}

	return "Untitled Article"
}

// extractContent extracts article content intelligently (generic for all sources)
func extractContent(e *colly.HTMLElement, source string) string {
	var contentParts []string

	// Generic content selectors (work for most news sites)
	selectors := []string{
		"article .content p",
		"article .post-content p",
		"article .article-content p",
		"article .entry-content p",
		"article .article-body p",
		"article .story-body p",
		"article main p",
		".content article p",
		".post-content p",
		".article-content p",
		".entry-content p",
		".article-body p",
		".story-body p",
		"main article p",
		"article p",
		".content p",
		"main p",
	}

	// Try each selector
	for _, selector := range selectors {
		e.ForEach(selector, func(_ int, el *colly.HTMLElement) {
			text := strings.TrimSpace(el.Text)
			// Filter out short paragraphs (likely navigation/ads)
			// Also filter out common non-content text
			if len(text) > 50 && !isNonContentText(text) {
				contentParts = append(contentParts, text)
			}
		})

		// If we got enough content, break
		if len(contentParts) > 5 {
			break
		}
	}

	// Fallback 1: get all text from article tag
	if len(contentParts) < 3 {
		articleText := e.ChildText("article")
		if articleText != "" && len(articleText) > 200 {
			// Split by newlines and filter
			lines := strings.Split(articleText, "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if len(line) > 50 && !isNonContentText(line) {
					contentParts = append(contentParts, line)
				}
			}
		}
	}

	// Fallback 2: get from main tag
	if len(contentParts) < 3 {
		mainText := e.ChildText("main")
		if mainText != "" && len(mainText) > 200 {
			lines := strings.Split(mainText, "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if len(line) > 50 && !isNonContentText(line) {
					contentParts = append(contentParts, line)
				}
			}
		}
	}

	if len(contentParts) == 0 {
		return ""
	}

	fullContent := strings.Join(contentParts, "\n\n")
	return cleanContent(fullContent)
}

// isNonContentText checks if text is likely not article content
func isNonContentText(text string) bool {
	textLower := strings.ToLower(text)
	nonContentPatterns := []string{
		"subscribe",
		"newsletter",
		"follow us",
		"share this",
		"related articles",
		"read more",
		"advertisement",
		"sponsored",
		"cookie",
		"privacy policy",
		"terms of service",
		"©",
		"all rights reserved",
	}

	for _, pattern := range nonContentPatterns {
		if strings.Contains(textLower, pattern) {
			return true
		}
	}

	return false
}

// extractAuthor extracts article author
func extractAuthor(e *colly.HTMLElement) string {
	selectors := []string{
		".post-meta__author-name",
		".author-name",
		".post__author",
		".article__author",
		".author",
		"[rel='author']",
		"meta[name='author']",
	}

	for _, selector := range selectors {
		var author string
		if strings.Contains(selector, "meta") {
			author = e.ChildAttr(selector, "content")
		} else {
			author = e.ChildText(selector)
		}

		if author != "" {
			return strings.TrimSpace(author)
		}
	}

	return ""
}

// extractImage extracts article main image (generic for all sources)
func extractImage(e *colly.HTMLElement) string {
	// First try Open Graph and Twitter meta tags (most reliable)
	metaSelectors := []string{
		"meta[property='og:image']",
		"meta[name='og:image']",
		"meta[property='twitter:image']",
		"meta[name='twitter:image']",
		"meta[itemprop='image']",
	}

	for _, selector := range metaSelectors {
		imageURL := e.ChildAttr(selector, "content")
		if imageURL != "" {
			return normalizeImageURL(imageURL, e.Request.URL)
		}
	}

	// Then try image selectors with multiple attributes
	imageSelectors := []string{
		"article .featured-image img",
		"article .post-image img",
		"article .article-image img",
		"article .post-cover img",
		"article .post__picture img",
		".featured-image img",
		".post-image img",
		".article-image img",
		".post-cover img",
		".post__picture img",
		".hero-image img",
		".main-image img",
		"article header img",
		"article img:first-of-type",
		"main img:first-of-type",
		"article img",
		"main img",
	}

	var foundImageURL string
	for _, selector := range imageSelectors {
		e.ForEach(selector, func(_ int, imgEl *colly.HTMLElement) {
			if foundImageURL != "" {
				return // Already found an image
			}

			// Try multiple attributes for lazy loading
			imageURL := imgEl.Attr("src")
			if imageURL == "" {
				imageURL = imgEl.Attr("data-src")
			}
			if imageURL == "" {
				imageURL = imgEl.Attr("data-lazy-src")
			}
			if imageURL == "" {
				imageURL = imgEl.Attr("data-original")
			}
			if imageURL == "" {
				imageURL = imgEl.Attr("data-image")
			}
			if imageURL == "" {
				// Try srcset
				srcset := imgEl.Attr("srcset")
				if srcset != "" {
					// Extract first URL from srcset
					parts := strings.Split(srcset, ",")
					if len(parts) > 0 {
						imageURL = strings.TrimSpace(strings.Split(parts[0], " ")[0])
					}
				}
			}

			if imageURL != "" {
				normalized := normalizeImageURL(imageURL, e.Request.URL)
				if normalized != "" {
					foundImageURL = normalized
				}
			}
		})

		if foundImageURL != "" {
			break
		}
	}

	return foundImageURL
}

// normalizeImageURL converts relative URLs to absolute URLs
func normalizeImageURL(imageURL string, baseURL *url.URL) string {
	if imageURL == "" {
		return ""
	}

	// Remove query parameters that might cause issues
	if idx := strings.Index(imageURL, "?"); idx > 0 {
		imageURL = imageURL[:idx]
	}

	// Handle protocol-relative URLs
	if strings.HasPrefix(imageURL, "//") {
		return "https:" + imageURL
	}

	// Handle absolute URLs
	if strings.HasPrefix(imageURL, "http://") || strings.HasPrefix(imageURL, "https://") {
		return imageURL
	}

	// Handle relative URLs
	if strings.HasPrefix(imageURL, "/") {
		return baseURL.Scheme + "://" + baseURL.Host + imageURL
	}

	// Handle relative URLs without leading slash
	return baseURL.Scheme + "://" + baseURL.Host + "/" + imageURL
}

// extractTags extracts article tags/categories
func extractTags(e *colly.HTMLElement) []string {
	var tags []string
	tagMap := make(map[string]bool) // Deduplicate

	selectors := []string{
		".post__tags a",
		".tags a",
		".article-tags a",
		".category a",
		".tag a",
	}

	for _, selector := range selectors {
		e.ForEach(selector, func(_ int, tagEl *colly.HTMLElement) {
			tag := strings.TrimSpace(tagEl.Text)
			if tag != "" && !tagMap[tag] {
				tagMap[tag] = true
				tags = append(tags, tag)
			}
		})
	}

	return tags
}

// extractPublishedDate extracts article published date
func extractPublishedDate(e *colly.HTMLElement) time.Time {
	selectors := []string{
		"meta[property='article:published_time']",
		"meta[name='publish-date']",
		"time[datetime]",
		".publish-date",
		".post-date",
	}

	for _, selector := range selectors {
		var dateStr string
		if strings.Contains(selector, "meta") {
			dateStr = e.ChildAttr(selector, "content")
		} else if strings.Contains(selector, "time") {
			dateStr = e.ChildAttr(selector, "datetime")
		} else {
			dateStr = e.ChildText(selector)
		}

		if dateStr != "" {
			// Try to parse various date formats
			formats := []string{
				time.RFC3339,
				"2006-01-02T15:04:05Z07:00",
				"2006-01-02",
				"January 2, 2006",
			}

			for _, format := range formats {
				if t, err := time.Parse(format, dateStr); err == nil {
					return t
				}
			}
		}
	}

	return time.Now()
}

// cleanContent cleans and normalizes content
func cleanContent(content string) string {
	// Remove excessive whitespace
	content = strings.Join(strings.Fields(content), " ")

	// Remove common noise patterns
	noise := []string{
		"Subscribe to our newsletter",
		"Sign up for our newsletter",
		"Follow us on",
		"Share this article",
		"Related articles",
		"Advertisement",
	}

	for _, pattern := range noise {
		content = strings.ReplaceAll(content, pattern, "")
	}

	return strings.TrimSpace(content)
}

// createSummary creates a summary from content
func createSummary(content string, maxLen int) string {
	if len(content) <= maxLen {
		return content
	}

	// Try to cut at sentence boundary
	summary := content[:maxLen]
	lastPeriod := strings.LastIndex(summary, ".")
	lastQuestion := strings.LastIndex(summary, "?")
	lastExclamation := strings.LastIndex(summary, "!")

	cutPoint := max(lastPeriod, max(lastQuestion, lastExclamation))
	if cutPoint > maxLen/2 {
		return strings.TrimSpace(summary[:cutPoint+1])
	}

	// Cut at last space
	lastSpace := strings.LastIndex(summary, " ")
	if lastSpace > 0 {
		return strings.TrimSpace(summary[:lastSpace]) + "..."
	}

	return summary + "..."
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
