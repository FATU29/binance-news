# Crawler API Improvements - Adaptive & Intelligent Crawling

## 📊 So sánh với yêu cầu

### ✅ Đã có (Existing Features)

1. **Thu thập từ nhiều nguồn**: 10 sources configured
2. **Dữ liệu đầy đủ**: News model with comprehensive fields for analysis
3. **StructureMonitor**: Basic tracking of selector success/failure
4. **ContentQuality**: Filter low-quality content

### 🚀 Cải tiến mới (New Improvements)

#### 1. **Tự động học cấu trúc HTML (Auto-Learn Structure)** ✨

**File**: `internal/service/selector_learner.go`

**Tính năng**:

- **Tự động phát hiện selectors**: Scan trang web và tự động tìm ra CSS selectors phù hợp
- **Học từ lịch sử**: Track success/failure rate của mỗi selector
- **Confidence scoring**: Đánh giá độ tin cậy của selector (0-100%)
- **Primary selector promotion**: Tự động promote selector hoạt động tốt nhất

**Cơ chế hoạt động**:

```go
// Discover selectors từ một trang web
discovered, err := selectorLearner.DiscoverSelectors(ctx, sourceURL, sourceName)

// Patterns được test:
- Article containers: article, .post, .news-item, etc.
- Titles: h1, h2.title, [itemprop='headline'], etc.
- Content: .content, .article-body, [itemprop='articleBody']
- Images: .featured-image img, figure img
- Authors: .author, [rel='author'], [itemprop='author']
- Dates: time, [datetime], .published
```

**Database Tables**:

```sql
learned_selectors: Stores discovered selectors with confidence scores
selector_fallbacks: Fallback strategies when primary fails
```

#### 2. **Xử lý thay đổi HTML (Handle HTML Changes)** 🔄

**File**: `internal/crawler/adaptive_crawler.go`

**Tính năng**:

- **Automatic fallback**: Khi primary selector fail, tự động thử fallback selectors
- **Auto-healing**: Tự động tìm và promote selector thay thế
- **Multiple extraction strategies**: Thử nhiều pattern khác nhau cho cùng element
- **Structure change detection**: Phát hiện khi website thay đổi cấu trúc

**Flow xử lý**:

```
1. Try primary selector
   ↓ (if fails or < 3 results)
2. Try learned fallback selectors
   ↓ (if still fails)
3. Trigger auto-discovery
   ↓
4. Update configuration with new selector
   ↓
5. Send alert to monitor
```

**Ví dụ**:

```go
adaptiveCrawler := NewAdaptiveCrawler(baseCrawler, learner, monitor)
results, err := adaptiveCrawler.CrawlAdaptive(ctx, source)

// Tự động:
// - Detect failed selector
// - Try 5 fallback selectors
// - Record success/failure
// - Auto-heal if needed
```

#### 3. **Quản lý nguồn động (Dynamic Source Management)** 🎛️

**Files**:

- `internal/service/source_service.go`
- `internal/handler/source_handler.go`

**API Endpoints**:

##### **GET /api/v1/sources** - Liệt kê tất cả sources

```bash
curl -X GET "http://localhost:9000/api/v1/sources?enabled=true"
```

##### **GET /api/v1/sources/{name}** - Chi tiết source

```bash
curl -X GET "http://localhost:9000/api/v1/sources/cointelegraph"
```

##### **POST /api/v1/sources** - Tạo source mới

```bash
curl -X POST "http://localhost:9000/api/v1/sources" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "new-crypto-site",
    "base_url": "https://example.com",
    "enabled": true,
    "priority": 8,
    "crawl_freq": 30,
    "selectors": {
      "article_list": ".news-item",
      "title": "h1",
      "content": ".article-content"
    },
    "language": "en"
  }'
```

##### **PUT /api/v1/sources/{name}** - Cập nhật source

```bash
curl -X PUT "http://localhost:9000/api/v1/sources/cointelegraph" \
  -H "Content-Type: application/json" \
  -d '{
    "enabled": false,
    "priority": 5
  }'
```

##### **DELETE /api/v1/sources/{name}** - Xóa source

```bash
curl -X DELETE "http://localhost:9000/api/v1/sources/old-source"
```

