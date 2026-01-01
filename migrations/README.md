# Database Migrations

This directory contains SQL migration scripts for the database schema.

## Running Migrations

### Manual Migration (Development)

```bash
# Connect to PostgreSQL
psql -U postgres -d crypto_news -f migrations/001_add_monitoring_tables.sql
```

### Docker Environment

```bash
# Copy migration file to container
docker cp migrations/001_add_monitoring_tables.sql crypto-postgres:/tmp/

# Execute migration
docker exec -it crypto-postgres psql -U postgres -d crypto_news -f /tmp/001_add_monitoring_tables.sql
```

### Using Go Migration Tool

```go
// In main.go or separate migration script
import "crawl-news/internal/db"

func runMigrations() {
    // Auto-migrate models
    db.DB.AutoMigrate(
        &service.SelectorPattern{},
        &service.StructureAlert{},
    )
}
```

## Migration Files

### 001_add_monitoring_tables.sql
- Adds `selector_patterns` table for tracking selector success rates
- Adds `structure_alerts` table for monitoring structure changes
- Creates indexes for performance
- Adds constraints for data integrity

## Schema

### selector_patterns
Tracks how well different CSS selectors work for extracting data from news sources.

| Column | Type | Description |
|--------|------|-------------|
| id | SERIAL | Primary key |
| source | VARCHAR(100) | News source name |
| element_type | VARCHAR(50) | Type of element (title, content, etc.) |
| selector | VARCHAR(500) | CSS selector string |
| success_count | INT | Number of successful extractions |
| failure_count | INT | Number of failed extractions |
| success_rate | FLOAT | Success percentage (0-100) |
| last_used | TIMESTAMP | When this selector was last used |

### structure_alerts
Alerts when website structure changes are detected.

| Column | Type | Description |
|--------|------|-------------|
| id | SERIAL | Primary key |
| source | VARCHAR(100) | News source name |
| selector | VARCHAR(500) | Failing selector |
| failure_count | INT | Number of consecutive failures |
| severity | VARCHAR(20) | 'warning' or 'critical' |
| notified | BOOLEAN | Whether admin has been notified |
| resolved | BOOLEAN | Whether issue is resolved |
| created_at | TIMESTAMP | When alert was created |
| resolved_at | TIMESTAMP | When alert was resolved |

## Rollback

To rollback migrations:

```sql
DROP TABLE IF EXISTS structure_alerts;
DROP TABLE IF EXISTS selector_patterns;
```

## Best Practices

1. **Naming**: Use format `NNN_description.sql` (e.g., `001_add_monitoring_tables.sql`)
2. **Idempotency**: Use `CREATE TABLE IF NOT EXISTS` for safety
3. **Comments**: Add comments explaining purpose
4. **Testing**: Test migrations on development database first
5. **Backup**: Always backup production database before migrating

## Future Migrations

Future migrations should be numbered sequentially:
- `002_add_user_tables.sql` - User accounts and preferences
- `003_add_price_alignments.sql` - News-price correlation data
- etc.

