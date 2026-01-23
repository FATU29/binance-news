# AI-Only Crawling Mode

## Tổng quan

Chế độ AI-only cho phép crawler chỉ sử dụng AI parser để extract content, bỏ qua hoàn toàn rule-based parsing. Điều này hữu ích khi:
- Muốn test AI parser với dữ liệu mới
- Website structure thay đổi quá nhiều
- Muốn đảm bảo tất cả articles đều được parse bằng AI

## Cấu hình

### 1. Environment Variables

Thêm vào `.env` hoặc docker-compose:

```bash
# Force AI-only parsing (skip rule-based)
AI_FORCE_PARSING=true

# AI Service URL
AI_SERVICE_URL=http://ai:8000
AI_SERVICE_TIMEOUT=60
```

### 2. Clear Database

```bash
# Clear PostgreSQL database
cd crawl-news
./scripts/clear-database.sh
```

Hoặc trong Docker:

```bash
# Connect to postgres container
docker exec -it crypto-postgres psql -U postgres -d crypto_news

# Clear all tables
DROP SCHEMA public CASCADE;
CREATE SCHEMA public;
GRANT ALL ON SCHEMA public TO postgres;
GRANT ALL ON SCHEMA public TO public;
```

### 3. Start Crawl với AI-only mode

```bash
# Set environment variable
export AI_FORCE_PARSING=true

# Start crawl service
docker-compose up -d crawl

# Or use the script
./scripts/clear-and-crawl.sh
```

## Cách hoạt động

### Normal Mode (Hybrid)
1. Thử rule-based parsing trước
2. Nếu fail hoặc chất lượng thấp → Fallback sang AI
3. Nếu AI fail → Fallback về rule-based

### AI-Only Mode
1. **Bỏ qua hoàn toàn rule-based parsing**
2. Chỉ dùng AI parser
3. Nếu AI fail → Không có fallback (log error)

## Code Changes

### Config
- `AIServiceConfig.ForceAIParsing` - Flag để enable AI-only mode
- Load từ env: `AI_FORCE_PARSING=true`

### Crawler Logic
- `CrawlDetailPage` kiểm tra `IsForceAIParsing()`
- Nếu true → Skip rule-based, chỉ dùng AI
- Nếu false → Hybrid mode (rule-based first, AI fallback)

## Monitoring

### Logs
```
AI-only mode: Using AI parser for: https://example.com/article
AI parser successfully extracted article: Title (1234 chars, method: ai, confidence: 0.95)
```

### Database
- Tất cả articles sẽ có `parsing_method = "ai"`
- `parsing_confidence` sẽ được set từ AI response

## Performance

- **Normal mode**: ~50-200ms/page (rule-based success) hoặc ~2-5s/page (AI fallback)
- **AI-only mode**: ~2-5s/page (luôn dùng AI)

## Cost

- **Normal mode**: ~$0.0001-0.001/page (chỉ dùng AI khi cần)
- **AI-only mode**: ~$0.001-0.01/page (luôn dùng AI)

## Troubleshooting

### AI Service không available
```
Error: failed to call AI HTML parser: connection refused
```
→ Kiểm tra AI service đang chạy và `AI_SERVICE_URL` đúng

### AI parsing fail
```
AI parser failed in AI-only mode: ...
```
→ Kiểm tra OpenAI API key và logs của AI service

### Database connection
```
Error: failed to connect to database
```
→ Kiểm tra PostgreSQL đang chạy và connection string đúng

## Best Practices

1. **Test với ít sources trước**: Crawl 1-2 sources để test
2. **Monitor costs**: AI-only mode tốn nhiều hơn
3. **Check logs**: Theo dõi success rate và errors
4. **Backup data**: Trước khi clear database, backup nếu cần

## Example Usage

```bash
# 1. Clear database
./scripts/clear-database.sh

# 2. Set AI-only mode
export AI_FORCE_PARSING=true

# 3. Start services
docker-compose up -d

# 4. Start crawl
curl -X POST http://localhost:9000/api/v1/crawler/start \
  -H "Content-Type: application/json" \
  -d '{"source": "cointelegraph"}'

# 5. Check results
curl http://localhost:9000/api/v1/news?limit=10
```