##### **PUT /api/v1/sources/{name}/selectors** - Cập nhật selectors

```bash
curl -X PUT "http://localhost:9000/api/v1/sources/cointelegraph/selectors" \
  -H "Content-Type: application/json" \
  -d '{
    "article_list": ".new-selector",
    "title": "h1.new-title"
  }'
```

##### **POST /api/v1/sources/{name}/enable** - Enable source

##### **POST /api/v1/sources/{name}/disable** - Disable source

##### **POST /api/v1/sources/{name}/test** - Test source

##### **GET /api/v1/sources/{name}/health** - Health metrics

#### 4. **Selector Learning APIs** 🎓

**File**: `internal/handler/selector_handler.go`

##### **POST /api/v1/selectors/discover** - Tự động phát hiện selectors

```bash
curl -X POST "http://localhost:9000/api/v1/selectors/discover" \
  -H "Content-Type: application/json" \
  -d '{
    "source_url": "https://cointelegraph.com/tags/bitcoin",
    "source_name": "cointelegraph"
  }'

# Response:
{
  "success": true,
  "data": {
    "message": "Selectors discovered successfully",
    "count": 15,
    "selectors": [
      {
        "source": "cointelegraph",
        "element_type": "article_list",
        "selector": ".post-card-inline",
        "confidence": 85.5,
        "success_count": 1
      },
      ...
    ]
  }
}
```

##### **GET /api/v1/selectors/best** - Lấy selector tốt nhất

```bash
curl -X GET "http://localhost:9000/api/v1/selectors/best?source=cointelegraph&element_type=title"

# Response: Best performing selector with highest confidence
```

##### **GET /api/v1/selectors/fallbacks** - Lấy fallback selectors

```bash
curl -X GET "http://localhost:9000/api/v1/selectors/fallbacks?source=cointelegraph&element_type=content"

# Response: List of 5 fallback selectors sorted by confidence
```

##### **POST /api/v1/selectors/record** - Ghi nhận kết quả extraction

```bash
curl -X POST "http://localhost:9000/api/v1/selectors/record" \
  -H "Content-Type: application/json" \
  -d '{
    "source": "cointelegraph",
    "element_type": "title",
    "selector": "h1.post__title",
    "success": true
  }'
```

##### **POST /api/v1/selectors/promote** - Promote selector thành primary

```bash
curl -X POST "http://localhost:9000/api/v1/selectors/promote" \
  -H "Content-Type: application/json" \
  -d '{
    "source": "cointelegraph",
    "element_type": "title",
    "selector": "h1.new-selector"
  }'
```

##### **POST /api/v1/selectors/auto-heal** - Tự động sửa selector lỗi

```bash
curl -X POST "http://localhost:9000/api/v1/selectors/auto-heal" \
  -H "Content-Type: application/json" \
  -d '{
    "source": "cointelegraph",
    "element_type": "title",
    "failed_selector": "h1.old-selector"
  }'

# Tự động:
# 1. Record failure of old selector
# 2. Try fallback selectors
# 3. Find working replacement
# 4. Promote new selector
# 5. Return replacement
```

##### **GET /api/v1/selectors/stats/{source}** - Thống kê selectors

```bash
curl -X GET "http://localhost:9000/api/v1/selectors/stats/cointelegraph"

# Response:
{
  "source": "cointelegraph",
  "total_learned": 25,
  "total_active": 20,
  "by_element": {
    "title": {"total": 5, "active": 4},
    "content": {"total": 8, "active": 7},
    ...
  }
}
```

##### **GET /api/v1/selectors/list** - Danh sách selectors (with filters)

```bash
curl -X GET "http://localhost:9000/api/v1/selectors/list?source=cointelegraph&element_type=title&active=true&page=1&limit=20"
```

## 🗄️ Database Schema

### New Tables

#### 1. **learned_selectors**

```sql
- id: Serial primary key
- source: Source name
- element_type: title, content, image, etc.
- selector: CSS selector string
- confidence: 0-100 score
- success_count: Number of successful extractions
- failure_count: Number of failed extractions
- is_active: Currently active?
- is_primary: Primary selector for this type?
- discovered_at: When was it discovered
- last_tested: Last test timestamp
```

**Indexes**: source, element_type, confidence, active, primary

#### 2. **selector_fallbacks**

```sql
- id: Serial primary key
- source: Source name
- element_type: Element type
- primary_selector: Main selector
- fallback_selector: Backup selector
- priority: Lower = higher priority
- success_rate: Historical success rate
- last_used: Last usage timestamp
```

#### 3. **crawl_sources**

```sql
- id: Unique identifier
- name: Source name (unique)
- base_url: Website URL
- enabled: Active status
- priority: Crawl priority (higher = more important)
- crawl_freq: Minutes between crawls
- last_crawled: Last crawl timestamp
- selectors: JSONB of selectors
- categories: JSONB array of categories
- language: Content language
```

### Views

#### **selector_health_view**

Monitoring view showing:

- Confidence scores
- Success/failure rates
- Health status (healthy/warning/critical/inactive)
- Last tested timestamps

## 🔧 Integration Steps

### 1. Run Database Migration

```bash
cd /home/fat/code/cryto-final-project/crawl-news
psql -U postgres -d crypto_news -f migrations/002_add_selector_learning.sql
```

### 2. Update main.go

Wire up new services and handlers (see updated main.go below)

### 3. Update Routes

Add new routes for sources and selectors

### 4. Test New Features

```bash
# Test source management
curl -X GET "http://localhost:9000/api/v1/sources"

# Test selector discovery
curl -X POST "http://localhost:9000/api/v1/selectors/discover" \
  -H "Content-Type: application/json" \
  -d '{"source_url": "https://cointelegraph.com/tags/bitcoin", "source_name": "cointelegraph"}'

# Test adaptive crawling (should auto-fallback if selectors fail)
curl -X POST "http://localhost:9000/api/v1/crawler/start" \
  -H "Content-Type: application/json" \
  -d '{"source": "cointelegraph"}'
```

## 📈 Benefits

### 1. **Resilience** 💪

- System tự động adapt khi website thay đổi cấu trúc
- Không cần manual intervention khi selector break
- Multiple fallback strategies

### 2. **Scalability** 📊

- Dễ dàng thêm source mới qua API
- Không cần code changes để add new sites
- Dynamic configuration management

### 3. **Intelligence** 🧠

- Tự học patterns từ nhiều sites
- Cải thiện accuracy theo thời gian
- Confidence-based selector selection

### 4. **Maintainability** 🔧

- Centralized source management
- Health monitoring và alerts
- Clear separation of concerns

### 5. **Observability** 👀

- Detailed statistics về selector performance
- Health metrics per source
- Historical tracking of changes

## 🎯 Use Cases

### Case 1: Website thay đổi cấu trúc

```
1. User notices không crawl được data từ CoinTelegraph
2. System tự động detect: primary selector ".post-card" fail
3. Adaptive crawler tries fallbacks: ".article-item", ".news-card"
4. Found working: ".news-card" với 87% confidence
5. Auto-promote ".news-card" as new primary
6. Alert admin về thay đổi
7. Continue crawling normally
```

### Case 2: Thêm nguồn tin mới

```
1. Admin gọi POST /api/v1/sources với new site info
2. System lưu configuration
3. Gọi POST /api/v1/selectors/discover để auto-detect selectors
4. System test discovered selectors
5. Promote best performing selectors
6. Source sẵn sàng để crawl
```

### Case 3: Optimize existing source

```
1. Gọi GET /api/v1/sources/coindesk/health
2. Thấy success_rate đang giảm (72%)
3. Trigger POST /api/v1/selectors/discover để re-learn
4. So sánh với current selectors
5. Update selectors nếu found better ones
6. Monitor improvement
```

## 🚀 Next Steps

1. **Implement JSON serialization** cho Selector trong source_service.go
2. **Add authentication** cho source/selector management APIs
3. **Add webhooks** để notify khi có structural changes
4. **Machine Learning** để predict selector reliability
5. **A/B Testing** framework cho selectors
6. **Visual selector picker** tool (GUI)

## 📝 Notes

- Migration file đã tạo: `migrations/002_add_selector_learning.sql`
- Cần update main.go để wire up new services
- Tất cả APIs đều có error handling và logging
- Database có indexes appropriate cho performance
- View `selector_health_view` để monitor health
